package main

import (
	"fmt"
	"os"
	"os/exec"
	"net/http"

	"text/tabwriter"

	"openjtalk-go/libopj"
)

func printAnalysis(w *tabwriter.Writer, text string, morphemes []libopj.Morpheme) {
	fmt.Printf("\n【解析結果】「%s」\n", text)
	fmt.Fprintln(w, "ID\t表層形\t品詞\t細分類1\t細分類2\t細分類3\t活用型\t活用形\t原型\t読み\t発音\tAcc\tMora\tRule\tFlag")
	for i, m := range morphemes {
		fmt.Fprintf(w, "[%d]\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\n",
			i, m.Surface, m.POS, m.POSGroup1, m.POSGroup2, m.POSGroup3,
			m.CType, m.CForm, m.Original, m.Read, m.Pronunciation,
			m.Accent, m.MoraSize, m.ChainRule, m.ChainFlag)
	}
	w.Flush()
}

func opjt_main(bus *BusHQdat, dicpath string) {

	// Python APIサーバーの起動
	go func() {
		// Windows環境での仮想環境のPythonパス
		pythonPath := `src\ppsf_pipeline\LibreSVIP\venv\Scripts\python.exe`
		cmd := exec.Command(pythonPath, `src\ppsf_pipeline\api_server.py`)
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
		case "genppsf":
			text := msg.Option[0]
			morphemes, err := engine.Analyze(text)
			if err != nil {
				fmt.Fprintf(os.Stderr, "分析エラー: %v\n", err)
				continue
			}
			printAnalysis(w, text, morphemes)

			// PPSF生成への連携
			notes, _ := ConvertToNotes(morphemes, 1920) // 1920 tick から開始
			outputFile := "test_generated.ppsf.bin"
			if len(msg.Option) > 1 {
				outputFile = msg.Option[1]
			}
			RequestPPSFGeneration(notes, outputFile)
		
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
	"ン":  "N",
	"ッ":  "cl",
	"ー":  "ー", // 長音はそのまま渡すか、前の母音を重ねるなどの処理が必要
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
	"ぞ":  "dz o",
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
