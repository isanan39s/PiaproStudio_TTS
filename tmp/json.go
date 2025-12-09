package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"net/url"
	"os"
)

// Mora は各音素の情報を保持します
type Mora struct {
	Text            string  `json:"text"`             /// 文字
	Consonant       string  `json:"consonant"`        /// 母音発音
	ConsonantLength float64 `json:"consonant_length"` /// 子音長さ
	Vowel           string  `json:"vowel"`            /// 母音発音
	VowelLength     float64 `json:"vowel_length"`     /// 母音長さ
	Pitch           float64 `json:"pitch"`
}

// AccentPhrase はアクセント句の情報を保持します
type AccentPhrase struct {
	Moras  []Mora `json:"moras"`						/// 各音素
	Accent int    `json:"accent"`		 				/// アクセント位置
	//PauseMora       *Mora  `json:"pause_mora"`       	/// アクセント句の末尾につく無音モーラ
	IsInterrogative bool `json:"is_interrogative"` 		/// ?か tなら語尾上げる？
}

// ResponseData は全体のレスポンス構造を保持します
type ResponseData struct {
	AccentPhrases   []AccentPhrase `json:"accent_phrases"`
	SpeedScale      float64        `json:"speedScale"`
	PitchScale      float64        `json:"pitchScale"`
	IntonationScale float64        `json:"intonationScale"`
	Kana            string         `json:"kana"`
}

var g_httpClient = &http.Client{}
// 任意の文字列に変えるターゲット
const targetText = "ﾐｸｻﾝｶﾜｲｲﾔｯﾀｰ"
// const targetText = "Hello World!" 
// const targetText = "テストテスト"

func get_Accents(text string)string{
// ベースURLを定義
	baseURL := "http://localhost:50021/audio_query"

	// クエリパラメータを構築(モーラ取得)
	moraQueryParams := url.Values{}
	moraQueryParams.Add("text", text) // ここに任意の文字列を設定
	moraQueryParams.Add("speaker", "1")
	moraQueryParams.Add("enable_katakana_english", "true")

	// 完全なリクエストURLを作成（url.Values.Encode()が自動的にエンコードします）
	requestURL := baseURL + "?" + moraQueryParams.Encode()
	fmt.Printf("Request URL: %s\n", requestURL)

	// HTTP POSTリクエストを作成（ボディは空）
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(""))
	if err != nil {
		fmt.Printf("リクエストの作成中にエラーが発生しました: %v\n", err)
		os.Exit(1)
	}

	// リクエストを送信
	resp, err := g_httpClient.Do(req)
	if err != nil {
		fmt.Printf("HTTPリクエストの送信中にエラーが発生しました: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// レスポンスを処理
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("レスポンスボディの読み取り中にエラーが発生しました: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("レスポンスステータス: %s\n", resp.Status)
	fmt.Printf("レスポンスボディ: %s\n", string(body))

	return string(body)

}


func main() {
	text:=targetText
	if len(os.Args) >= 2 {
		text=os.Args[1]
	}

	responseBody:=get_Accents(text)
	var data ResponseData
	err:= json.Unmarshal([]byte(responseBody), &data)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return
	}

	// 0番目の accent_phrases の 0番目の moras の vowel_length にアクセス
	if len(data.AccentPhrases) > 0 && len(data.AccentPhrases[0].Moras) > 0 {
		firstVowelLength := data.AccentPhrases[0].Moras[0].VowelLength
		fmt.Printf("0番目のvowel_lengthの値: %f\n", firstVowelLength)
	} else {
		fmt.Println("データ構造が予期せぬ形式です。")
	}
}
