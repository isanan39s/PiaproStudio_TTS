package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (api *APIserver) entry(w http.ResponseWriter, r *http.Request) {
	var cmd string
	var options []string

	// 1. URLクエリパラメータをチェック
	query := r.URL.Query()
	if query.Get("cmd") != "" {
		cmd = query.Get("cmd")
		if query.Get("text") != "" {
			options = []string{query.Get("text")}
		}
	}

	// 2. クエリになければJSONボディをチェック (POST時のみ)
	if cmd == "" && r.Method == http.MethodPost {
		var req struct {
			Cmd    string   `json:"cmd"`
			Option []string `json:"option"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			cmd = req.Cmd
			options = req.Option
		}
	}

	// コマンドがない場合はヘルプを表示
	if cmd == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "初音ミク TTS API 簡易ヘルプ")
		fmt.Fprintln(w, "--------------------------------")
		fmt.Fprintln(w, "GET/POST クエリ形式:")
		fmt.Fprintln(w, "  ?cmd=say&text=こんにちは    : 音声を生成して再生")
		fmt.Fprintln(w, "  ?cmd=stop                  : 再生停止")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "JSON POST形式:")
		fmt.Fprintln(w, "  {\"cmd\": \"say\", \"option\": [\"テキスト\"]}")
		return
	}

	// WAV返却が必要なコマンドの判定
	if cmd == "getwav" {
		reply := make(chan []byte, 1)
				api.bus.sendMsg(MsgBus{
			Cmd:    "genppsf",
			To:     "txt2ppsf",
			From:   "api",
			Option: options,
		})
		// 直接 getwav コマンドを txt2ppsf に送信し、ReplyChan で結果を受け取る
		msg := MsgBus{
			Cmd:       cmd,
			To:        "txt2ppsf",
			From:      "api",
			Option:    options,
			ReplyChan: reply,
		}
		// 直接 getwav コマンドを txt2ppsf に送信し、ReplyChan で結果を待つ
		api.bus.sendMsg(msg)
		select {
		case wavData := <-reply:
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("Content-Disposition", "attachment; filename=\""+GenerateFilename(options[0])+".wav\"")
			w.Write(wavData)
			return
		case <-time.After(30 * time.Second):
			http.Error(w, "Capture timeout", http.StatusRequestTimeout)
			return
		}
	}

	if cmd == "say" {
		api.bus.sendMsg(MsgBus{
			Cmd:    "genppsf",
			To:     "txt2ppsf",
			From:   "api",
			Option: options,
		})
		api.bus.sendMsg(MsgBus{
			Cmd:  "say",
			To:   "txt2ppsf",
			From: "api"})
	}

	// 通常のコマンド（非同期：say, stop等）
	msg := MsgBus{
		Cmd:    cmd,
		To:     "txt2ppsf",
		From:   "api",
		Option: options,
	}
	api.bus.sendMsg(msg)

	// 通常のレスポンス
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "dispatched",
		"cmd":    cmd,
	})
}
