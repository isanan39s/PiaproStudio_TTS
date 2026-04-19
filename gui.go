package main

import (
	"log"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"pipelined.dev/audio/vst2"
	"runtime"
	"sync/atomic"
	"time"
	//"unsafe"
)

type MyMainWindow struct {
	*walk.MainWindow
	childWin4Vst *walk.Composite
	windowRady   atomic.Bool
}

func UIthread(endchan chan struct{}, rsvchan chan MsgBus, sndchan chan MsgBus, vst *VSTHost) {

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mw := &MyMainWindow{}

	go func() {

		for rsvMsg := range rsvchan {
			switch rsvMsg.Cmd {
			case "GUI.close":
				mw.Synchronize(func() {
					mw.Close()
					///終了処理も呼ぶ
				})

			case "GUI.openGUI":
				// プラグインのロードとウィンドウの準備ができるまで待機します
				for !vst.isLoadedPlagin || !mw.windowRady.Load() {
					time.Sleep(50 * time.Millisecond)
				}

				sndchan <- MsgBus{
					To:   "main",
					From: "GUI",
					Cmd:  "vstloadrady",
				}

				// mw.Synchronize(func() {
				// 	println("GUIを開きます")
				// 	hwnd := uintptr(mw.childWin4Vst.Handle())
				// 	println("get hwnd=",hwnd)
				// 	vst.plugin.Dispatch(vst2.PluginOpcode(vst.opcodes["PlugEditOpen"]), 0, 0, unsafe.Pointer(hwnd), 0)
				// 	println("dispatchd!!!!")
				// })
				// 定期的にアイドル処理を呼ぶためのゴルーチン（GUIが開いている間）
				go func() {
					ticker := time.NewTicker(30 * time.Millisecond) // 少し間隔を広げて負荷を調整
					defer ticker.Stop()
					for range ticker.C {
						if mw.MainWindow == nil {
							return
						}
						mw.Synchronize(func() {
							// Piapro Studio などのプラグインは、この呼び出しによって内部のGUI更新を行います
							//vst.plugin.Dispatch(vst2.PluginOpcode(vst.opcodes["PlugEditIdle"]), 0, 0, nil, 0)
						})
					}
				}()
				println("GUIオープンコマンド送信完了")
			case "GUI.closeGUI":
				// flagsとか作ってGUIopen=falseしてもいいかも
				mw.Synchronize(func() {
					vst.plugin.Dispatch(vst2.PluginOpcode(vst.opcodes["PlugEditClose"]), 0, 0, nil, 0)
				})
			}

		}

	}()

	MainWindow{
		AssignTo: &mw.MainWindow,
		Title:    "title",
		Size:     Size{640, 480},
		Layout:   VBox{},

		MenuItems: []MenuItem{
			Menu{
				Text: "ファイル",
				Items: []MenuItem{
					Action{
						Text:        "終了",
						OnTriggered: func() { mw.Close() },
					},
				},
			},
		},

		Children: []Widget{
			Composite{
				AssignTo:        &mw.childWin4Vst,
				Layout:          nil,
				DoubleBuffering: true,
			},
		},
	}.Create()

	mw.Show()
	mw.windowRady.Store(true)
	sndchan <- MsgBus{
		To:   "main",
		From: "GUI",
		Cmd:  "GUI.WindowReady",
	}
	print("ウインドウよーい")
	err := mw.Run()
	print("ウインドウ止め")

	if err != 0 {
		print("異常あり")
		log.Fatal(err)
	} else {
		print("問題なし")
	}
	print("ウインドウ止めた")
	close(endchan)
}
