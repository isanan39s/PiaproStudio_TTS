package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"
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
		return int64(512)
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

func vstiPlaginRunner(host2vstiMessageChan chan string, vst *vst2.VST, plugin *vst2.Plugin, opcode map[string]int) {
	println("start plagin thead")
	is_openWindow := false
	var msg MSG
	loopcnt := 0

	// Pluginをいつでも動かせるように
	plugin.Start()
	plugin.Dispatch(vst2.PluginOpcode(opcode["plugStateChanged"]), 0, 0, nil, 0)
	plugin.Resume() // Resume plugin for processing
	plugin.SetBufferSize(int(hostCallback(vst2.HostGetBufferSize, 0, 0, nil, 0)))

	defer println("さいなら")

	for {
		//println("loop",loopcnt)///動作確認用
		loopcnt++
		if loopcnt > 823901 {
			loopcnt = 0
		}

		if is_openWindow {
			// PeekMessage: ノンブロッキングでメッセージをチェック 多分win側からのメッセージ
			ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, PM_REMOVE)

			if ret > 0 {
				// メッセージがあれば処理
				if msg.Message == 0x0012 { // WM_QUIT ×とかaltF4?
					break
				}
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			} else {
				// メッセージがなければ少し待機（CPU 負荷軽減）
			}
			procSleep.Call(10)

		} else {
			procSleep.Call(250)
		}

		var value string
		var ok bool
		select {
		case value, ok = <-host2vstiMessageChan:
			if !ok {
				fmt.Println("チャネルは閉じられています。ループ終了。")
				return //クローズされたらループを抜ける
			}
		default:
			if !is_openWindow {
				time.Sleep(10 * time.Millisecond)
			}
			continue
		}

		// メッセージをコマンドと引数に分割します

		fmt.Println("メッセージを受信しました:", value)
		msgFromHost := strings.SplitN(value, ":", 2)
		command := msgFromHost[0]

		switch command {
		case "loadFXB":
			if len(msgFromHost) >= 2 && msgFromHost[1] != "" {
				if err := loadFXB(plugin, msgFromHost[1]); err != nil {
					log.Fatalf("Failed to load FXB file: %v", err)
				}
			}

		case "openGUI":
			//			time.Sleep(200 * time.Millisecond)
			OpenPluginGUIWithWindow(plugin, opcode)
			is_openWindow = true
			//			time.Sleep(200 * time.Millisecond)
			defer plugin.Dispatch(vst2.PluginOpcode(opcode["PlugEditClose"]), 0, 0, nil, 0)

		case "saveFXB":
			if len(msgFromHost) >= 2 && msgFromHost[1] != "" {
				if err := SaveFXB(plugin, msgFromHost[1]); err != nil {
					log.Fatalf("Failed to save FXB file: %v", err)
				}
			}
		case "processWAV":
			if len(msgFromHost) >= 2 {

				parts := strings.Split(msgFromHost[1], ":")
				durationStr := parts[0]
				duration, err := time.ParseDuration(durationStr)
				if err != nil {
					log.Printf("無効なdurationです: %v", err)
				} else {
					// Call the new realtime playback function
					if err := playRealtime(plugin, duration); err != nil {
						log.Printf("リアルタイム再生に失敗しました: %v", err)
					}
				}
			}
		case "vstiexit":
			return // forループを抜けてゴルーチンを終了します
		}
	}

}
