package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"unsafe"

	"pipelined.dev/audio/vst2"
)

var timeInfo = &vst2.TimeInfo{
	SampleRate: 48000.0,
	Tempo:      120.0,
}

func HostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
	return hostCallback(op, index, value, ptr, opt)
}

// デバッグ版 hostCallback: どの opcode でクラッシュするか特定用
func hostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
	fmt.Printf("[hostCallback] opcode=%v (%d) index=%d value=%d ptr=%p opt=%f\n", op, op, index, value, ptr, opt)

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
		// To-Do: value引数でフィルタリングする
		return int64(uintptr(unsafe.Pointer(timeInfo)))
	case vst2.HostCanDo:
		return 0
	case vst2.HostOpcode(6): // hostWantMidi
		// このホストは MIDI を受け付けることを知らせる (1 = yes)
		return 1
	case vst2.HostGetVendorString, vst2.HostGetProductString:
		return 0
	case vst2.HostIdle:
		return 0
	case vst2.HostSizeWindow:
		return 0
	default:
		fmt.Printf("[hostCallback] ⚠️ UNHANDLED opcode=%v (%d) INDEX=%d VALUE=%d PTR=%p OPT=%f -- returning 1\n", op, op, index, value, ptr, opt)
		return 1
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
	if len(os.Args) < 2 {
		log.Println(" 使用法: go run . <VST2 プラグインのパス> [bank.fxb] [--gui]")
		return
	}

	pluginPath := os.Args[1]

	vst, plugin, opcodes, err := loadPlagin(pluginPath)
	if err != nil {
		log.Fatalf("failed to load plugin: %v", err)
	}
	defer vst.Close()
	defer plugin.Close()

	openGUI := false
	if len(os.Args) >= 3 && os.Args[len(os.Args)-1] == "--gui" {
		openGUI = true
		println("gui enable")
	}

	var exec chan ExecRequest // GUI スレッド向けの実行チャネル（なければ nil）
	if openGUI {
		// win32.go の関数（OpenPluginGUIWithWindow）を呼ぶ（非ブロッキング）
		fmt.Println("opengui")
		done,  err := OpenPluginGUIWithWindow(plugin, opcodes)
		if err != nil {
			log.Fatalf("failed to open plugin GUI: %v", err)
		}
		exec = e
		_ = done // 必要なら <-done で待てます
		fmt.Println("Plugin GUI started (non-blocking). Close the plugin window to finish; or press Enter to exit now.")
	}

	procSleep.Call(5000)

	// バンクファイルが指定されていれば読み込み（--gui フラグと独立して処理）
	if len(os.Args) >= 3 && os.Args[2] != "" && os.Args[2] != "--gui" {
		bankPath := os.Args[2]
		data, err := ioutil.ReadFile(bankPath)
		if err != nil {
			log.Fatalf(" バンクファイルの読み込みに失敗しました: %v", err)
		}

		// GUI スレッド上で SetBankData を実行する
		if exec != nil {
			resp := make(chan error, 1)
			exec <- ExecRequest{
				Fn: func() error {
					plugin.SetBankData(data)
					return nil
				},
				Resp: resp,
			}
			if err := <-resp; err != nil {
				log.Fatalf(" SetBankData failed: %v", err)
			}
			println(" バンクをセットしました:", bankPath, "size", len(data))
		} else {
			// exec が無ければ直接呼ぶ（GUI スレッドが存在しない場合）
			plugin.SetBankData(data)
			println(" バンクをセットしました(直接):", bankPath, "size", len(data))
		}
	}

	bufio.NewScanner(os.Stdin).Scan()

	fmt.Println("プログラムを正常に終了します。")
}
