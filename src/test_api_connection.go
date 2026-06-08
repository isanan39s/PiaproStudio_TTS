package main

import (
	"bytes"
	"fmt"
	"net/http"
)

func main() {
	// ユーザー指定の8080ポートへ修正
	jsonBody := `{"cmd": "genppsf","option": ["かえるのうた", "output.ppsf.bin"]}`

	resp, err := http.Post("http://127.0.0.1:8080/", "application/json", bytes.NewBufferString(jsonBody))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Println("Response Status:", resp.Status)
}
