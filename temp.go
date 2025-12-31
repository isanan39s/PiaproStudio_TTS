package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time" // for example GetTime

	"pipelined.dev/audio/vst2"
	"pipelined.dev/pipe"
	"pipelined.dev/audio/wav"
)

func main() {
	// --- Step 0: Prerequisites ---
	// 実行前に以下のコマンドで必要なモジュールをインストールしてください:
	// go get pipelined.dev/pipe
	// go get pipelined.dev/audio/vst2
	// go get pipelined.dev/wav

	// VSTプラグインのパスをコマンドライン引数から取得
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run temp.go <path_to_vst_plugin.dll_or_vst>")
		os.Exit(1)
	}
	pluginPath := os.Args[1]

	// --- Step 1: プラグインを開く ---
	fmt.Printf("Opening VST plugin: %s\n", pluginPath)
	plugin, err := vst2.Open(pluginPath)
	if err != nil {
		log.Fatalf("Failed to open VST plugin: %v", err)
	}
	// 関数終了時にプラグインのリソースを解放することを保証
	defer func() {
		fmt.Println("Closing VST plugin.")
		if closeErr := plugin.Close(); closeErr != nil {
			log.Printf("Error closing plugin: %v", closeErr)
		}
	}()
	fmt.Printf("Plugin opened: %s - %s (Version: %d)\n", plugin.Vendor, plugin.Name, plugin.Version)

	// --- Step 2: ホストを準備する ---
	// ホストコールバックの実装例
	customHost := vst2.Host{
		// プラグインからホストのテンポ/時間情報を問い合わせられたときに呼び出される
		GetTime: func() *vst2.TimeInfo {
			// これはダミーの実装です。
			// 実際のDAWホストでは、再生位置、テンポ、拍子などをリアルタイムで提供します。
			now := time.Now()
			// 簡略化のため、常に一定のテンポで再生中と仮定
			return &vst2.TimeInfo{
				Tempo:        120.0,
				TimeSigNum:   4,
				TimeSigDen:   4,
				SamplePos:    int64(now.UnixNano()/int64(time.Millisecond)) * 44.1, // ダミーのサンプル位置
				PpqPos:       float64(now.UnixNano()/int64(time.Millisecond)) * 0.002, // ダミーのPPQ位置
				Transporting: 1,          // 再生中
			}
		},
		// プラグインからホストの能力を問い合わせられたときに呼び出される
		CanDo: func(do string) int32 {
			fmt.Printf("Plugin asks if host can '%s'\n", do)
			switch do {
			case "sendVstEvents", "sendVstMidiEvent":
				// このホストはMIDIイベント送信をサポートすると仮定
				return vst2.HostCanDo.Yes
			case "receiveVstTimeInfo":
				// GetTimeを実装したので時間情報提供をサポート
				return vst2.HostCanDo.Yes
			default:
				return vst2.HostCanDo.No
			}
		},
		// その他の必要なコールバックをここで実装...
	}
	customHost.Init()
	// 関数終了時にホストのリソースを解放することを保証
	defer func() {
		fmt.Println("Closing host.")
		if closeErr := customHost.Close(); closeErr != nil {
			log.Printf("Error closing host: %v", closeErr)
		}
	}()
	fmt.Println("Host prepared.")

	// --- Step 3: プラグインの初期化 ---
	fmt.Println("Initializing VST plugin with host...")
	if err := plugin.Init(&customHost); err != nil {
		log.Fatalf("Failed to initialize VST plugin: %v", err)
	}
	fmt.Println("Plugin initialized.")

	// --- Step 4: (オプション) プラグインの事前設定 ---
	// パラメータの一覧を表示
	fmt.Println("\nAvailable Parameters:")
	for i, param := range plugin.Parameters() {
		fmt.Printf("  %d: %s (Value: %.2f, Display: %s)\n", i, param.Name, param.Value(), param.Display())
	}

	// 例: 最初のパラメータを特定の値に設定
	if plugin.NumParameters() > 0 {
		paramIndex := 0
		newValue := 0.75 // 0.0から1.0の範囲
		fmt.Printf("Setting parameter %d ('%s') to %.2f\n", paramIndex, plugin.Parameters()[paramIndex].Name, newValue)
		plugin.SetParameter(paramIndex, newValue)
		fmt.Printf("  New value: %.2f (Display: %s)\n", plugin.Parameters()[paramIndex].Value(), plugin.Parameters()[paramIndex].Display())
	}

	// チャンク (プリセット) の設定はここでは省略しますが、plugin.SetChunk() で可能です。
	// 例:
	// if chunkData, err := os.ReadFile("my_preset.fxb"); err == nil {
	//     plugin.SetChunk(chunkData, false) // falseはプログラムチャンク用
	//     fmt.Println("Loaded preset from my_preset.fxb")
	// }

	// --- Step 5: 音声処理を実行する (pipelined.dev/pipe を使用) ---
	fmt.Println("\nStarting audio processing via pipeline (reading from stdin, writing to stdout)...")
	fmt.Println("Please pipe WAV data to stdin (e.g., `cat input.wav | go run temp.go <plugin_path> > output.wav`)")

	p, err := pipe.New(
		1024, // オーディオバッファサイズ
		pipe.Line{
			Source: &wav.Source{
				Reader: os.Stdin, // 標準入力からWAVデータを読み込む
			},
			Processors: pipe.Processors(
				// plugin.Processor() がVst2プラグインをpipe.ProcessorFuncとしてラップ
				plugin.Processor(
					// ここで処理中のパラメータを動的に設定することも可能 (例)
					// vst2.Param("Gain", 0.5),
				),
			),
			Sink: &wav.Sink{
				Writer: os.Stdout, // 標準出力にWAVデータを書き出す
			},
		},
	)
	if err != nil {
		log.Fatalf("Failed to build pipe: %v", err)
	}

	// パイプラインを実行し、完了を待つ
	if err := pipe.Wait(p.Start(context.Background())); err != nil {
		log.Fatalf("Failed to execute pipe: %v", err)
	}

	fmt.Println("\nAudio processing complete.")
}
