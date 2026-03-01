package main

import (
	"log"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"runtime"
	// "pipelined.dev/audio/vst2"
)

type MyMainWindow struct {
	*walk.MainWindow
	childWin4Vst *walk.Composite
}

func UIthread(endchan chan struct{}, rsvchan chan MsgBus,snd) {

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
			}

		}

	}()

	if _, err := (MainWindow{
		AssignTo: &mw.MainWindow,
		Title:    "title",
		//size layout
		Size:   Size{640, 480},
		Layout: VBox{},


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
	}).Run(); err != nil {
		log.Fatal(err)
	}

	close(endchan)
}
