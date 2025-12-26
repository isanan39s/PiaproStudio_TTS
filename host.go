package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	//"log"
	//"strings"
	//"time"
	"unsafe"

	"pipelined.dev/audio/vst2"
)

// / vstiからの問い合わせに対する応答
// デバッグ版 hostCallback: どの opcode でクラッシュするか特定用
func hostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
	fmt.Printf("[hostCallback] opcode=%v (%d) index=%d value=%d\n", op, op, index, value)
	switch op {
	case vst2.HostGetVendorVersion:
		return 10
	case vst2.HostGetSampleRate:
		return int64(48000)
	case vst2.HostGetBufferSize:
		return int64(1024) // バッファサイズを512から1024に変更
	case vst2.HostGetCurrentProcessLevel:
		return int64(0)
	case vst2.HostGetTime:
		const (
			sampleRate = 48000.0
			tempo      = 120.0
		)
		// プラグインに詳細なTimeInfo構造体を渡す
		// Piapro Studioが正しく機能するためには、音楽的な文脈（テンポ、拍子、小節）が必要
		samplesPerBeat := (sampleRate * 60.0) / tempo
		hostTimeInfo = vst2.TimeInfo{
			SamplePos:          float64(hostCurrentSample),
			SampleRate:         sampleRate,
			Tempo:              tempo,
			BarStartPos:        0.0,                                         // ダミー値：最初の小節にいると仮定
			PpqPos:             float64(hostCurrentSample) / samplesPerBeat, // 四分音符単位での音楽的な位置
			TimeSigNumerator:   4,                                           // ダミー値：4/4拍子
			TimeSigDenominator: 4,                                           // ダミー値：4/4拍子
			Flags: vst2.TransportPlaying |
				vst2.TempoValid |
				vst2.PpqPosValid |
				vst2.TimeSigValid |
				vst2.BarsValid,
		}
		return int64(uintptr(unsafe.Pointer(&hostTimeInfo)))
	case vst2.HostOpcode(6): // hostWantMidi (opcode 6)
		return 1
	case vst2.HostGetVendorString, vst2.HostGetProductString:
		return 0
	case vst2.HostIdle:
		return 0
	case vst2.HostSizeWindow:
		return 0
	default:
		fmt.Printf("[hostCallback] ⚠️ UNHANDLED opcode=%v (%d)\n", op, op) /// 例外対応
		return 0
	}
}

// /vsti(dll)を読み込み、起動準備と窓口を提供する
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
			plugin.Dispatch(vst2.PluginOpcode(i), 0, 0, unsafe.Pointer(&buf[0]), 0) ///opcodeを用いた操作の例
			vendor = string(bytes.TrimRight(buf[:], "\x00"))
			break
		}
	}

	fmt.Println("---------------------------------------")
	fmt.Printf(" ロード成功。プラグイン情報を取得しました:\n")
	fmt.Printf("   プラグイン名: %s\n", name)
	fmt.Printf("   ベンダー名: %s\n", vendor)
	fmt.Printf("   パラメータ数: %d\n", numParams)
	fmt.Println("   opcode :", opcodes)
	fmt.Println("---------------------------------------")

	if numParams > 0 {
		fmt.Println("パラメータ一覧:")
		for i := 0; i < numParams; i++ {
			fmt.Printf("  %d: %s\n", i, plugin.ParamName(i))
		}
	}
	return vst, plugin, opcodes, nil
}

// / .fxbからパラメータ(歌詞音階調声その他)を読み込み
// / vstiPlaginRunnerからの呼出専用
func loadFXB(plugin *vst2.Plugin, path string) error {
	fmt.Println("Loading .fxb:", path)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Failed to read bank file: %v", err)
	}

	plugin.SetBankData(data) ///本体 あとはファイルからの読み出し

	fmt.Println("Bank set:", path, "size", len(data))
	return nil
}

