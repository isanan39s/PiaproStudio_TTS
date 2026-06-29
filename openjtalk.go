package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"text/tabwriter"
	"time"
	"unsafe"

	"openjtalk-go/libopj"
)

var currentlen int32

func printAnalysis(w *tabwriter.Writer, text string, morphemes []libopj.Morpheme, labels []string) {
	fmt.Printf("\n【解析結果】「%s」\n", text)
	fmt.Fprintln(w, "ID\t表層形\t品詞\t細分類1\t細分類2\t細分類3\t活用型\t活用形\t原型\t読み\t発音\tAcc\tMora\tRule\tFlag")
	for i, m := range morphemes {
		fmt.Fprintf(w, "[%d]\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\n",
			i, m.Surface, m.POS, m.POSGroup1, m.POSGroup2, m.POSGroup3,
			m.CType, m.CForm, m.Original, m.Read, m.Pronunciation,
			m.Accent, m.MoraSize, m.ChainRule, m.ChainFlag)
	}
	w.Flush()

	if len(labels) > 0 {
		fmt.Println("\n【フルコンテキストラベル解析】")
		parsed := libopj.ParseHTSLabels(labels)
		for i, l := range parsed {
			peak := ""
			if l.DistToAccent == 0 && l.Phoneme != "pau" && l.Phoneme != "sil" {
				peak = " ★アクセント核"
			}
			fmt.Printf("[%d] %-4s (句内位置:%d/%d)%s\n", i, l.Phoneme, l.MoraPos, l.PhraseMoraCount, peak)
		}
	}
}

