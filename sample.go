package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"pipelined.dev/audio/vst2"
	"strings"
	"unsafe"
)

// デバッグ版 hostCallback: どの opcode でクラッシュするか特定用
func hostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
	fmt.Printf("[hostCallback] opcode=%v (%d) index=%d value=%d\n", op, op, index, value)

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
		fmt.Printf("[hostCallback] ⚠️ UNHANDLED opcode=%v (%d)\n", op, op)
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

func main() {
	var pluginPath, savePath, loadPath string
	var openGUI bool

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
	defer vst.Close()
	defer plugin.Close()

	// Load FXB if requested
	if loadPath != "" {
		fmt.Println("Loading .fxb:", loadPath)
		data, err := ioutil.ReadFile(loadPath)
		if err != nil {
			log.Fatalf("Failed to read bank file: %v", err)
		}
		plugin.Start()
		plugin.SetBankData(data)
		fmt.Println("Bank set:", loadPath, "size", len(data))
		plugin.Suspend() // Suspend after setting data if not opening GUI
	}

	if openGUI {
		if err := OpenPluginGUIWithWindow(plugin, opcodes); err != nil {
			log.Fatalf("failed to open plugin GUI: %v", err)
		}
	}

	// Save FXB if requested
	if savePath != "" {
		println("enter to save parmetors")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		if err := SaveFXB(plugin, savePath); err != nil {
			log.Fatalf("Failed to save FXB file: %v", err)
		}
	}

	fmt.Println("Program finished successfully.")
}
