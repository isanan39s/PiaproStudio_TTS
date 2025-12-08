package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	Moras  []Mora `json:"moras"`
	Accent int    `json:"accent"` ///アクセント位置
	//PauseMora       *Mora  `json:"pause_mora"`       ///アクセント句の末尾につく無音モーラ
	IsInterrogative bool `json:"is_interrogative"` ///?か tなら語尾上げる？
}

// ResponseData は全体のレスポンス構造を保持します
type ResponseData struct {
	AccentPhrases   []AccentPhrase `json:"accent_phrases"`
	SpeedScale      float64        `json:"speedScale"`
	PitchScale      float64        `json:"pitchScale"`
	IntonationScale float64        `json:"intonationScale"`
	Kana            string         `json:"kana"`
}

func main() {
	jsonData := `{
 "accent_phrases": [
		{
		  "moras": [
			{
			  "text": "テ",
			  "consonant": "t",
			  "consonant_length": 0.0727369412779808,
			  "vowel": "e",
			  "vowel_length": 0.1318332552909851,
			  "pitch": 5.911419868469238
			},
			{
			  "text": "キ",
			  "consonant": "k",
			  "consonant_length": 0.06951668113470078,
			  "vowel": "I",
			  "vowel_length": 0.076276995241642,
			  "pitch": 0
			},
			{
			  "text": "ス",
			  "consonant": "s",
			  "consonant_length": 0.08548218011856079,
			  "vowel": "u",
			  "vowel_length": 0.07536246627569199,
			  "pitch": 5.894742012023926
			},
			{
			  "text": "ト",
			  "consonant": "t",
			  "consonant_length": 0.08034105598926544,
			  "vowel": "o",
			  "vowel_length": 0.23629078269004822,
			  "pitch": 5.71484375
			}
		  ],
		  "accent": 1,
		  "pause_mora": null,
		  "is_interrogative": true
		}
	  ],
	  "speedScale": 1,	
	  "pitchScale": 0,
	  "intonationScale": 1,
	  "volumeScale": 1,
	  "prePhonemeLength": 0.1,
	  "postPhonemeLength": 0.1,
	  "pauseLength": null,
	  "pauseLengthScale": 1,
	  "outputSamplingRate": 24000,
	  "outputStereo": false,
	  "kana": "テ'_キスト？"
	}`

	url := "http://localhost:50021/" // リクエストを送信するURL
	http_post_data := `{"name": "Go Developer", "language": "Go"}`
	bodyReader := strings.NewReader(http_post_data)
	// POSTリクエストを送信
	// 第3引数にコンテンツタイプを指定します
	resp, err := http.Post(url, "application/json", bodyReader)
	if err != nil {
		fmt.Printf("POSTリクエストの送信中にエラーが発生しました: %v\n", err)
		return
	}
	defer resp.Body.Close() // レスポンスボディを必ずクローズする

	// レスポンスのステータスコードを確認
	fmt.Printf("ステータスコード: %d\n", resp.StatusCode)

	// レスポンスボディを読み込む
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("レスポンスボディの読み込み中にエラーが発生しました: %v\n", err)
		return
	}

	// レスポンスボディの内容を表示
	fmt.Printf("レスポンスボディ: %s\n", responseBody)

	var data ResponseData
	err := json.Unmarshal([]byte(jsonData), &data)
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
