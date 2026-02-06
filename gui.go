package main

import (
	"log"
	"unsafe"

	"github.com/lxn/walk"
	"pipelined.dev/audio/vst2"
)

var (
	mainWindow *walk.MainWindow
)

// OpenPluginGUIWithWindow は、lxn/walkを使用してウィンドウを作成し、
// その中にVSTプラグインのGUIエディタをホストします。
func OpenPluginGUIWithWindow(plugin *vst2.Plugin, opcodes map[string]int) {
	var err error

	// カスタムウィジェットを作成し、そのHWNDをVSTプラグインに渡す
	var customWidget *walk.CustomWidget

	// MainWindowの定義
	err = walk.NewMainWindow(&walk.MainWindow{
		AssignTo: &mainWindow,
		Title:    "Piapro Studio VSTi",
		MinSize:  walk.Size{Width: 800, Height: 600},
		Layout:   walk.NewVBoxLayout(),
		Children: []walk.Widget{
			// カスタムウィジェットをウィンドウの子として配置
			walk.NewCustomWidget(&customWidget, walk.CustomWidgetStyle(walk.Style{}), func(widget *walk.CustomWidget, p *walk.PaintParams) error {
				// カスタム描画ロジック (今回は不要)
				return nil
			}),
		},
	})

	if err != nil {
		log.Fatalf("Failed to create main window: %v", err)
	}

	// MainWindowが表示された後に実行されるコールバックを設定
	mainWindow.VisibleChanged().Attach(func() {
		if mainWindow.Visible() {
			// ウィンドウのハンドル(HWND)を取得
			hwnd := mainWindow.Handle()

			// VSTプラグインにウィンドウハンドルを渡してGUIを開く
			log.Printf("Dispatching PlugEditOpen with HWND: %v", hwnd)
			plugin.Dispatch(vst2.PluginOpcode(opcodes["PlugEditOpen"]), 0, 0, unsafe.Pointer(hwnd), 0)

			// プラグインのエディタのサイズを取得してウィンドウをリサイズ
			var rect vst2.Rect
			plugin.Dispatch(vst2.PluginOpcode(opcodes["PlugEditGetRect"]), 0, 0, unsafe.Pointer(&rect), 0)

			if rect.Width > 0 && rect.Height > 0 {
				// クライアント領域のサイズを設定
				mainWindow.SetClientSize(walk.Size{Width: int(rect.Width), Height: int(rect.Height)})
			}
		}
	})

	// ウィンドウが閉じられるときにPlugEditCloseをディスパッチする
	mainWindow.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		log.Println("Dispatching PlugEditClose")
		plugin.Dispatch(vst2.PluginOpcode(opcodes["PlugEditClose"]), 0, 0, nil, 0)
	})

	// MainWindowのイベントループを開始（ブロッキング）
	// これをゴルーチンで実行しないと、main関数がここでブロックされてしまう
	mainWindow.Run()
}
