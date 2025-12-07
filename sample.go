package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime/trace"
	"time"
)

func main() {
	f, err := os.Create("trace.out")
	if err != nil {
		log.Fatalf("failed to create trace output file: %v", err)
	}
	defer f.Close()

	if err := trace.Start(f); err != nil {
		log.Fatalf("failed to start trace: %v", err)
	}
	defer trace.Stop()

	var pluginPath string
	pluginPath = os.Args[1]
	if len(os.Args) < 2 || os.Args[1] == "--gui" {
		pluginPath = "c:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll" // デフォルトのプラグインパス
	}

	// Load the plugin using the exported function from host.go
	vst, plugin, opcodes, err := LoadPlugin(pluginPath) // Renamed from loadPlagin to LoadPlugin
	if err != nil {
		log.Fatalf("failed to load plugin: %v", err)
	}
	defer vst.Close()
	defer plugin.Close()

	openGUI := false
	if  os.Args[len(os.Args)-1] == "--gui" {
		openGUI = true
		println("gui enable")
	}

	var exec chan ExecRequest // GUI スレッド向けの実行チャネル
	if openGUI {
		fmt.Println("opengui")
		e, err := OpenPluginGUIWithWindow(plugin, opcodes)
		if err != nil {
			log.Fatalf("failed to open plugin GUI: %v", err)
		}
		exec = e
		fmt.Println("Plugin GUI started (non-blocking). Close the plugin window to finish.")
	}

	// Wait a bit to allow GUI to initialize, then process bank data
	time.Sleep(1 * time.Second) // Moved from 5 seconds to 1 second

	if len(os.Args) >= 3 && os.Args[2] != "" && os.Args[2] != "--gui" {
		bankPath := os.Args[2]
		data, err := ioutil.ReadFile(bankPath)
		if err != nil {
			log.Fatalf(" バンクファイルの読み込みに失敗しました: %v", err)
		}

		if exec != nil {
			resp := make(chan error, 1)
			req := ExecRequest{
				Fn: func() error {
					plugin.SetBankData(data)
					return nil
				},
				Resp: resp,
			}
			exec <- req
			if err := <-resp; err != nil {
				log.Fatalf(" SetBankData failed: %v", err)
			}
			println(" バンクをセットしました:", bankPath, "size", len(data))
		} else {
			plugin.SetBankData(data)
			println(" バンクをセットしました(直接):", bankPath, "size", len(data))
		}
	}

	fmt.Println("Press Enter to exit...")
	bufio.NewScanner(os.Stdin).Scan()

	fmt.Println("プログラムを正常に終了します。")
}