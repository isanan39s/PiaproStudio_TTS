package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"openjtalk-go/libopj"
	"os"
	"unsafe"
)

type NoteReq struct {
	Tick    int32  `json:"tick"`
	Pitch   int    `json:"pitch"`
	Dur     int32  `json:"dur"`
	Lyric   string `json:"lyric"`
	Phoneme string `json:"phoneme"`
}

// RequestPPSFGeneration sends notes to the PPSF generator API and saves the resulting bin file.
func RequestPPSFGeneration(notes []NoteReq, outputFilename string,bus *BusHQdat) {
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
		To: "vst_host",
		Cmd: "load_fxb2",
		Option: []string{
			unsafe.String(&body[0],len(body)),
		},
	})


	fmt.Printf("Success! Generated %s (%d bytes)\n", outputFilename, len(body))
}

func ConvertToNotes(morphemes []libopj.Morpheme, baseTick int32) ([]NoteReq, int32) {
	var notes []NoteReq
	currentTick := baseTick

	// 1モーラあたりの長さ (200ms = 192 dur)
	const moraDur int32 = 160
	// 基準ピッチ (MIDIノート番号 60 = C4)
	const basePitch int = 60

	for _, m := range morphemes {
		pron := m.Pronunciation
		runes := []rune(pron)
		var moras []string

		// モーラ分解 (カタカナの拗音などを考慮)
		for i := 0; i < len(runes); i++ {
			mora := string(runes[i])
			if i+1 < len(runes) {
				next := runes[i+1]
				// ャュョ などの小書きカタカナを判定
				if next == 'ァ' || next == 'ィ' || next == 'ゥ' || next == 'ェ' || next == 'ォ' ||
					next == 'ャ' || next == 'ュ' || next == 'ョ' || next == 'ヮ' {
					mora += string(next)
					i++
				}
			}
			moras = append(moras, mora)
		}

		for i, mora := range moras {
			moraIdx := i + 1

			// ピッチの計算
			pitch := basePitch
			if m.Accent == 0 {
				if moraIdx != 1 {
					pitch = basePitch + 2
				}
			} else if m.Accent == 1 {
				if moraIdx == 1 {
					pitch = basePitch + 2
				}
			} else {
				if moraIdx >= 2 && moraIdx <= m.Accent {
					pitch = basePitch + 2
				}
			}

			ph, exists := kanaToPhoneme[mora]
			if !exists {
				ph = "u n k"
			}

			notes = append(notes, NoteReq{
				Tick:    currentTick,
				Pitch:   pitch,
				Dur:     moraDur,
				Lyric:   mora,
				Phoneme: ph,
			})
			currentTick += moraDur
		}

		// 読点や助詞の後に短いポーズ
		if m.POS == "記号" || m.POS == "助詞" {
			notes = append(notes, NoteReq{
				Tick:    currentTick,
				Pitch:   0,
				Dur:     30,
				Lyric:   "、",
				Phoneme: "pau",
			})
			currentTick += 96
		}
	}

	return notes, currentTick
}
