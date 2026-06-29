package main

import (
	// "encoding/binary"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	// "strings"
	"path/filepath"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func getNextDumpCount() int {
	max := 0
	files, _ := os.ReadDir(".")
	re := regexp.MustCompile(`^raw_state_(\d+)\.bin$`)
	for _, file := range files {
		res := re.FindStringSubmatch(file.Name())
		if len(res) > 1 {
			num, _ := strconv.Atoi(res[1])
			if num > max {
				max = num
			}
		}
	}
	return max
}

func NewGUI(bus *BusHQdat) *MyMainWindow {
	currentPath, _ := os.Executable()

	mw := new(MyMainWindow)
	mw.bus = bus
	mw.toBus = make(chan MsgBus, 100)
	mw.closing = make(chan struct{})
	mw.dumpCount = getNextDumpCount()
	mw.pulgpath = "C:\\Program Files\\Vstplugins\\Piapro Studio VSTi.dll"
	mw.wavOutPath = filepath.Dir(currentPath)
	bus.registAddr("gui", mw.toBus)

	if err := (MainWindow{
		AssignTo: &mw.MainWindow,
		Title:    "VST Host Demo",
		Size:     Size{Width: 800, Height: 600}, // 高さを少し広げる
		Layout:   VBox{Margins: Margins{Left: 5, Top: 5, Right: 5, Bottom: 5}},
		Children: []Widget{
			// --- 上部コントロールエリア ---
			Composite{
				Layout: VBox{MarginsZero: true},
				Children: []Widget{
					HSplitter{
						Children: []Widget{
							PushButton{
								Text:      "select pluginfile",
								OnClicked: func() { mw.onSelectPlugin() },
							},
							PushButton{
								Text:      "VSTをロード",
								OnClicked: mw.onLoadPlugin,
							},
						},
					},
					Label{
						Text:     mw.pulgpath,
						AssignTo: &mw.plugptahLabel,
					},
					Label{Text: "--------\r\nWave output (32bit-float)"},

					HSplitter{
						Children: []Widget{
							PushButton{
								Text: "再生",
								OnClicked: func() {
									mw.bus.sendMsg(MsgBus{Cmd: "play", To: "vst_host", From: "gui"})
								},
							},
							PushButton{
								Text: "停止",
								OnClicked: func() {
									mw.bus.sendMsg(MsgBus{Cmd: "stop", To: "vst_host", From: "gui"})
									mw.bus.sendMsg(MsgBus{
										To:     "vst_host",
										From:   "gui",
										Cmd:    "seek_ppq",
										Option: []string{mw.ppqLineEdit.Text()},
									})
								},
							},
						},
					},
					HSplitter{
						Children: []Widget{
							LineEdit{
								AssignTo: &mw.ppqLineEdit,
								Text:     "0",
							},
							PushButton{
								Text: "指定したPPQに移動",
								OnClicked: func() {
									mw.bus.sendMsg(MsgBus{
										To:     "vst_host",
										From:   "gui",
										Cmd:    "seek_ppq",
										Option: []string{mw.ppqLineEdit.Text()},
									})
								},
							},
						},
					},
				},
			},

			// --- 出力設定エリア (独立させて確実に表示) ---
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					CheckBox{
						AssignTo: &mw.is_fOut,
						Text:     "ファイルに書き出し",
						Checked:  true,
						OnCheckedChanged: func() {
							state := "off"
							if mw.is_fOut.Checked() {
								state = "on"
							}
							mw.bus.sendMsg(MsgBus{
								Cmd:    "set_output",
								To:     "vst_host",
								From:   "gui",
								Option: []string{"wav", state, "output.wav"},
							})
						},
					},
					CheckBox{
						AssignTo: &mw.is_sOut,
						Text:     "スピーカに出力",
						Checked:  true,
						OnCheckedChanged: func() {
							state := "off"
							if mw.is_sOut.Checked() {
								state = "on"
							}
							mw.bus.sendMsg(MsgBus{
								Cmd:    "set_output",
								To:     "vst_host",
								From:   "gui",
								Option: []string{"speaker", state},
							})
						},
					},
				},
			},

			// --- デバッグエリア ---
			Composite{
				Layout: VBox{MarginsZero: true},
				Children: []Widget{
					Label{Text: "-------\r\nfor debuging"},
					HSplitter{
						Children: []Widget{
							PushButton{
								Text: "Dump Raw",
								OnClicked: func() {
									mw.dumpCount++
									filename := fmt.Sprintf("./dump_fxb/raw_state_%03d.bin", mw.dumpCount)
									mw.bus.sendMsg(MsgBus{Cmd: "dump_raw", To: "vst_host", From: "gui", Option: []string{filename}})
								},
							},
							PushButton{
								Text: "Load Raw",
								OnClicked: func() {
									dlg := new(walk.FileDialog)
									dlg.Filter = "Binary files (*.bin)|*.bin"
									if ok, _ := dlg.ShowOpen(mw.MainWindow); ok {
										mw.bus.sendMsg(MsgBus{Cmd: "load_raw", To: "vst_host", From: "gui", Option: []string{dlg.FilePath}})
									}
								},
							},
							PushButton{
								Text: "Diff Last 2",
								OnClicked: func() {
									if mw.dumpCount < 2 {
										return
									}
									fileA := fmt.Sprintf("./dump_fxb/raw_state_%03d.bin", mw.dumpCount-1)
									fileB := fmt.Sprintf("./dump_fxb/raw_state_%03d.bin", mw.dumpCount)
									mw.bus.sendMsg(MsgBus{Cmd: "compare", To: "vst_host", From: "gui", Option: []string{fileA, fileB}})
								},
							},
						},
					},
				},
			},

			// --- VSTエディタエリア ---
			CustomWidget{
				AssignTo:           &mw.vstContainer,
				MinSize:            Size{Height: 200},
				AlwaysConsumeSpace: true,
			},
			Composite{
				Layout: VBox{MarginsZero: true},
				Children: []Widget{

					HSplitter{
						Children: []Widget{
							LineEdit{
								AssignTo: &mw.ttsinput,
							},

							PushButton{
								Text: "生成",
								OnClicked: func() {
									input:=mw.ttsinput.Text()
									mw.bus.sendMsg(MsgBus{
										Cmd:    "genppsf",
										To:     "txt2ppsf",
										From:   "gui",
										Option: []string{input},
									})
									go func() {
										reply := make(chan []byte, 1)
										mw.bus.sendMsg(MsgBus{
											Cmd:       "getwav",
											To:        "txt2ppsf",
											From:      "gui",
											ReplyChan: reply,
										})
										wavData := <-reply
										if mw.is_fOut.Checked() && len(wavData) > 0 {
											fname := filepath.Join(mw.wavOutPath, fmt.Sprintf("%s", GenerateFilename(mw.ttsinput.Text())))

											if err := os.WriteFile(fname+".wav", wavData, 0644); err != nil {
												log.Printf("[GUI] WAV 書き出し失敗: %v", err)
											} else {
												log.Printf("[GUI] WAV 書き出し完了: %s", fname)
											}

											/// aviutl/かんしくん用txt書き出し todo:on offつける
											if err := os.WriteFile(fname+".txt", []byte(input), 0644); err != nil {
												log.Printf("[GUI] txt 書き出し失敗: %v", err)
											} else {
												log.Printf("[GUI] txt 書き出し完了: %s", fname)
											}
										}
									}()
								},
							},
						},
					},
				},
			},

			// --- ステータスエリア ---
			Label{
				Text:     "停止中",
				AssignTo: &mw.statusLabel,
				Font:     Font{PointSize: 12, Bold: true},
			},
		},
		MenuItems: []MenuItem{
			Menu{
				Text: "FILE",
				Items: []MenuItem{
					Action{
						Text: "open ppsv4xvsti",
						Shortcut: Shortcut{
							Key:       walk.KeyO,
							Modifiers: walk.ModControl,
						},
						OnTriggered: mw.onLoadPlugin,
					},
					Action{
						Text: "save FXB",
						Shortcut: Shortcut{
							Key:       walk.KeyS,
							Modifiers: walk.ModControl,
						},
						OnTriggered: func() {
							filename := fmt.Sprintf("raw_state_%03d.fxb", mw.dumpCount)
							mw.toBus <- MsgBus{
								To:     "vst_host",
								From:   "gui",
								Cmd:    "save_fxb",
								Option: []string{filename},
							}
						},
					},
					Action{
						Text: "load FXB",
						OnTriggered: func() {

							dlg := new(walk.FileDialog)
							dlg.Filter = "Bank files (*.fxb)|*.fxb"
							if ok, _ := dlg.ShowOpen(mw.MainWindow); ok {
								mw.toBus <- MsgBus{
									To:     "vst_host",
									From:   "gui",
									Cmd:    "load_fxb",
									Option: []string{dlg.FilePath},
								}
							}
						},
					},
					Action{
						Text: "Quit",
						Shortcut: Shortcut{
							Key:       walk.KeyQ,
							Modifiers: walk.ModControl,
						},
						OnTriggered: func() {
							mw.bus.sendMsg(MsgBus{Cmd: "close", To: "vst_host", From: "gui"})
							mw.Close()
						},
					},
					Action{
						Text: "wavファイルの保存先",
						OnTriggered: func() {
							dlg := new(walk.FileDialog)
							dlg.Title = "保存先フォルダを選択してください"

							// 2. フォルダ選択ダイアログを表示
							// (第一引数に親ウィンドウを指定)
							if ok, _ := dlg.ShowBrowseFolder(mw.MainWindow); ok {
								// 3. 選択されたパスを取得
								mw.wavOutPath = dlg.FilePath

								// 結果をメッセージボックスで表示
							}
						},
					},
				},
			},

			Menu{
				Text: "help",
				Items: []MenuItem{
					Menu{
						Text: "about this program",
						Items: []MenuItem{
							Action{
								Text: "このプログラムについて",
								OnTriggered: func() {
									mw.appInfo()
								},
							},
						},
					},
				},
			},
		},
	}).Create(); err != nil {
		log.Fatalf("MainWindow生成失敗: %v", err)
	}

	// Closingイベントを別途アタッチ
	mw.Closing().Attach(func(canClose *bool, reason walk.CloseReason) {
		close(mw.closing)
		mw.bus.sendMsg(MsgBus{To: "txt2ppsf", Cmd: "kill"})

		mw.bus.sendMsg(MsgBus{Cmd: "close", To: "vst_host", From: "gui"})
	})

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // 更新頻度を 100ms に調整
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mw.bus.sendMsg(MsgBus{Cmd: "idle", To: "vst_host", From: "gui"})
				mw.bus.sendMsg(MsgBus{Cmd: "get_TimeInfo", To: "vst_host", From: "gui"}) // 位置情報を要求
			case <-mw.closing:
				return
			}
		}
	}()

	go mw.loop()
	return mw
}

