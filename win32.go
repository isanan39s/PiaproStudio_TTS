package main

///copilotくんが書いてくれました
import (
	"bufio"
	"fmt"
	"os"
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
)

const (
	CS_HREDRAW          = 0x0002
	CS_VREDRAW          = 0x0001
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	SW_SHOW             = 5
	WM_DESTROY          = 0x0002
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

func getModuleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
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

	h, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(WS_OVERLAPPEDWINDOW),
		100, 100, 800, 600,
		0,
		0,
		uintptr(getModuleHandle()),
		0,
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
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) == -1 || ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
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
	closeCode, _ := opcodes["PlugEditClose"]

	hwnd, err := createWin32Window("VST Plugin Host Window")
	if err != nil {
		return fmt.Errorf("create window failed: %w", err)
	}
	fmt.Println("created window hwnd:", hwnd)

	done := make(chan struct{})
	go runMessageLoop(done)

	// call PlugEditOpen with parent HWND
	parentPtr := unsafe.Pointer(uintptr(hwnd))
	plugin.Dispatch(vst2.PluginOpcode(openCode), 0, 0, parentPtr, 0)
	fmt.Println(" PlugEditOpen dispatched (parent HWND passed)")

	fmt.Println("Press Enter to close editor...")
	bufio.NewScanner(os.Stdin).Scan()

	// close editor if opcode exists
	if closeCode != 0 {
		plugin.Dispatch(vst2.PluginOpcode(closeCode), 0, 0, nil, 0)
	}
	// Wait for message loop to end (window may be destroyed via wndProc)
	<-done
	return nil
}