// / .fxbにパラメータ(歌詞音階調声その他)を保存
// / vstiPlaginRunnerからの呼出専用
func SaveFXB(plugin *vst2.Plugin, path string) error {
	// プラグインのライフサイクル（Start/Suspend）はvstiPlaginRunnerで管理されます。
	data := plugin.GetBankData() ///これ本体 あとはエラーチェックと書き込み

	if data == nil {
		return fmt.Errorf("failed to get plugin bank data")
	}
	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write fxb file: %w", err)
	}

	fmt.Printf("Plugin state saved to %s\n", path)
	return nil
}

func vstiPlaginRunner(vst *vst2.VST, plugin *vst2.Plugin, opcode map[string]int, buf_chan chan []byte, startProcessing chan struct{}) {
	println("start plagin thead")
	
	// Pluginをいつでも動かせるように
	println("VST-DISPATCH-ENTRY: cmd=0 (plugOpen)")
	plugin.Start() // Corresponds to plugOpen (0)
	println("VST-DISPATCH-EXIT: cmd=0 result=0") // VstPlugin.cppのVstPlugin::open()の戻り値を模倣

	println("VST-DISPATCH-ENTRY: cmd=12 (plugStateChanged)")
	plugin.Dispatch(vst2.PluginOpcode(opcode["plugStateChanged"]), 0, 0, nil, 0) // Corresponds to plugStateChanged (12)
	println("VST-DISPATCH-EXIT: cmd=12 result=0") // VstPlugin.cppの戻り値を模倣

	plugin.Resume() // Resume plugin for processing (corresponds to effMainsChanged 1)
	
	// Plugin SetBufferSize: ホストのバッファサイズを設定
	bufSize := int(hostCallback(vst2.HostGetBufferSize, 0, 0, nil, 0)) // hostCallbackはすでに1024を返す
	println("Plugin SetBufferSize:", bufSize)
	println("VST-DISPATCH-ENTRY: cmd=11 (plugSetBufferSize) param2=", bufSize)
	plugin.SetBufferSize(bufSize)
	println("VST-DISPATCH-EXIT: cmd=11 result=0") // VstPlugin.cppの戻り値を模倣
	
	samplesToProcess := bufSize // hostCallbackから取得したバッファサイズを使用
	channelCount := 2 // 固定のチャンネル数

	in := vst2.NewFloatBuffer(channelCount, samplesToProcess)
	out := vst2.NewFloatBuffer(channelCount, samplesToProcess)
	defer in.Free()
	defer out.Free()

	defer println("さいなら")

	// mainゴルーチンからの開始指示を待つ
	<-startProcessing 
	println("Audio processing started by main goroutine.")

	loopcnt := 0
	for {
		loopcnt++
		if loopcnt > 823901 { // オーバーフロー防止
			loopcnt = 0
		}
		//println("loop", loopcnt) ///動作確認用

		// --- オーディオ処理 ---
		println("VST-PROCESS-ENTRY (loop:", loopcnt, ")")
		plugin.ProcessFloat(in, out)
		println("VST-PROCESS-EXIT")

		// --- PlugEditIdleの呼び出し (LMMSの模倣) ---
		println("VST-DISPATCH-ENTRY: cmd=19 (PlugEditIdle)")
		plugin.Dispatch(vst2.PluginOpcode(opcode["PlugEditIdle"]), 0, 0, nil, 0)
		println("VST-DISPATCH-EXIT: cmd=19 result=0")

		// --- オーディオデータ変換と送信 ---
		buf := make([]byte, samplesToProcess*channelCount*2) // 16bitは2バイト
		for i := 0; i < samplesToProcess; i++ {
			for c := 0; c < channelCount; c++ {
				sample := out.Channel(c)[i]
				sampleInt := int16(sample * 32767.0)
				binary.LittleEndian.PutUint16(buf[(i*channelCount+c)*2:], uint16(sampleInt))
			}
		}
		
		buf_chan <- buf // 処理済みバッファを再生側に送信
	}
}
