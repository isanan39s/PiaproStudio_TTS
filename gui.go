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
}

func UIthread(endchan chan struct{}, msgchan chan MsgBus) {

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mw := &MyMainWindow{}

	go func() {

		for rsvMsg := range msgchan {
			switch rsvMsg.cmd {
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

		Children: []Widget{
			Label{
				Text: "test",
			},
		},
	}).Run(); err != nil {
		log.Fatal(err)
	}

	close(endchan)
}