func opjt_main(bus *BusHQdat, dicpath string) {

	// Python APIサーバーの起動
	go func() {
		// Windows環境での仮想環境のPythonパス
		pythonPath := `python-3.14.5-embed-amd64\python.exe`
		cmd := exec.Command(pythonPath, `ppsf_pipeline\api_server.py`)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "PPSF APIサーバーの起動に失敗しました (Path: %s): %v\n", pythonPath, err)
		} else {
			fmt.Println("PPSF APIサーバーを起動しました。")
		}
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "サーバーが異常終了しました: %v\n", err)
		}
	}()

	// msgBus加盟
	toBus := make(chan MsgBus, 100)
	bus.registAddr("txt2ppsf", toBus)

	// 辞書捜索
	var dictPath string = "open_jtalk_dic_utf_8-1.11"
	candidateDirs := []string{
		dicpath,
		"dic",
		`C:\openjtalk\naist-jdic`,
		"open_jtalk_dic_utf_8-1.11",
	}
	for _, d := range candidateDirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			dictPath = d
			break
		}
	}

	engine, err := libopj.NewOpenJTalkEngine(dictPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	defer engine.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 0, '\t', 0)

	for msg := range toBus {

		switch msg.Cmd {
		case "getwav":
			// text := msg.Option[0]
			// morphemes, _ := engine.Analyze(text)
			// labels := engine.GetLabels()
			// notes, lastTick := ConvertToNotesCombined(morphemes, libopj.ParseHTSLabels(labels), 1920)
			// outputFile := GeneratePPSFFilename(text)
			// RequestPPSFGeneration(notes, outputFile, lastTick, bus)

			go func() {
				// ロードと初期化待ちを少し長めにする
				time.Sleep(300 * time.Millisecond)
				rawReply := make(chan []byte, 1)
				bus.sendMsg(MsgBus{
					To:        "vst_host",
					Cmd:       "capture",
					Option:    []string{fmt.Sprintf("%d", currentlen)},
					ReplyChan: rawReply,
				})

				rawBytes := <-rawReply
				var floatBuf []float32
				if len(rawBytes) > 0 {
					floatBuf = unsafe.Slice((*float32)(unsafe.Pointer(&rawBytes[0])), len(rawBytes)/4)
				}
				wavBytes := encodeFloat32ToWav(floatBuf, 48000)
				msg.ReplyChan <- wavBytes
			}()

		case "say": /// 自動停止付きvst_host.play
			// bus.sendMsg(MsgBus{Cmd: "stop", To: "vst_host", From: "gui"})
			// text := msg.Option[0]
			// morphemes, _ := engine.Analyze(text)
			// labels := engine.GetLabels()
			// notes, lastTick := ConvertToNotesCombined(morphemes, libopj.ParseHTSLabels(labels), 1920)
			// outputFile := GeneratePPSFFilename(text)
			// RequestPPSFGeneration(notes, outputFile, lastTick, bus)

			go func() {
				time.Sleep(300 * time.Millisecond)
				bus.sendMsg(MsgBus{To: "vst_host", Cmd: "seek_ppq", Option: []string{"0"}})
				bus.sendMsg(MsgBus{Cmd: "play", To: "vst_host"})

				// 音符の長さに合わせて自動停止 (180 BPM, 480 TPQN -> 1 tick = 0.6944 ms)
				playDur := time.Duration(float64(currentlen)*0.6944) * time.Millisecond
				time.Sleep(playDur + 500*time.Millisecond) // 余韻500ms
				bus.sendMsg(MsgBus{Cmd: "stop", To: "vst_host", From: "txt2ppsf"})
			}()

		case "genppsf":
			bus.sendMsg(MsgBus{Cmd: "stop", To: "vst_host", From: "gui"})
			text := msg.Option[0]
			morphemes, err := engine.Analyze(text)
			if err != nil {
				fmt.Fprintf(os.Stderr, "分析エラー: %v\n", err)
				continue
			}
			labels := engine.GetLabels()

			printAnalysis(w, text, morphemes, labels)

			parseLabel := libopj.ParseHTSLabels(labels)
			// PPSF生成への連携
			notes, ticklen := ConvertToNotesCombined(morphemes, parseLabel, 1920) // 1920 tick から開始
			currentlen = ticklen
			println(currentlen)

			// テキストと日時からユニークなファイル名を生成
			outputFile := GenerateFilename(text) + ".bin"

			RequestPPSFGeneration(notes, outputFile, ticklen, bus)
			// bus.sendMsg(MsgBus{To: "vst_host", Cmd: "seek_ppq", Option: []string{"0"}})

			// go func() {
			// 	time.Sleep(300 * time.Millisecond)
			// 	bus.sendMsg(MsgBus{Cmd: "play", To: "vst_host",Option:[]string{fmt.Sprintf("%d", ticklen)} })

			// 	// 音符の長さに合わせて自動停止 (180 BPM, 480 TPQN -> 1 tick = 0.6944 ms)
			// 	playDur := time.Duration(float64(ticklen)*0.6944) * time.Millisecond
			// 	time.Sleep(playDur + 500*time.Millisecond) // 余韻500ms
			// 	bus.sendMsg(MsgBus{Cmd: "stop", To: "vst_host", From: "txt2ppsf"})
			// }()

		case "kill":
			resp, err := http.Get("http://127.0.0.1:8000/quit")
			if err != nil {
				// すでに死んでいる、または接続できない場合
				fmt.Fprintf(os.Stderr, "サーバー終了リクエストに失敗（すでに終了している可能性があります）: %v\n", err)
				return
			}
			defer resp.Body.Close()

			fmt.Println("Pythonサーバーへ終了シグナルを送信しました。")

		}

	}
}