func (mw *MyMainWindow) loop() {
	for {
		select {
		case msg := <-mw.toBus:
			switch msg.Cmd {
			case "plugin_loaded":
				log.Println("GUI: プラグインがロードされました")
			case "reply_timeinfo": // ホストからの時間情報返答を処理
				if len(msg.Option) > 0 {
					status := msg.Option[0]
					mw.Synchronize(func() {
						if mw.statusLabel != nil {
							mw.statusLabel.SetText(status)
						}
					})
				}
			}
		case <-mw.closing:
			return
		}
	}
}

func (mw *MyMainWindow) onSelectPlugin() error {
	dlg := new(walk.FileDialog)
	dlg.Filter = "VST2 DLL (*.dll)|*.dll"
	if ok, _ := dlg.ShowOpen(mw.MainWindow); ok {
		mw.pulgpath = dlg.FilePath
		mw.plugptahLabel.SetText(mw.pulgpath)
	} else {
		return fmt.Errorf("kyannseru")
	}
	return nil
}

func (mw *MyMainWindow) onLoadPlugin() {
	if mw.pulgpath == "" {
		if mw.onSelectPlugin() != nil {
			return
		}
	}

	hwndStr := fmt.Sprintf("%x", mw.vstContainer.Handle())
	mw.bus.sendMsg(MsgBus{Cmd: "load", To: "vst_host", From: "gui", Option: []string{mw.pulgpath, hwndStr}})

}

func (mw *MyMainWindow) appInfo() { ///イベントハンドラ的な
	walk.MsgBox(mw.MainWindow.Form(), "このプログラムについて", "アプリ名\nversion:0.1\nby isanan39s", walk.MsgBoxOK)
}
