package main

import "os"

func main() {
	// メッセージバスの初期化
	msgChan := make(chan MsgBus, 100)
	endChan := make(chan struct{})
	bus := BusHQ(msgChan, endChan)

	// GUIの初期化 (ウィンドウ作成)
	mw := NewGUI(bus)

	// VSTホストの初期化
	NewVstHost(bus, mw.Synchronize)

	// メインループの開始 (ウィンドウが閉じられるまでブロック)
	mw.Run()

	// ウィンドウが閉じられた後、プロセスを確実に終了させる
	os.Exit(0)
}
