package main

import (
	"bufio"
	"encoding/binary"
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


var isTransportPlaying bool
// playRealtime はプラグインからのオーディオを処理し、オーディオデバイスで直接再生します。
func playRealtime(plugin *vst2.Plugin, duration time.Duration) error {
	const (
		sampleRate   = 48000
		channelCount = 2
		format       = oto.FormatSignedInt16LE // 16bit整数深度に相当
	)

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



		// グローバルな再生中フラグをONにする
		isTransportPlaying = true
		// このゴルーチン終了時に必ずOFFにする
		defer func() { isTransportPlaying = false }()
		vstBufferSize:= int(hostCallback(vst2.HostGetBufferSize,0,0,nil,0))
		remainingSamples := int(duration.Seconds() * sampleRate)
		for remainingSamples > 0 {
			///準備
			samplesToProcess := int(hostCallback(vst2.HostGetBufferSize,0,0,nil,0))
			if samplesToProcess > remainingSamples {
				samplesToProcess = remainingSamples
			}

			in := vst2.NewFloatBuffer(channelCount, samplesToProcess)
			out := vst2.NewFloatBuffer(channelCount, samplesToProcess)
			plugin.ProcessFloat(in, out)

///取得＆変換

			// floatサンプルを16bit PCMのバイトストリームに変換
			buf := make([]byte, samplesToProcess*channelCount*2) // 16bitは2バイト
			for i := 0; i < samplesToProcess; i++ {
				for c := 0; c < channelCount; c++ {
					sample := out.Channel(c)[i]
					sampleInt := int16(sample * 32767.0)
					binary.LittleEndian.PutUint16(buf[(i*channelCount+c)*2:], uint16(sampleInt))
				}
			}

			// PCMデータをパイプに書き込む　再生
			if _, err := pw.Write(buf); err != nil {
				errChan <- fmt.Errorf("failed to write to audio pipe: %w", err)
				return
			}

			///あと化ts付
			in.Free()
			out.Free()
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
	host2vstiMessageChan := make(chan string)

	pluginPath := "c:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll"
	var savePath, loadPath, outputWavPath string
	var openGUI bool
	duration := 5 * time.Second

	// 引数処理
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
		case "--output-wav":
			if i+1 < len(os.Args) {
				outputWavPath = os.Args[i+1]
				i++ // consume value
			} else {
				log.Fatal("--output-wav requires a file path")
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
	time.Sleep(400 * time.Millisecond)

	go vstiPlaginRunner(host2vstiMessageChan, vst, plugin, opcodes)
	time.Sleep(400 * time.Millisecond)

	/// fxb投入

	/// fxb投入
	if loadPath != "" {
		var massage_source = []string{"loadFXB", loadPath}
		host2vstiMessageChan <- strings.Join(massage_source, ":")
		println("send msg2vsti-therad",massage_source)
	}

	time.Sleep(400 * time.Millisecond)

	/// ウィンドウ召喚
	if openGUI {
		host2vstiMessageChan <- "openGUI"
		println("send msg2vsti-therad \"openGUI\"")
	}

	println("enter to save parmetors")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	/// fxb出力 Enterで
	if savePath != "" {
		var massage_source = []string{"saveFXB", savePath}
		host2vstiMessageChan <- strings.Join(massage_source, ":")
		println("send msg2vsti-therad",massage_source)
	}

	println("enter to save wave")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	// Process and save WAV if requested
	if outputWavPath != "" {
		// Send a message to the plugin runner goroutine to handle WAV processing safely.
		var massage_source= fmt.Sprintf("processWAV:%s:%s", duration.String(), outputWavPath)
		host2vstiMessageChan <- massage_source
		println("send msg2vsti-therad",massage_source)
	}

	host2vstiMessageChan <- "vstiexit"
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Program finished successfully.")

}