package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"pipelined.dev/audio/vst2"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procPeekMessageW     = user32.NewProc("PeekMessageW")
	procSleep            = kernel32.NewProc("Sleep")
)

const (
	CS_HREDRAW          = 0x0002
	CS_VREDRAW          = 0x0001
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	SW_SHOW             = 5
	WM_DESTROY          = 0x0002
	PM_REMOVE           = 0x0001
)

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type MSG struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// 修正版: getModuleHandle
func getModuleHandle() uintptr {
	// kernel32.GetModuleHandleW(NULL) を呼ぶ
	// NULL を渡すとカレント実行ファイルのハンドルが返される
	h, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	return h
}

func wndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	default:
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	}
}

// createWin32Window 内の CreateWindowExW 呼び出し部分を修正
func createWin32Window(title string) (uintptr, error) {
	className, _ := syscall.UTF16PtrFromString("GoVSTHostClass")
	titlePtr, _ := syscall.UTF16PtrFromString(title)

	wnd := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   syscall.NewCallback(wndProc),
		CbClsExtra:    0,
		CbWndExtra:    0,
		HInstance:     syscall.Handle(getModuleHandle()),
		HIcon:         0,
		HCursor:       0,
		HbrBackground: 0,
		LpszMenuName:  nil,
		LpszClassName: className,
		HIconSm:       0,
	}

	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wnd)))
	if r == 0 {
		return 0, fmt.Errorf("RegisterClassExW failed: %v", err)
	}

	// CreateWindowExW: (exStyle, className, windowName, style, x, y, width, height, parent, menu, instance, param)
	h, _, err := procCreateWindowExW.Call(
		0,                                  // exStyle
		uintptr(unsafe.Pointer(className)), // className (LPCWSTR)
		uintptr(unsafe.Pointer(titlePtr)),  // windowName (LPCWSTR)
		uintptr(WS_OVERLAPPEDWINDOW),       // style
		100,                                // x
		100,                                // y
		800,                                // width
		600,                                // height
		0,                                  // parent HWND (NULL = top-level)
		0,                                  // menu (NULL)
		getModuleHandle(),                  // hInstance
		0,                                  // lpParam (NULL)
	)
	if h == 0 {
		return 0, fmt.Errorf("CreateWindowExW failed: %v", err)
	}

	procShowWindow.Call(h, SW_SHOW)
	procUpdateWindow.Call(h)

	return h, nil
}

func runMessageLoop(done chan struct{}) {
	var msg MSG
	for {
		// PeekMessage: ノンブロッキングでメッセージをチェック
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, PM_REMOVE)

		if ret > 0 {
			// メッセージがあれば処理
			if msg.Message == 0x0012 { // WM_QUIT
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		} else {
			// メッセージがなければ少し待機（CPU 負荷軽減）
			procSleep.Call(10)
		}

		// done チャネルがクローズされたかチェック
		select {
		case <-done:
			return
		default:
		}
	}
	close(done)
}

// OpenPluginGUIWithWindow creates a Win32 window, opens the plugin editor with that window as parent,
// runs a message loop in a goroutine, waits for Enter on stdin, then closes the editor.
func OpenPluginGUIWithWindow(plugin *vst2.Plugin, opcodes map[string]int) error {
	openCode, ok := opcodes["PlugEditOpen"]
	if !ok {
		return fmt.Errorf("PlugEditOpen opcode not found")
	}
	//closeCode, _ := opcodes["PlugEditClose"]

	fmt.Println("create window")
	hwnd, err := createWin32Window("VST Plugin Host Window")
	if err != nil {
		return fmt.Errorf("create window failed: %w", err)
	}
	fmt.Println("created window hwnd:", hwnd)

	// プラグインを実行状態にする（GUI 開く前に必須）
	plugin.Start()
	//plugin.Resume()

	// call PlugEditOpen with parent HWND
	parentPtr := unsafe.Pointer(uintptr(hwnd))
	fmt.Println("open window")
	plugin.Dispatch(vst2.PluginOpcode(openCode), 0, 0, parentPtr, 0)
	fmt.Println(" PlugEditOpen dispatched (parent HWND passed)")
	fmt.Println("Close the window to exit...")

	//done := make(chan struct{})

	// メッセージループを実行（ウィンドウ破棄で終了）
	//runMessageLoop(done)

	// Suspend and close
	//plugin.Suspend()

	// close editor if opcode exists
	// if closeCode != 0 {
	// 	plugin.Dispatch(vst2.PluginOpcode(closeCode), 0, 0, nil, 0)
	// }

	return nil
}
