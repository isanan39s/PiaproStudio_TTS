package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"pipelined.dev/audio/vst2"
)

// デバッグ版 hostCallback: どの opcode でクラッシュするか特定用
func hostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
	// This callback is noisy, so we'll comment it out for now.
	// fmt.Printf("[hostCallback] opcode=%v (%d) index=%d value=%d\n", op, op, index, value)

	switch op {
	case vst2.HostGetVendorVersion:
		return 10
	case vst2.HostGetSampleRate:
		return int64(48000)
	case vst2.HostGetBufferSize:
		return int64(512)
	case vst2.HostGetCurrentProcessLevel:
		return int64(0)
	case vst2.HostGetTime:
		return 0
	case vst2.HostCanDo:
		return 0
	case vst2.HostOpcode(6): // hostWantMidi (opcode 6)
		return 1
	case vst2.HostGetVendorString, vst2.HostGetProductString:
		return 0
	case vst2.HostIdle:
		return 0
	case vst2.HostSizeWindow:
		return 0
	default:
		// デバッグ: 予期しない opcode をログ出力
		// fmt.Printf("[hostCallback] ⚠️ UNHANDLED opcode=%v (%d)\n", op, op)
		return 0
	}
}

func loadPlagin(path string) (*vst2.VST, *vst2.Plugin, map[string]int, error) {
	fmt.Printf(" VST2 プラグインをロード中: %s\n", path)

	vst, err := vst2.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}

	hostCallbackFunc := hostCallback
	plugin := vst.Plugin(hostCallbackFunc)
	if plugin == nil {
		return nil, nil, nil, fmt.Errorf("plugin instance creation failed")
	}

	name := vst.Name
	numParams := plugin.NumParams()
	var opcodes map[string]int = make(map[string]int)

	// opcode マップ構築とベンダー取得
	vendor := "unknown"
	for i := 0; i < 6000; i++ {
		opcodes[vst2.PluginOpcode(i).String()] = i
		if vst2.PluginOpcode(i).String() == "plugGetVendorString" || vst2.PluginOpcode(i).String() == "PlugGetVendorString" {
			var buf [1024]byte
			plugin.Dispatch(vst2.PluginOpcode(i), 0, 0, unsafe.Pointer(&buf[0]), 0)
			vendor = string(bytes.TrimRight(buf[:], "\x00"))
			break
		}
	}

	fmt.Println("---------------------------------------")
	fmt.Printf(" ロード成功。プラグイン情報を取得しました:\n")
	fmt.Printf("   プラグイン名: %s\n", name)
	fmt.Printf("   ベンダー名: %s\n", vendor)
	fmt.Printf("   パラメータ数: %d\n", numParams)
	fmt.Println("opcode :", opcodes)
	fmt.Println("---------------------------------------")

	if numParams > 0 {
		fmt.Println("パラメータ一覧:")
		for i := 0; i < numParams; i++ {
			fmt.Printf("  %d: %s\n", i, plugin.ParamName(i))
		}
	}
	return vst, plugin, opcodes, nil
}

// SaveFXB saves the plugin's state to an FXB file.
func SaveFXB(plugin *vst2.Plugin, path string) error {
	plugin.Start()
	data := plugin.GetBankData()
	plugin.Suspend()

	if data == nil {
		return fmt.Errorf("failed to get plugin bank data")
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write fxb file: %w", err)
	}

	fmt.Printf("Plugin state saved to %s\n", path)
	return nil
}

func processAndSaveWav(plugin *vst2.Plugin, path string, duration time.Duration) error {
	const (
		sampleRate = 48000
		channels   = 2
		bitDepth   = 16
		bufferSize = 512
	)

	// Create output file
	outFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create WAV encoder
	encoder := wav.NewEncoder(outFile, sampleRate, bitDepth, channels, 1) // 1 for PCM

	// Create audio buffer
	numSamples := int(duration.Seconds() * sampleRate)
	intBuf := &audio.IntBuffer{
		Format: &audio.Format{
			NumChannels: channels,
			SampleRate:  sampleRate,
		},
		Data:           make([]int, 0, numSamples*channels),
		SourceBitDepth: bitDepth,
	}

	// Start plugin
	plugin.SetSampleRate(sampleRate)
	plugin.SetBufferSize(bufferSize)
	plugin.Start()
	defer plugin.Suspend()

	// Send a MIDI note-on event to trigger sound
	// MIDI Note On: channel 1, note C4 (60), velocity 100
	noteOn := vst2.MIDIEvent{
		DeltaFrames: 0,
		Data:        [3]byte{0x90, 60, 100},
	}
	events := vst2.Events(&noteOn)
	defer events.Free()
	plugin.Dispatch(vst2.PlugProcessEvents, 0, 0, unsafe.Pointer(events), 0)

	// Process audio
	fmt.Printf("Processing %.2f seconds of audio...\n", duration.Seconds())
	remainingSamples := numSamples
	for remainingSamples > 0 {
		samplesToProcess := bufferSize
		if samplesToProcess > remainingSamples {
			samplesToProcess = remainingSamples
		}

		// Create VST buffers
		in := vst2.NewFloatBuffer(channels, samplesToProcess)
		out := vst2.NewFloatBuffer(channels, samplesToProcess)

		// Process audio
		plugin.ProcessFloat(in, out)

		// Convert and append to buffer
		for i := 0; i < samplesToProcess*channels; i++ {
			sample := out.Channel(i % channels)[i/channels]
			intBuf.Data = append(intBuf.Data, int(sample*32767.0))
		}

		in.Free()
		out.Free()
		remainingSamples -= samplesToProcess
	}

	// Write buffer to WAV file
	if err := encoder.Write(intBuf); err != nil {
		return fmt.Errorf("failed to write wav data: %w", err)
	}

	fmt.Printf("Audio successfully written to %s\n", path)
	return nil
}

