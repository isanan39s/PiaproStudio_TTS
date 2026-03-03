package main

import (
	"log"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"pipelined.dev/audio/vst2"
	"runtime"
	"sync/atomic"
	"unsafe"
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
				// flagsとか作ってGUIopen=trueしてもいいかも
				for vst.isLoadedPlagin == false || mw.windowRady.Load() == false {
				}
				println("OK!")
				mw.Synchronize(func() {
					vst.plugin.Dispatch(vst2.PluginOpcode(vst.opcodes["PlugEditOpen"]), 0, 0, unsafe.Pointer(uintptr(mw.childWin4Vst.Handle())), 0)
				})
			case "GUI.closeGUI":
				// flagsとか作ってGUIopen=trueしてもいいかも
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
	if err != 0 {
		log.Fatal(err)
	}
	print("ウインドウ止め")
	close(endchan)
}
