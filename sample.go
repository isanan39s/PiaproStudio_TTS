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

func main() {
	var pluginPath, savePath, loadPath, outputWavPath string
	var openGUI bool
	duration := 5 * time.Second

	// Parse command line arguments
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

	// Load FXB if requested
	if loadPath != "" {
		fmt.Println("Loading .fxb:", loadPath)
		data, err := ioutil.ReadFile(loadPath)
		if err != nil {
			log.Fatalf("Failed to read bank file: %v", err)
		}
		plugin.SetBankData(data)
		fmt.Println("Bank set:", loadPath, "size", len(data))
		//plugin.Suspend() // Suspend after setting data if not opening GUI
	}

	if openGUI {
		if err := OpenPluginGUIWithWindow(plugin, opcodes); err != nil {
			log.Fatalf("failed to open plugin GUI: %v", err)
		}
	}
	println("enter to save parmetors")

	// Save FXB if requested (interactive)
	if savePath != "" {
		if err := SaveFXB(plugin, savePath); err != nil {
			log.Fatalf("Failed to save FXB file: %v", err)
		}
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