func vstiPlaginRunner(host2vstiMessageChan chan string, vst *vst2.VST, plugin *vst2.Plugin, opcode map[string]int) {
	println("start plagin thead")
	is_openWindow := false
	var msg MSG
	for {
		
		if is_openWindow {
			// PeekMessage: ノンブロッキングでメッセージをチェック
			ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, PM_REMOVE)

			if ret > 0 {
				// メッセージがあれば処理
				if msg.Message == 0x0012 { // WM_QUIT
					break
				}
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			} else {
				// メッセージがなければ少し待機（CPU 負荷軽減）
				procSleep.Call(100)
			}

		}


		var msgFromHost []string
		// println("get msg")
		// if len(msgFromHost) <= 0 {
		// 	println("contenyu-")
		// 	continue
		// }

		select {
		case value, ok := <-host2vstiMessageChan:
			if ok {
				fmt.Println("値を取得しました:", value)
				msgFromHost = strings.SplitN(value, ":", 2)
			} else {
				fmt.Println("チャネルは閉じられています。ループ終了。")
				return // クローズされたらループを抜ける
			}
		default:
			// データがなかった場合、少し待機してから次のループへ
			//fmt.Println("データなし。少し待機してコンティニュー...")
			//time.Sleep(100 * time.Millisecond) // ここで意図的に待機
			continue
		}

		println("prosess msg")
		switch msgFromHost[0] {
		case "loadFXB":
			if len(msgFromHost) >= 2 && msgFromHost[1] != "" {
				fmt.Println("Loading .fxb:", msgFromHost[1])
				data, err := ioutil.ReadFile(msgFromHost[1])
				if err != nil {
					log.Fatalf("Failed to read bank file: %v", err)
				}
			time.Sleep(200*time.Millisecond)

				plugin.SetBankData(data)
			time.Sleep(200*time.Millisecond)

				fmt.Println("Bank set:", msgFromHost[1], "size", len(data))
				//plugin.Suspend() // Suspend after setting data if not opening GUI
			}

		case "openGUI":
			OpenPluginGUIWithWindow(plugin, opcode)
			is_openWindow = true
			time.Sleep(200*time.Millisecond)

		case "saveFXB":
			if err := SaveFXB(plugin, msgFromHost[1]); err != nil {
				log.Fatalf("Failed to save FXB file: %v", err)
			}
		}
	}

}

func main() {
	host2vstiMessageChan := make(chan string, 127)

	var pluginPath, savePath, loadPath, outputWavPath string
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

	if pluginPath == "" {
		pluginPath = "c:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll" // Default plugin path
	}

	vst, plugin, opcodes, err := loadPlagin(pluginPath)
	if err != nil {
		log.Fatalf("failed to load plugin: %v", err)
	}
	plugin.Start()
	defer vst.Close()
	defer plugin.Close()
			time.Sleep(200*time.Millisecond)
			time.Sleep(200*time.Millisecond)

	go vstiPlaginRunner(host2vstiMessageChan, vst, plugin, opcodes)
			time.Sleep(200*time.Millisecond)
			time.Sleep(200*time.Millisecond)

	/// fxb投入
	if loadPath != "" {
		var massage_source = []string{"loadFXB", loadPath}
		host2vstiMessageChan <- strings.Join(massage_source, ":")
		println(strings.Join(massage_source, ":"))
	}

	/// ウィンドウ召喚
	if openGUI {

		host2vstiMessageChan <- "openGUI"
		println("openGUI")
	}
	println("enter to save parmetors")

	/// fxb出力 Enterで
		bufio.NewReader(os.Stdin).ReadBytes('\n')

	if savePath != "" {

		var massage_source = []string{"saveFXB", loadPath}
		host2vstiMessageChan <- strings.Join(massage_source, ":")
		println(strings.Join(massage_source, ":"))
	}

	println("enter to save wave")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	// Process and save WAV if requested
	if outputWavPath != "" {
		if err := processAndSaveWav(plugin, outputWavPath, duration); err != nil {
			log.Fatalf("Failed to process and save WAV: %v", err)
		}
	}

	fmt.Println("Program finished successfully.")
}
