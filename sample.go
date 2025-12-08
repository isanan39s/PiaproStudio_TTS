package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"unsafe"

	"pipelined.dev/audio/vst2"
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

func main() {
	var pluginPath string
	pluginPath = os.Args[1]
	if len(os.Args) < 2 || os.Args[1] == "--gui" {
		pluginPath = "c:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll" // デフォルトのプラグインパス
	}

	vst, plugin, opcodes, err := loadPlagin(pluginPath)
	if err != nil {
		log.Fatalf("failed to load plugin: %v", err)
	}
	defer vst.Close()
	defer plugin.Close()

	openGUI := false
	if os.Args[len(os.Args)-1] == "--gui" {
		openGUI = true
	}

	// バンクファイルが指定されていれば読み込み（--gui フラグと独立して処理）
	if len(os.Args) >= 3 && os.Args[2] != "" && os.Args[2] != "--gui" {
		println("setting .fbx")
		bankPath := os.Args[2]
		data, err := ioutil.ReadFile(bankPath)
		if err != nil {
			log.Fatalf(" バンクファイルの読み込みに失敗しました: %v", err)
		}

		// バンクをセットする前に plugin を開始する
		plugin.Start()
		plugin.SetBankData(data)
		println(" バンクをセットしました:", bankPath, "size", len(data))
	}

	if openGUI {
		// win32.go の関数（OpenPluginGUIWithWindow）を呼ぶ
		if err := OpenPluginGUIWithWindow(plugin, opcodes); err != nil {
			log.Fatalf("failed to open plugin GUI: %v", err)
		}
	}

	fmt.Println("プログラムを正常に終了します。")
}