var kanaToPhoneme = map[string]string{
	"ア":  "a",
	"イ":  "i",
	"ウ":  "M",
	"エ":  "e",
	"オ":  "o",
	"カ":  "k a",
	"キ":  "k' i",
	"ク":  "k M",
	"ケ":  "k e",
	"コ":  "k o",
	"キャ": "k' a",
	"キュ": "k' M",
	"キョ": "k' o",
	"サ":  "s a",
	"シ":  "S i",
	"ス":  "s M",
	"セ":  "s e",
	"ソ":  "s o",
	"シャ": "S a",
	"シュ": "S M",
	"シェ": "S e",
	"ショ": "S o",
	"タ":  "t a",
	"チ":  "tS i",
	"ツ":  "ts M",
	"テ":  "t e",
	"ト":  "t o",
	"チャ": "tS a",
	"チュ": "tS M",
	"チェ": "tS e",
	"チョ": "tS o",
	"ツァ": "ts a",
	"ツィ": "ts i",
	"ツェ": "ts e",
	"ツォ": "ts o",
	"ティ": "t' i",
	"テュ": "t' M",
	"トゥ": "t M",
	"ナ":  "n a",
	"ニ":  "J i",
	"ヌ":  "n M",
	"ネ":  "n e",
	"ノ":  "n o",
	"ニャ": "J a",
	"ニュ": "J M",
	"ニョ": "J o",
	"ハ":  "h a",
	"ヒ":  "C i",
	"フ":  "p\\ M",
	"ヘ":  "h e",
	"ホ":  "h o",
	"ヒャ": "C a",
	"ヒュ": "C M",
	"ヒョ": "C o",
	"ファ": "p\\ a",
	"フィ": "p\\ i",
	"フェ": "p\\ e",
	"フォ": "p\\ o",
	"フュ": "p\\ j M",
	"マ":  "m a",
	"ミ":  "m' i",
	"ム":  "m M",
	"メ":  "m e",
	"モ":  "m o",
	"ミャ": "m' a",
	"ミュ": "m' M",
	"ミョ": "m' o",
	"ヤ":  "j a",
	"ユ":  "j M",
	"ヨ":  "j o",
	"ラ":  "4 a",
	"リ":  "4' i",
	"ル":  "4 M",
	"レ":  "4 e",
	"ロ":  "4 o",
	"リャ": "4' a",
	"リュ": "4' M",
	"リョ": "4' o",
	"ワ":  "w a",
	"ヲ":  "w o",
	"ン":  "n",
	"ッ":  "cl",
	"ー":  "ー",
	"ウィ": "w i",
	"ウェ": "w e",
	"ウォ": "w o",
	"ガ":  "g a",
	"ギ":  "g' i",
	"グ":  "g M",
	"ゲ":  "g e",
	"ゴ":  "g o",
	"ギャ": "g' a",
	"ギュ": "g' M",
	"ギョ": "g' o",
	"ザ":  "dz a",
	"ジ":  "dZ i",
	"ズ":  "dz M",
	"ゼ":  "dz e",
	"ゾ":  "dz o",
	"ジャ": "dZ a",
	"ジュ": "dZ M",
	"ジェ": "dZ e",
	"ジョ": "dZ o",
	"ダ":  "d a",
	"ヂ":  "dZ i",
	"ヅ":  "dz M",
	"デ":  "d e",
	"ド":  "d o",
	"ヂャ": "dZ a",
	"ヂュ": "dZ M",
	"ヂョ": "dZ o",
	"ディ": "d' i",
	"デュ": "d' M",
	"ドゥ": "d M",
	"バ":  "b a",
	"ビ":  "b' i",
	"ブ":  "b M",
	"ベ":  "b e",
	"ボ":  "b o",
	"ビャ": "b' a",
	"ビュ": "b' M",
	"ビョ": "b' o",
	"パ":  "p a",
	"ピ":  "p' i",
	"プ":  "p M",
	"ペ":  "p e",
	"ポ":  "p o",
	"ピャ": "p' a",
	"ピュ": "p' M",
	"ピョ": "p' o",
	"ヴァ": "v a",
	"ヴィ": "v i",
	"ヴ":  "v M",
	"ヴェ": "v e",
	"ヴォ": "v o",
	"ヴャ": "v j a",
	"ヴュ": "v j M",
	"ヴョ": "v j o",
}

func encodeFloat32ToWav(audioBuf []float32, sampleRate int) []byte {
	pcmData := make([]byte, len(audioBuf)*2)
	for i, sample := range audioBuf {
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}
		val := int16(sample * 32767.0)
		binary.LittleEndian.PutUint16(pcmData[i*2:], uint16(val))
	}

	dataSize := uint32(len(pcmData))
	wavBuf := new(bytes.Buffer)
	wavBuf.Grow(44 + len(pcmData))

	wavBuf.Write([]byte("RIFF"))
	binary.Write(wavBuf, binary.LittleEndian, uint32(dataSize+36))
	wavBuf.Write([]byte("WAVEfmt "))
	binary.Write(wavBuf, binary.LittleEndian, uint32(16))
	binary.Write(wavBuf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(wavBuf, binary.LittleEndian, uint16(2)) // Stereo
	binary.Write(wavBuf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(wavBuf, binary.LittleEndian, uint32(sampleRate*2*2))
	binary.Write(wavBuf, binary.LittleEndian, uint16(2*2))
	binary.Write(wavBuf, binary.LittleEndian, uint16(16))
	wavBuf.Write([]byte("data"))
	binary.Write(wavBuf, binary.LittleEndian, dataSize)
	wavBuf.Write(pcmData)

	return wavBuf.Bytes()
}
