package main

import (
	"bufio"

	"fmt"
	"io/ioutil"
	"log"
	"os"
)

func main() {
	var pluginPath string
	pluginPath = os.Args[1]
	if len(os.Args) < 2 || os.Args[1] == "--gui" {
		pluginPath = "c:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll" // デフォルトのプラグインパス
	}

	/// VST プラグインをロード
	vst, plugin, opcodes, err := loadPlagin(pluginPath)
	if err != nil {
		log.Fatalf("failed to load plugin: %v", err)
	}
	println("open vst")
	defer vst.Close()
	defer plugin.Close()///後で閉じる


	openGUI := false
	if os.Args[len(os.Args)-1] == "--gui" {
		openGUI = true
		println("gui enable")
	}

	//var exec chan ExecRequest // GUI スレッド向けの実行チャネル（なければ nil）
	if openGUI {
		// win32.go の関数（OpenPluginGUIWithWindow）を呼ぶ（非ブロッキング）
		fmt.Println("opengui")
		done, err := OpenPluginGUIWithWindow(plugin, opcodes)
		if err != nil {
			log.Fatalf("failed to open plugin GUI: %v", err)
		}
		//exec = e
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
		// if exec != nil {
		// 	resp := make(chan error, 1)
		// 	exec <- ExecRequest{
		// 		Fn: func() error {
		// 			plugin.SetBankData(data)
		// 			return nil
		// 		},
		// 		Resp: resp,
		// 	}
		// 	if err := <-resp; err != nil {
		// 		log.Fatalf(" SetBankData failed: %v", err)
		// 	}
		// 	println(" バンクをセットしました:", bankPath, "size", len(data))
		// } else {
			// exec が無ければ直接呼ぶ（GUI スレッドが存在しない場合）
			plugin.SetBankData(data)
			println(" バンクをセットしました(直接):", bankPath, "size", len(data))
		//}
	}

	bufio.NewScanner(os.Stdin).Scan()

	fmt.Println("プログラムを正常に終了します。")
}
