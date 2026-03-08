package main

import (
	"log"
	"os"
	//"pipelined.dev/audio/vst2"
	"strconv"
	"strings"
)

// var (
// 	hostCurrentSample int64         // ホストコールバック用のグローバルサンプルカウンター
// 	hostTimeInfo      vst2.TimeInfo // プラグインに安定したポインタを渡すためのグローバルなTimeInfo構造体
// )

func main() {
	pluginPath := "c:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll"
	var savePath, loadPath string
	var openGUI bool
	vsthost := &VSTHost{}

	// 引数処理
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--save-fxb":
			if i+1 < len(os.Args) {
				savePath = os.Args[i+1]
				i++ // 値を消費
			} else {
				log.Fatal("--save-fxb requires a file path")
			}
		case "--load-fxb":
			if i+1 < len(os.Args) {
				loadPath = os.Args[i+1]
				i++ // 値を消費
			} else {
				log.Fatal("--load-fxb requires a file path")
			}
		case "--duration":
			if i+1 < len(os.Args) {
				_, err := strconv.Atoi(os.Args[i+1])
				if err != nil {
					log.Fatalf("invalid duration: %v", err)
				}
				// duration = time.Duration(d) * time.Second
				i++ // 値を消費
			} else {
				log.Fatal("--duration requires a number of seconds")
			}
		case "--gui":
			openGUI = true
		default:
			if !strings.HasPrefix(arg, "--") && pluginPath == "" {
				pluginPath = arg
			}
		}
	}

	_ = savePath
	_ = loadPath
	_ = openGUI
	/*

		トド岩:色々準備

	*/

	UIendchan := make(chan struct{})
	VSTendchan := make(chan struct{})
	endchan := make(chan struct{})

	msgchan := make(chan MsgBus) //　スレからHQ
	hq := BusHQ(msgchan, endchan)

	UIChan := make(chan MsgBus)  // HQからスレ
	VSTChan := make(chan MsgBus) // HQからスレ
	mainChan := make(chan MsgBus)
	hq.registAddr("GUI", UIChan)
	hq.registAddr("VSTiTh", VSTChan)
	hq.registAddr("main", mainChan)

	println("aaaaa")

	go func() { UIthread(UIendchan, UIChan, msgchan, vsthost) }()
	go func() { vsthost.VSTPlaginThrad(VSTendchan, VSTChan, msgchan) }()

	tmp:=<-mainChan //window待機 いい感じに受信処理したい
	println(tmp.Cmd,tmp.From)
	msgchan <- MsgBus{
		To:   "VSTiTh",
		From: "main",
		Cmd:  "VSTiTh.loadPlugin",
		Option: []string{
			pluginPath,
		},
	}

	msgchan <- MsgBus{
		To:   "GUI",
		From: "main",
		Cmd:  "GUI.openGUI",
	}

	<-VSTendchan
	<-UIendchan

}
