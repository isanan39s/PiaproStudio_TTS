package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/oto/v2"
	"pipelined.dev/audio/vst2"
)

var (
	hostCurrentSample int64         // Global sample counter for the host callback.
	hostTimeInfo      vst2.TimeInfo // Global TimeInfo struct to ensure a stable pointer is passed to the plugin.
)

////intにしてchan通す

var isTransportPlaying bool

// playRealtime はプラグインからのオーディオを処理し、オーディオデバイスで直接再生します。
func playRealtime(plugin *vst2.Plugin, duration time.Duration, buf_chan chan []byte, end chan bool) error {
	const (
		sampleRate   = 48000
		channelCount = 2
		format       = oto.FormatSignedInt16LE // 16bit整数深度に相当
	)

	// グローバルな再生中フラグをONにする
	isTransportPlaying = true
	// この関数終了時に必ずOFFにする
	defer func() { isTransportPlaying = false }()

	// 1. otoのコンテキストをセットアップ
	// v2 APIではNewContext(sampleRate, channelCount, format)を使用
	otoCtx, readyChan, err := oto.NewContext(sampleRate, channelCount, format)
	if err != nil {
		return fmt.Errorf("oto.NewContext failed: %w", err)
	}
	// オーディオドライバの準備が完了するまで待機
	<-readyChan

	// 2. VSTループからプレイヤーへデータをストリーミングするためのパイプを作成
	pr, pw := io.Pipe()

	// 3. パイプからデータを読み込むプレイヤーを作成し、再生を開始
	player := otoCtx.NewPlayer(pr)
	defer player.Close()
	player.Play()

	// 4. VST処理を別のゴルーチンで実行し、パイプに書き込む
	errChan := make(chan error, 1)
	go func() {
		// このゴルーチンが終了する際に、パイプの書き込み側を閉じてプレイヤーにEOFを通知
		defer pw.Close()
		// プラグインからのパニックをキャッチするためのrecover
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("PANIC during VST processing: %v", r)
			}
		}()

		vstBufferSize := int(hostCallback(vst2.HostGetBufferSize, 0, 0, nil, 0))
		remainingSamples := int(duration.Seconds() * sampleRate)
		//for remainingSamples > 0 {
		for !<-end {
			///準備
			samplesToProcess := int(hostCallback(vst2.HostGetBufferSize, 0, 0, nil, 0))
			if samplesToProcess > remainingSamples {
				samplesToProcess = remainingSamples
			}

			///取得＆変換

			// floatサンプルを16bit PCMのバイトストリームに変換
			buf := make([]byte, samplesToProcess*channelCount*2) // 16bitは2バイト

			buf = <-buf_chan

			// PCMデータをパイプに書き込む　再生
			if _, err := pw.Write(buf); err != nil {
				errChan <- fmt.Errorf("failed to write to audio pipe: %w", err)
				return
			}

			///あと化ts付

			remainingSamples -= samplesToProcess
			hostCurrentSample += int64(samplesToProcess)
		}

		// isTransportPlayingは、この関数のdeferによって、この後の処理の前にfalseになる
		// フラッシング用のループ
		// isTransportPlaying = false
		for i := 0; i < 10; i++ {
			in := vst2.NewFloatBuffer(channelCount, vstBufferSize)
			out := vst2.NewFloatBuffer(channelCount, vstBufferSize)
			plugin.ProcessFloat(in, out)
			in.Free()
			out.Free()
			hostCurrentSample += int64(vstBufferSize)
		}
	}()

	// 5. プレイヤーの再生が完了するのを待機し、ゴルーチンからのエラーもチェックする
	for player.IsPlaying() {
		select {
		case err := <-errChan:
			return err // 処理ゴルーチンからのエラーを返す
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// プレイヤー停止後に最終的なエラーがないか確認
	select {
	case err := <-errChan:
		return err
	default:
		println("[playRealtime] Playback finished successfully.")
		return nil
	}
}


func main() {
	//host2vstiMessageChan := make(chan string) // UI/メッセージチャネル (使われなくなる)
	// buf_chanのバッファサイズは、otoが処理する量に応じて調整可能
	buf_chan := make(chan []byte, 10) // VSTゴルーチンとotoゴルーチンを繋ぐバッファ (例: 10ブロック分)

	pluginPath := "c:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll"
	var savePath, loadPath string
	var openGUI bool
	duration := 5 * time.Second // デフォルトの再生時間

	// 引数処理 (変更なし)
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--save-fxb":
			if i+1 < len(os.Args) {
				savePath = os.Args[i+1]
				i++ // consume value
			} else {
				log.Fatal("--save-fxb requires a file path")
			}
		case "--load-fxb":
			if i+1 < len(os.Args) {
				loadPath = os.Args[i+1]
				i++ // consume value
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
				i++ // consume value
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

	// VST処理ゴルーチンへの開始指示チャンネル
	startProcessing := make(chan struct{})
	// vstiPlaginRunnerは、メッセージチャネルを直接扱わず、純粋にオーディオを生成
	go vstiPlaginRunner(vst, plugin, opcodes, buf_chan, startProcessing)
	time.Sleep(400 * time.Millisecond) // ゴルーチン起動後の待機

	// --- VSTプラグインの初期化と設定をメインゴルーチンで直接実行 ---
	// loadFXB (mainで直接実行)
	if loadPath != "" {
		println("Loading .fxb:", loadPath)
		if err := loadFXB(plugin, loadPath); err != nil {
			log.Fatalf("Failed to load FXB file: %v", err)
		}
		println("Bank set:", loadPath)
	}
	time.Sleep(400 * time.Millisecond) // 処理間の待機

	// openGUI (mainで直接実行)
	if openGUI {
		println("Opening GUI...")
		OpenPluginGUIWithWindow(plugin, opcodes)
		// GUIウィンドウが閉じられるのを待つか、一定時間後に閉じるかなどのロジックが必要
		// 今は単純に待機
		println("GUIウィンドウを閉じるとプログラムが進行します。")
		// GUIが閉じられたらCloseEditorを呼ぶ必要がある
		println("Dispatch: PlugEditClose (15) param1=0 param2=0")
		plugin.Dispatch(vst2.PluginOpcode(opcodes["PlugEditClose"]), 0, 0, nil, 0)
	}

	println("enter to save parameters (not implemented in this flow)")
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	// saveFXB (mainで直接実行)
	if savePath != "" {
		println("Saving FXB:", savePath)
		if err := SaveFXB(plugin, savePath); err != nil {
			log.Fatalf("Failed to save FXB file: %v", err)
		}
		println("FXB saved:", savePath)
	}

	println("enter to start real-time audio playback")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	
	end := make(chan bool) // playRealtimeへの終了通知チャネル
	close(startProcessing) // vstiPlaginRunnerにオーディオ処理開始を指示

	// リアルタイム再生の開始
	println("Starting real-time playback...")
	if err := playRealtime(duration, buf_chan, end); err != nil {
		log.Fatalf("リアルタイム再生に失敗しました: %v", err)
	}
	
	// 再生が終了したらvstiPlaginRunnerを停止
	// host2vstiMessageChan <- "vstiexit" // これも不要になる
	close(buf_chan) // vstiPlaginRunnerに終了を通知
	println("send msg2vsti-therad vstiexit") // これはデバッグログ。実際は終了している
	time.Sleep(500 * time.Millisecond) // クリーンアップのための少しの待機

	fmt.Println("Program finished successfully.")
}
