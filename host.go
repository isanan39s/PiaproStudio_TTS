package main

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"pipelined.dev/audio/vst2"
)

type VSTHost struct {
	vsti    *vst2.VST
	plugin  *vst2.Plugin
	opcodes map[string]int

	hostCurrentSample int64         // ホストコールバック用のグローバルサンプルカウンター
	hostTimeInfo      vst2.TimeInfo // プラグインに安定したポインタを渡すためのグローバルなTimeInfo構造体

	canLoadFXB     bool ///制御 似たようなの増やす予定
	isLoadedPlagin bool
}

type VSTWindowSize struct {
	wide  int
	hight int
}

// / vstiからの問い合わせに対する応答
// デバッグ版 hostCallback: どの opcode でクラッシュするか特定用
func (vsthost *VSTHost) hostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
	fmt.Printf("[hostCallback] opcode=%v (%d) index=%d value=%d\n", op, op, index, value)
	switch op {
	case vst2.HostGetVendorVersion:
		return int64(1)
	case vst2.HostGetSampleRate:
		return int64(48000)
	case vst2.HostGetBufferSize:
		return int64(1024) // バッファサイズを512から1024に変更
	case vst2.HostGetCurrentProcessLevel:
		/*
			ProcessLevelUnknown (0): 不明。
			ProcessLevelUser (1): ユーザーインターフェース（UI）スレッド。GUIの更新などの非リアルタイム処理に適しています。
			ProcessLevelRealtime (2): リアルタイムスレッド。実際のオーディオ信号処理（process / processReplacing）を行っている最中です。
			ProcessLevelOffline (3): オフライン処理中（書き出しなど）。
		*/
		return int64(2)
	case vst2.HostGetTime:
		const (
			sampleRate = 48000.0
			tempo      = 120.0
		)
		// プラグインに詳細なTimeInfo構造体を渡す
		// Piapro Studioが正しく機能するためには、音楽的な文脈（テンポ、拍子、小節）が必要
		samplesPerBeat := (sampleRate * 60.0) / tempo
		currentPpq := float64(vsthost.hostCurrentSample) / samplesPerBeat

		// 簡易的な小節開始位置の計算 (4/4拍子の場合)
		// 本来は拍子の変化（テンポマップ）を考慮する必要があります
		beatsPerBar := float64(4) // TimeSigNumerator
		barStart := float64(int(currentPpq/beatsPerBar)) * beatsPerBar

		vsthost.hostTimeInfo = vst2.TimeInfo{
			SamplePos:          float64(vsthost.hostCurrentSample),
			SampleRate:         sampleRate,
			Tempo:              tempo,
			PpqPos:             currentPpq,
			BarStartPos:        barStart, // 修正: 0.0ではなく計算値を推奨
			TimeSigNumerator:   4,
			TimeSigDenominator: 4,
			Flags: vst2.TransportPlaying |
				vst2.TempoValid |
				vst2.PpqPosValid |
				vst2.TimeSigValid |
				vst2.BarsValid,
		}
		println(int64(uintptr(unsafe.Pointer(&vsthost.hostTimeInfo))))
		return int64(uintptr(unsafe.Pointer(&vsthost.hostTimeInfo)))
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
func (vsthost *VSTHost) loadPlugin(path string) error {
	vsthost.isLoadedPlagin = false
	fmt.Printf(" VST2 プラグインをロード中: %s\n", path)

	var err error
	vsthost.vsti, err = vst2.Open(path)
	if err != nil {
		return err
	}

	hostCallbackFunc := func(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
		return vsthost.hostCallback(op, index, value, ptr, opt)
	}

	vsthost.plugin = vsthost.vsti.Plugin(hostCallbackFunc)
	if vsthost.plugin == nil {
		return fmt.Errorf("plugin instance creation failed")
	}

	name := vsthost.vsti.Name
	numParams := vsthost.plugin.NumParams()
	vsthost.opcodes = make(map[string]int)

	// opcode マップ構築とベンダー取得
	vendor := "unknown"
	for i := 0; i < 6000; i++ {
		vsthost.opcodes[vst2.PluginOpcode(i).String()] = i
		if vst2.PluginOpcode(i).String() == "plugGetVendorString" || vst2.PluginOpcode(i).String() == "PlugGetVendorString" {
			var buf [1024]byte
			vsthost.plugin.Dispatch(vst2.PluginOpcode(i), 0, 0, unsafe.Pointer(&buf[0]), 0) ///opcodeを用いた操作の例
			vendor = string(bytes.TrimRight(buf[:], "\x00"))
			break
		}
	}

	var r unsafe.Pointer


	vsthost.plugin.Start()
	//vsthost.plugin.Dispatch(vst2.PluginOpcode(vsthost.opcodes["plugOpen"]), 0, 0, nil, 0.0)
	vsthost.plugin.SetSampleRate(48000)
	vsthost.plugin.SetBufferSize(128)
	//vsthost.plugin.Dispatch(vst2.PluginOpcode(vsthost.opcodes["plugStateChanged"]), 0, 0, nil, 0)
	vsthost.plugin.Resume()
	//vsthost.plugin.Suspend()
	vsthost.plugin.Dispatch(vst2.PluginOpcode(vsthost.opcodes["PlugEditGetRect"]),    0,
    0,
    unsafe.Pointer(&r),
    0,)

	vsthost.isLoadedPlagin = true

	fmt.Println("---------------------------------------")
	fmt.Printf(" ロード成功。プラグイン情報を取得しました:\n")
	fmt.Printf("   プラグイン名: %s\n", name)
	fmt.Printf("   ベンダー名: %s\n", vendor)
	fmt.Printf("   パラメータ数: %d\n", numParams)
	fmt.Println("   opcode :", vsthost.opcodes)
	fmt.Println("---------------------------------------")

	if numParams > 0 {
		fmt.Println("パラメータ一覧:")
		for i := 0; i < numParams; i++ {
			fmt.Printf("  %d: %s\n", i, vsthost.plugin.ParamName(i))
		}
	}
	println("Load Plugin return")
	return nil
}

func (vsthost *VSTHost) VSTPlaginThrad(endchan chan struct{}, rsvchan chan MsgBus, sndchan chan MsgBus) {

	vsthost.isLoadedPlagin = false

	defer close(endchan)

	funchan := make(chan func())
	go func() {
		for rsvMsg := range rsvchan {
			switch rsvMsg.Cmd {
			case "VSTiTh.close":

			case "VSTiTh.loadFXB":
				path := rsvMsg.Option[0]
				///読み込み＆etc
				data, err := os.ReadFile(path)
				if err != nil {
					return
				}
				if vsthost.canLoadFXB == true {
					vsthost.plugin.SetBankData(data) ///本体 あとはファイルからの読み出し
				}

			case "VSTiTh.loadPlugin":
				path := rsvMsg.Option[0]
				funchan <- func() {
					vsthost.loadPlugin(path)

				}

			}

		}
	}()

	//初期化とか？

	tmp := <-funchan
	tmp()
	for {
	}
	///メインのこーど 今後書く

}

///todo:レシーバーでクラスもどき
