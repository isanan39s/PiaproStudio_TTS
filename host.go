package main

// import (
// 	"bytes"
// 	"fmt"
// 	"unsafe"
// 	"pipelined.dev/audio/vst2"
// )
// // timeInfo: LMMSの実装を参考に、より詳細なパラメータを設定した最終バージョン
// var timeInfo = &vst2.TimeInfo{
// 	SampleRate:         48000.0,
// 	Tempo:              120.0,
// 	PpqPos:             0.0, // Musical Position, in Quarter Note
// 	TimeSigNumerator:   4,   // 4/4拍子
// 	TimeSigDenominator: 4,
// 	Flags:              vst2.TempoValid | vst2.PpqPosValid | vst2.TimeSigValid,
// }
// func HostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
// 	return HostCallbackImpl(op, index, value, ptr, opt)
// }
// // HostCallbackImpl: LMMSの調査結果を反映した最終版
// func HostCallbackImpl(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
// 	// fmt.Printf("[hostCallback] opcode=%v (%d)\n", op, op)
// 	switch op {
// 	case vst2.HostGetVendorVersion:
// 		return 10
// 	case vst2.HostGetSampleRate:
// 		return int64(48000)
// 	case vst2.HostGetBufferSize:
// 		return int64(512)
// 	case vst2.HostGetCurrentProcessLevel:
// 		return int64(0)
// 	case vst2.HostGetTime:
// 		return int64(uintptr(unsafe.Pointer(timeInfo)))
// 	case vst2.HostCanDo:
// 		return 0
// 	case vst2.HostOpcode(6): // hostWantMidi
// 		return 1 // MIDIを受け付ける
// 	case vst2.HostOpcode(29): // audioMasterNeedIdle / HostNeedIdle
// 		return 1 // アイドル処理が必要であることを伝える
// 	case vst2.HostGetVendorString, vst2.HostGetProductString:
// 		return 0
// 	case vst2.HostIdle:
// 		return 0
// 	case vst2.HostSizeWindow:
// 		return 0
// 	default:
// 		// fmt.Printf("[hostCallback] ⚠️ UNHANDLED opcode=%v (%d)\n", op, op)
// 		return 0 // 不明なものは0を返すのが最も安全
// 	}
// }
// func LoadPlugin(path string) (*vst2.VST, *vst2.Plugin, map[string]int, error) {
// 	fmt.Printf(" VST2 プラグインをロード中: %s\n", path)
// 	vst, err := vst2.Open(path)
// 	if err != nil {
// 		return nil, nil, nil, err
// 	}
// 	hostCallbackFunc := HostCallback
// 	plugin := vst.Plugin(hostCallbackFunc)
// 	if plugin == nil {
// 		return nil, nil, nil, fmt.Errorf("plugin instance creation failed")
// 	}
// 	name := vst.Name
// 	numParams := plugin.NumParams()
// 	var opcodes map[string]int = make(map[string]int)
// 	// opcode マップ構築とベンダー取得
// 	vendor := "unknown"
// 	for i := 0; i < 6000; i++ {
// 		opcodes[vst2.PluginOpcode(i).String()] = i
// 		if vst2.PluginOpcode(i).String() == "plugGetVendorString" || vst2.PluginOpcode(i).String() == "PlugGetVendorString" {
// 			var buf [1024]byte
// 			plugin.Dispatch(vst2.PluginOpcode(i), 0, 0, unsafe.Pointer(&buf[0]), 0)
// 			vendor = string(bytes.TrimRight(buf[:], "\x00"))
// 			break
// 		}
// 	}
// 	fmt.Println("---------------------------------------")
// 	fmt.Printf(" ロード成功。プラグイン情報を取得しました:\n")
// 	fmt.Printf("   プラグイン名: %s\n", name)
// 	fmt.Printf("   ベンダー名: %s\n", vendor)
// 	fmt.Printf("   パラメータ数: %d\n", numParams)
// 	if numParams > 0 {
// 		fmt.Println("パラメータ一覧:")
// 		for i := 0; i < numParams; i++ {
// 			fmt.Printf("  %d: %s\n", i, plugin.ParamName(i))
// 		}
// 	}
// 	fmt.Println("   opcode :", opcodes)
// 	fmt.Println("---------------------------------------")
// 	return vst, plugin, opcodes, nil
// }
