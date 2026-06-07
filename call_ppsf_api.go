package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type NoteReq struct {
	Tick     int32  `json:"tick"`
	Pitch    int    `json:"pitch"`
	Dur      int32  `json:"dur"`
	Lyric    string `json:"lyric"`
	Phoneme  string `json:"phoneme"`
}

func main() {
	// かえるのうた 全14音
	melody := []NoteReq{
		{1920, 60, 480, "か", "k a"}, {2400, 62, 480, "え", "e"},
		{2880, 64, 480, "る", "4 M"}, {3360, 65, 480, "の", "n o"},
		{3840, 67, 480, "う", "M"}, {4320, 69, 480, "た", "t a"},
		{4800, 71, 480, "が", "g a"}, {5280, 69, 480, "き", "k i"},
		{5760, 67, 480, "こ", "k o"}, {6240, 65, 480, "え", "e"},
		{6720, 64, 480, "て", "t e"}, {7200, 62, 480, "く", "k M"},
		{7680, 60, 480, "る", "4 M"}, {8160, 59, 960, "よ", "j o"},
	}

	req := map[string]interface{}{
		"output": "frog_song_full.ppsf.bin",
		"notes":  melody,
	}
	reqBytes, _ := json.Marshal(req)

	resp, err := http.Post("http://127.0.0.1:8000/generate", "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		fmt.Println("API Error:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	os.WriteFile("frog_song_full.ppsf.bin", body, 0644)
	fmt.Printf("Success! Generated frog_song_full.ppsf.bin (%d bytes)\n", len(body))
}
