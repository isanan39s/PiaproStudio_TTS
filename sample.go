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
	msgchan := make(chan MsgBus)

	go func() { UIthread(UIendchan, msgchan) }()
	go func() { VSTPlaginThrad(VSTendchan, msgchan) }()

	<-UIendchan
	<-VSTendchan

}

type MsgBus struct {
	cmd    string
	option []string
}
