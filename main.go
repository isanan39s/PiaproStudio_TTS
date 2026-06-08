package main

import (
	"net/http"
	"os"
)

func main() {
	// メッセージバスの初期化
	msgChan := make(chan MsgBus, 100)
	endChan := make(chan struct{})
	bus := BusHQ(msgChan, endChan)

	go opjt_main(bus, "")

	mux := http.NewServeMux()

	apiserver := &APIserver{
		toBus : make(chan MsgBus, 100),
		bus:bus,
	}


	bus.registAddr("api", apiserver.toBus)
	mux.HandleFunc("/",apiserver.entry)

	go http.ListenAndServe(":8080", mux)

	// GUIの初期化 (ウィンドウ作成)
	mw := NewGUI(bus)
	println("inited window")
	// VSTホストの初期化
	NewVstHost(bus, mw.Synchronize)
	println("inited host")

	// メインループの開始 (ウィンドウが閉じられるまでブロック)
	mw.Run()

	// ウィンドウが閉じられた後、プロセスを確実に終了させる
	os.Exit(0)
}
