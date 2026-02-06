package main

import (
	// "bufio"
	// "fmt"
	// "io" // io.Pipeのために追加
	"log"
	"os"
	"strconv"
	"strings"
	// "time"

	// "github.com/hajimehoshi/oto/v2" // otoライブラリのために追加
	"pipelined.dev/audio/vst2"
)

var (
	hostCurrentSample  int64         // ホストコールバック用のグローバルサンプルカウンター
	hostTimeInfo       vst2.TimeInfo // プラグインに安定したポインタを渡すためのグローバルなTimeInfo構造体
	isTransportPlaying bool          // トランスポートの状態を制御するグローバルフラグ
)

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

	_=savePath
	_=loadPath
	_=openGUI


	endchan:=make(chan struct{})

	go func() {
		
		UIthread(endchan)
	}()

	<-endchan
	
}
