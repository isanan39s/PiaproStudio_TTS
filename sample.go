package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"unsafe"
	"bufio"
	"syscall"

	"pipelined.dev/audio/vst2"
)

// 改良版 hostCallback: よく使われる opcode に対して安全なデフォルトを返す
func hostCallback(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
	switch op {
	case vst2.HostGetVendorVersion:
		return 10 // ホストバージョン
	case vst2.HostGetSampleRate:
		// pluing がサンプルレートを要求したら 44100 を返す（適宜変更）
		return int64(48000)
	case vst2.HostGetBufferSize:
		return int64(512)
	case vst2.HostGetCurrentProcessLevel:
		return int64(0) // safe default
	case vst2.HostGetTime:
		// もし TimeInfo ポインタを期待するなら 0 を返す（プラグイン側で nil を扱えるか依存）
		return 0
	case vst2.HostCanDo:
		// host の機能応答: 0 = unknown / -1 = no / 1 = yes (モジュール依存)
		return 0
	case vst2.HostGetVendorString:
		// 文字列を期待される場合は何もしない（nilポインタ扱い）: 0
		return 0
	case vst2.HostGetProductString:
		// 文字列を期待される場合は何もしない（nilポインタ扱い）: 0
		return 0
	default:
		// ログしておくとデバッグに便利
		fmt.Printf("hostCallback: unhandled opcode=%v index=%d value=%d opt=%v ptr=%v\n", op, index, value, opt, ptr)
		return 0
	}
}

// GUI を開く（簡易: 親 HWND = nil）
func openPluginGUI(plugin *vst2.Plugin) {
	for i := 0; i < 600; i++ {
		if vst2.PluginOpcode(i).String() == "PlugEditOpen" || vst2.PluginOpcode(i).String() == "plugEditOpen" {
			plugin.Dispatch(vst2.PluginOpcode(i), 0, 0, nil, 0)
			return
		}
	}
	fmt.Println("⚠️ PlugEditOpen opcode が見つかりませんでした")
}

func loadPlagin(path string) (*vst2.VST, *vst2.Plugin, map[string]int,error) {
	fmt.Printf("▶️ VST2 プラグインをロード中: %s\n", path)

	// --- 1. ライブラリのロード (VST) ---
	vst, err := vst2.Open(path)
	if err != nil {
		log.Fatalf("❌ VSTライブラリのロードに失敗しました: %v", err)
		return nil, nil,nil, err
	}

	// --- 2. プラグインインスタンスの作成 (Plugin) ---
	hostCallbackFunc := hostCallback
	plugin := vst.Plugin(hostCallbackFunc) // <- v0.11.0 の正しい呼び方
	if plugin == nil {
		log.Fatalf("❌ プラグインインスタンスの作成に失敗しました（nil が返されました）")
	}

	// --- 3. 情報の取得 ---
	name := vst.Name
	numParams := plugin.NumParams()
	var opcodes map[string]int = make(map[string]int)

	// 実行時に opcode 名から plugGetVendorString の値を探して使う（モジュールを編集せずに取得するため）
	vendor := "unknown"
	found := false
	for i := 0; i < 6000; i++ { // 十分大きな範囲を探索
		opcodes[vst2.PluginOpcode(i).String()] = i
		if (vst2.PluginOpcode(i).String() == "plugGetVendorString"||vst2.PluginOpcode(i).String() == "PlugGetVendorString") {
			// バッファは 64 バイト程度あれば十分（ascii64 に相当）
			println("getting vendor")
			var buf [1024]byte
			plugin.Dispatch(vst2.PluginOpcode(i), 0, 0, unsafe.Pointer(&buf[0]), 0)
			vendor = string(bytes.TrimRight(buf[:], "\x00"))
			found = true
			break
		}
	}
	if !found {
		// 探せなかったら既定値のまま
		vendor = "unknown"
	}

	fmt.Println("---------------------------------------")
	fmt.Printf("✅ ロード成功。プラグイン情報を取得しました:\n")
	fmt.Printf("   プラグイン名: %s\n", name)
	fmt.Printf("   ベンダー名: %s\n", vendor)
	fmt.Printf("   パラメータ数: %d\n", numParams)
	fmt.Println("---------------------------------------")

	// パラメータ名の例表示（あれば）
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
		log.Println("⚠️ 使用法: go run main.go <VST2 プラグインのパス>")
		return
	}

	pluginPath := os.Args[1]

	vst, plugin,opcodes, err := loadPlagin(pluginPath)
	_=err
	
	fmt.Println("利用可能な opcode 一覧:", opcodes)

	// --- GUI を開く（オプション: --gui フラグで） ---
	openGUI := false
	if len(os.Args) >= 4 && os.Args[3] == "--gui" {
		openGUI = true
	}
	if openGUI {
		openPluginGUI(plugin)
		fmt.Println("▶️ GUI 開要求を送信しました。プラグインがウィンドウを作るか確認してください。")
	}

	// --- バンクファイルが指定されていれば読み込んでセットする ---
	if len(os.Args) >= 3 && os.Args[2] != "" {
		bankPath := os.Args[2]
		data, err := ioutil.ReadFile(bankPath)
		if err != nil {
			log.Fatalf("❌ バンクファイルの読み込みに失敗しました: %v", err)
		}
		plugin.SetBankData(data)
		fmt.Printf("▶️ バンクをセットしました: %s (%d bytes)\n", bankPath, len(data))
	}

	///待機
	bufio.NewScanner(os.Stdin).Scan() 

	defer vst.Close()
	defer plugin.Close()

	fmt.Println("プログラムを正常に終了します。")
}
