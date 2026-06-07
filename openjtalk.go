package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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

func main() {
	var initialText string
	var dictPath string

	if len(os.Args) >= 2 {
		initialText = os.Args[1]
	}
	if len(os.Args) >= 3 {
		dictPath = os.Args[2]
	} else {
		candidateDirs := []string{
			"open_jtalk_dic_utf_8-1.11",
			"dic",
			`C:\openjtalk\naist-jdic`,
		}
		for _, d := range candidateDirs {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dictPath = d
				break
			}
		}
		if dictPath == "" {
			dictPath = "open_jtalk_dic_utf_8-1.11"
		}
	}

	engine, err := libopj.NewOpenJTalkEngine(dictPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 0, '\t', 0)

	if initialText != "" {
		morphemes, err := engine.Analyze(initialText)
		if err != nil {
			fmt.Fprintf(os.Stderr, "分析エラー: %v\n", err)
		} else {
			printAnalysis(w, initialText, morphemes)
		}
		return
	}

	fmt.Println("Open JTalk 言語解析ループモード (Enterのみで終了)")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n解析するテキスト > ")
		if !scanner.Scan() {
			break
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			break
		}

		morphemes, err := engine.Analyze(text)
		if err != nil {
			fmt.Fprintf(os.Stderr, "分析エラー: %v\n", err)
			continue
		}
		printAnalysis(w, text, morphemes)
	}
}
