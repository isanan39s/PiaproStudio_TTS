package main

import (
	"log"

	"runtime"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	// "pipelined.dev/audio/vst2"
)

type MyMainWindow struct{
	*walk.MainWindow

}


func UIthread(endchan chan struct{},msgchan chan MsgBus){
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mw:=&MyMainWindow{}


	if _,err:=(MainWindow{
		AssignTo: &mw.MainWindow,
		Title: "title",
		//size layout
		Size: Size{640,480},
		Layout: VBox{},

		Children: []Widget{
			Label{
				Text: "test",
			},
		},
	}).Run();err!=nil{log.Fatal(err)}
	



		close(endchan)
}


