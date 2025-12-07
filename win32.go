package main

///copilotくんが書いてくれました
import (
	"fmt"
	"runtime"
	"syscall"
	"time"
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
	// 追加: PeekMessageW と Sleep
	procPeekMessageW = user32.NewProc("PeekMessageW")
	procSleep        = kernel32.NewProc("Sleep")
	procSetWindowPos = user32.NewProc("SetWindowPos")
)

const (
	CS_HREDRAW          = 0x0002
	CS_VREDRAW          = 0x0001
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	SW_SHOW             = 5
	WM_DESTROY          = 0x0002
	// 追加: PM_REMOVE
	PM_REMOVE    = 0x0001
	SWP_NOMOVE   = 0x0002
	SWP_NOZORDER = 0x0004
)

// ExecRequest は GUI スレッド上で実行したい関数を表すリクエスト。
// 呼び出し側は Resp チャネルで完了／エラーを受け取る。
type ExecRequest struct {
	Fn   func() error
	Resp chan error
}

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

type ERect struct {
	Top    int16
	Left   int16
	Bottom int16
	Right  int16
}

func getModuleHandle() uintptr {
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
		0,
		0,
		800,
		600,
		0,
		0,
		getModuleHandle(),
		0,
	)
	if h == 0 {
		return 0, fmt.Errorf("CreateWindowExW failed: %v", err)
	}

	procShowWindow.Call(h, SW_SHOW)
	procUpdateWindow.Call(h)

	return h, nil
}

// runMessageLoop はメッセージを処理しつつ、exec チャネル経由で
// GUI スレッド上で実行すべき関数を受け取り実行する。
func runMessageLoop(plugin *vst2.Plugin, opcodes map[string]int, exec chan ExecRequest) {
	var msg MSG
	idleCode, hasIdle := opcodes["PlugEditIdle"] // オペコードをループの前に一度取得

	for {
		// メッセージ処理（ノンブロッキング）
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, PM_REMOVE)
		if ret > 0 {
			if msg.Message == 0x0012 { // WM_QUIT
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		} else {
			// メッセージがないときは exec リクエストをチェックするか、アイドル処理
			select {
			case req, ok := <-exec:
				if !ok {
					// exec が閉じられたらループを抜ける
					return
				}
				var err error
				func() { // recover でパニックを捕まえる
					defer func() {
						if r := recover(); r != nil {
							err = fmt.Errorf("exec panic: %v", r)
						}
					}()
					err = req.Fn()
				}()
				select {
				case req.Resp <- err:
				default:
				}
			default:
				// メッセージも exec リクエストもなければ、プラグインをアイドル状態にしてスリープ
				if hasIdle {
					plugin.Dispatch(vst2.PluginOpcode(idleCode), 0, 0, nil, 0)
				}
				procSleep.Call(10)
			}
		}
	}
}

// OpenPluginGUIWithWindow は即座に (exec, nil) を返し、GUI スレッド
// はバックグラウンドで動作する。exec に ExecRequest を送ると GUI スレッド上で実行される。
func OpenPluginGUIWithWindow(plugin *vst2.Plugin, opcodes map[string]int) (chan ExecRequest, error) {
	openCode, ok := opcodes["PlugEditOpen"]
	if !ok {
		return nil, fmt.Errorf("PlugEditOpen opcode not found")
	}
	closeCode, _ := opcodes["PlugEditClose"]

	exec := make(chan ExecRequest)

	// GUI スレッドを立てる
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hwnd, err := createWin32Window("VST Plugin Host Window")
		if err != nil {
			fmt.Printf("create window failed: %v\n", err)
			// 起動失敗ならチャネルを閉じて終了
			close(exec)
			return
		}
		fmt.Println("created window hwnd:", hwnd)

		plugin.Start()
		plugin.Resume()

		// Set audio settings before getting rect
		if sampleRateCode, ok := opcodes["plugSetSampleRate"]; ok {
			fmt.Println("[GUI Thread] Setting sample rate to 48000")
			plugin.Dispatch(vst2.PluginOpcode(sampleRateCode), 0, 0, nil, float32(48000.0))
		}
		if blockSizeCode, ok := opcodes["plugSetBufferSize"]; ok {
			fmt.Println("[GUI Thread] Setting block size to 512")
			plugin.Dispatch(vst2.PluginOpcode(blockSizeCode), 0, 512, nil, 0)
		}
		if speakerCode, ok := opcodes["plugSetSpeakerArrangement"]; ok {
			fmt.Println("[GUI Thread] Setting speaker arrangement to stereo")
			stereoIn := vst2.SpeakerArrangement{
				NumChannels: 2,
				Type:        3, // kStereo
			}
			stereoOut := vst2.SpeakerArrangement{
				NumChannels: 2,
				Type:        3, // kStereo
			}
			plugin.Dispatch(vst2.PluginOpcode(speakerCode), 0, int64(uintptr(unsafe.Pointer(&stereoIn))), unsafe.Pointer(&stereoOut), 0)
		}

		// Get the plugin's editor rectangle before opening it
		var rectPtr *ERect // ERectへのポインタを保持する変数を宣言
		fmt.Println("[GUI Thread] Getting editor rect (expecting ERect**)...")
		// rectPtrのアドレスを渡すことで、ERect**として渡す
		plugin.Dispatch(vst2.PluginOpcode(13), 0, 0, unsafe.Pointer(&rectPtr), 0)

		if rectPtr != nil {
			rect := *rectPtr // ポインタをデリファレンスして実体を取得
			fmt.Printf("[GUI Thread] Plugin editor rect: %+v\n", rect)

			// ウィンドウのサイズをプラグインが要求したものに変更
			width := rect.Right - rect.Left
			height := rect.Bottom - rect.Top
			if width > 0 && height > 0 {
				fmt.Printf("[GUI Thread] Resizing window to %d x %d\n", width, height)
				procSetWindowPos.Call(
					hwnd,
					0, // Zオーダーは変更しない
					0, // x
					0, // y
					uintptr(width),
					uintptr(height),
					SWP_NOMOVE|SWP_NOZORDER,
				)
				println("resizd")
			}
		} else {
			fmt.Println("[GUI Thread] Warning: PlugEditGetRect returned a nil pointer.")
		}

		println("wait")
		// Add a small delay to allow the system to settle, working around a potential race condition in the plugin.
		time.Sleep(500 * time.Millisecond)

		parentPtr := unsafe.Pointer(uintptr(hwnd))
		plugin.Dispatch(vst2.PluginOpcode(openCode), 0, 0, parentPtr, 0)
		fmt.Println("▶️ PlugEditOpen dispatched (parent HWND passed)")


		// メッセージループ（中で exec を処理する）
		runMessageLoop(plugin, opcodes, exec)

		// GUI 終了処理
		plugin.Suspend()
		if closeCode != 0 {
			plugin.Dispatch(vst2.PluginOpcode(closeCode), 0, 0, nil, 0)
		}

		// exec を閉じる（送信側に通知）
		close(exec)
	}()

	return exec, nil
}
