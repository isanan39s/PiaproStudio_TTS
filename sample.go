package main

import (
	"bufio"
	"fmt"
	"io" // io.Pipeのために追加
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/oto/v2" // otoライブラリのために追加
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
	duration := 5 * time.Second // デフォルトの再生時間

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
				d, err := strconv.Atoi(os.Args[i+1])
				if err != nil {
					log.Fatalf("invalid duration: %v", err)
				}
				duration = time.Duration(d) * time.Second
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

	vst, plugin, opcodes, err := loadPlagin(pluginPath)
	if err != nil {
		log.Fatalf("failed to load plugin: %v", err)
	}
	defer vst.Close()
	defer plugin.Close()
	time.Sleep(400 * time.Millisecond) // プラグインロード後の待機

	// --- パイプと oto プレイヤーのセットアップ (コンシューマー側) ---
	pr, pw := io.Pipe()

	otoCtx, readyChan, err := oto.NewContext(48000, 2, oto.FormatSignedInt16LE)
	if err != nil {
		log.Fatalf("Failed to create oto context: %v", err)
	}
	println("please wait1")
	<-readyChan // オーディオドライバの準備が完了するまで待機

	player := otoCtx.NewPlayer(pr)
	defer player.Close()
	println("please wait4")

	println("please wait3")

	// VST処理ゴルーチンへの開始指示チャンネル
	startProcessing := make(chan struct{})
	host2vstiMessageChan := make(chan string)
	println("please wait2")

	// --- VST処理ゴルーチンの起動 (プロデューサー側) ---
	go vstiPlaginRunner(vst, plugin, opcodes, pw, startProcessing, host2vstiMessageChan)
	println("gorutin started")
	time.Sleep(400 * time.Millisecond) // ゴルーチン起動後の待機

	// --- VSTプラグインの初期化と設定をメインゴルーチンで直接実行 ---
	// loadFXB
	if loadPath != "" {
		var massage_source = []string{"loadFXB", loadPath}
		host2vstiMessageChan <- strings.Join(massage_source, ":")
		println("send msg2vsti-therad", massage_source)
	}

	time.Sleep(400 * time.Millisecond)
	close(startProcessing)

	/// ウィンドウ召喚
	if openGUI {
		println("openGUI")
		host2vstiMessageChan <- "openGUI"
		println("send msg2vsti-therad \"openGUI\"")
	}

	println("enter to save parmetors")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	/// fxb出力 Enterで
	if savePath != "" {
		var massage_source = []string{"saveFXB", savePath}
		host2vstiMessageChan <- strings.Join(massage_source, ":")
		println("send msg2vsti-therad", massage_source)
	}

	println("enter to start real-time audio processing")
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	// グローバルな再生中フラグをONにする
	isTransportPlaying = true
	defer func() { isTransportPlaying = false }() // main関数終了時にOFFにする

	// vstiPlaginRunnerにオーディオ処理開始を指示
	player.Play() // プレイヤーの再生を開始

	// リアルタイム再生の実行と待機
	println("Starting real-time playback for", duration)
	time.AfterFunc(duration, func() {
		pw.Close() // 指定時間経過後、プロデューサー側のパイプを閉じる
		println("Playback duration reached, closing producer pipe.")
	})

	// プレイヤーの再生が完了するのを待機
	for player.IsPlaying() {
		time.Sleep(10 * time.Millisecond)
	}
	println("[main] Playback finished successfully.")

	fmt.Println("Program finished successfully.")
}
