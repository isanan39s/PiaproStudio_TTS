package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"openjtalk-go/libopj"
	"os"
	"strconv"
	"strings"
	//"time"
	"unicode"
	"unsafe"
)

// GeneratePPSFFilename: 入力の先頭20字(クリーンアップ後) + 日時秒.bin を生成します
func GeneratePPSFFilename(text string) string {
	// ファイル名に使えない文字や空白を除去
	clean := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune(`\/:*?"<>|`, r) {
			return -1
		}
		return r
	}, text)

	runes := []rune(clean)
	if len(runes) > 20 {
		runes = runes[:20]
	}
	prefix := string(runes)
	if prefix == "" {
		prefix = "tts_output"
	}

	//timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s.bin", prefix)//, timestamp)
}

type NoteReq struct {
	Tick    int32  `json:"tick"`
	Pitch   int    `json:"pitch"`
	Dur     int32  `json:"dur"`
	Lyric   string `json:"lyric"`
	Phoneme string `json:"phoneme"`
}

// RequestPPSFGeneration sends notes to the PPSF generator API and saves the resulting bin file.
func RequestPPSFGeneration(notes []NoteReq, outputFilename string, lastTick int32, bus *BusHQdat) {
	req := map[string]interface{}{
		"output": outputFilename,
		"notes":  notes,
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		fmt.Println("JSON Marshal Error:", err)
		return
	}

	resp, err := http.Post("http://127.0.0.1:8000/generate", "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		fmt.Println("API Error:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Read Body Error:", err)
		return
	}
	err = os.WriteFile(outputFilename, body, 0644)
	if err != nil {
		fmt.Println("Write File Error:", err)
		return
	}

	bus.sendMsg(MsgBus{
		To:  "vst_host",
		Cmd: "load_fxb2",
		Option: []string{
			unsafe.String(&body[0], len(body)),
			strconv.Itoa(int(lastTick)),
		},
	})

	fmt.Printf("Success! Generated %s (%d bytes)\n", outputFilename, len(body))
}

var htsToVocaloid = map[string]string{
	"a": "a", "i": "i", "u": "M", "e": "e", "o": "o",
	"A": "a", "I": "i", "U": "M", "E": "e", "O": "o",
	"y": "j", "w": "w", "N": "n", "n": "n",
	"S": "S", "sh": "S", "tS": "tS", "ch": "tS", "ts": "ts", "z": "dz",
	"j": "dZ", // HTSの j は「じゃ」行
	"h": "h", "f": "p\\", "b": "b", "p": "p", "m": "m",
	"k": "k", "g": "g", "r": "4",
}

// ConvertToNotesCombined: 形態素(歌詞用)とラベル(音素・アクセント用)を組み合わせてノートを生成します
func ConvertToNotesCombined(morphemes []libopj.Morpheme, labels []libopj.Label, baseTick int32) ([]NoteReq, int32) {
	var notes []NoteReq
	currentTick := baseTick
	const basePitch int = 60

	// デュレーション設定 (180 BPM前提)
	const (
		durNormal   int32 = 145 // 標準
		durAccented int32 = 175 // アクセント核 (少し長め)
		durSokuon   int32 = 120 // 促音 (少し短め)
		durComma    int32 = 280 // 読点 (、)
		durPeriod   int32 = 480 // 句点 (。)
	)

	// 形態素から「かな」のストリームを作成
	var allMoras []string
	for _, m := range morphemes {
		if m.POS == "記号" {
			continue
		}
		runes := []rune(m.Pronunciation)
		for i := 0; i < len(runes); i++ {
			r := runes[i]
			if r == '\'' || r == '’' || r == '*' || r == '+' || r == ' ' || r == '、' || r == '。' {
				continue
			}
			mora := string(r)
			if i+1 < len(runes) {
				next := runes[i+1]
				if next == 'ァ' || next == 'ィ' || next == 'ゥ' || next == 'ェ' || next == 'ォ' ||
					next == 'ャ' || next == 'ュ' || next == 'ョ' || next == 'ヮ' {
					mora += string(next)
					i++
				}
			}
			allMoras = append(allMoras, mora)
		}
	}

	var currentPhonemes []string
	var lastMoraPos int = -1
	var lastLabel libopj.Label
	moraIdx := 0

	// ノートを確定させるヘルパー
	flush := func(l libopj.Label) {
		if len(currentPhonemes) == 0 {
			return
		}

		lyric := ""
		if moraIdx < len(allMoras) {
			lyric = allMoras[moraIdx]
			moraIdx++
		}

		// 音素の組み立てと翻訳
		var ph string
		duration := durNormal

		if lyric == "ッ" || lyric == "っ" {
			ph = "cl"
			duration = durSokuon
		} else {
			var translated []string
			for _, p := range currentPhonemes {
				if v, ok := htsToVocaloid[p]; ok {
					translated = append(translated, v)
				} else {
					translated = append(translated, p)
				}
			}
			ph = strings.Join(translated, " ")

			// アクセント核なら少し長くする
			if l.DistToAccent == 0 {
				duration = durAccented
			}
		}

		// ピッチ設定
		pitch := basePitch
		if l.DistToAccent == 0 && ph != "cl" {
			pitch += 2
		}

		notes = append(notes, NoteReq{
			Tick:    currentTick,
			Pitch:   pitch,
			Dur:     duration,
			Lyric:   lyric,
			Phoneme: ph,
		})
		currentTick += duration
		currentPhonemes = nil
	}

	// 母音判定
	isVowel := func(p string) bool {
		return strings.ContainsAny(p, "aiueoAIUEO") || p == "N" || p == "n"
	}

	for _, l := range labels {
		// 無音・ポーズの処理
		if l.Phoneme == "pau" || l.Phoneme == "sil" {
			flush(lastLabel)
			if l.Phoneme == "pau" {
				if l.IsShortPause {
					currentTick += durComma
				} else {
					currentTick += durPeriod
				}
			}
			lastMoraPos = -1
			continue
		}

		// 強制フラッシュ条件:
		// 1. モーラ番号が変わった
		// 2. すでに母音を保持している状態で、新しい音素が来た (同じモーラに複数の母音は入れない)
		hasVowel := false
		for _, p := range currentPhonemes {
			if isVowel(p) {
				hasVowel = true
				break
			}
		}

		if (l.MoraPos != lastMoraPos && lastMoraPos != -1) || (hasVowel && !isVowel(l.Phoneme)) {
			flush(lastLabel)
		}

		lastMoraPos = l.MoraPos
		lastLabel = l
		currentPhonemes = append(currentPhonemes, l.Phoneme)
	}
	flush(lastLabel)

	return notes, currentTick
}
