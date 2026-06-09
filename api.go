package main

import (
	"encoding/json"
	"net/http"
)

type APIserver struct {
	bus   *BusHQdat
	toBus chan MsgBus
}

func (api *APIserver) entry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// リクエスト構造体
	var req struct {
		Cmd    string   `json:"cmd"`
		Option []string `json:"option"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// MsgBusメッセージの生成
	msg := MsgBus{
		Cmd:    req.Cmd,
		To:     "txt2ppsf",
		Option: req.Option,
	}
	api.bus.sendMsg(msg)
	println(r, msg.Option[0])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "dispatched"})
}
