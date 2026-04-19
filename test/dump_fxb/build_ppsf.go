package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

/*
  PPSF (Piapro Studio Project File) リビルダー解説:

  1. FourCC チャンク構造:
     このファイルは "Four Character Code" (4文字のタグ) + 4バイトのサイズ(LE) で構成される
     チャンク形式のバイナリです。
     例: [EVTS][Size][Data...] , [ENOT][Size][Data...]

  2. 階層構造:
     PPSF (ルート)
      ├── EVTS (音響エンジン用: 0x08 音符イベントの羅列)
      └── EDTS (エディタ表示用コンテナ)
            └── ETRS -> ECLS -> ENOT (表示用音符チャンク)

  3. サイズ整合性の重要性:
     中身のバイト数が変わる場合、そのチャンクの Size フィールドだけでなく、
     親チャンク (EDTS等) や最外周 (PPSF) のサイズも連鎖的に更新する必要があります。
     これを怠ると「フォーマットエラー」になります。

  4. EVTS と ENOT の同期:
     Piapro Studio は、音を鳴らすためのデータ(EVTS)と表示するためのデータ(ENOT)を
     別々に持っています。これら2つの Pitch, Tick, Duration が一致していないと
     読み込みエラーや表示の不整合が発生します。
*/

type ChunkHeader struct {
	Magic [4]byte
	Size  uint32 // 続くデータのバイト数
}

// 歌詞・発音記号 (ENOT内部のVSQS)
type VsqsChunk struct {
	Header ChunkHeader // "VSQS"
	Data   []byte      // [ID:1][LyricLen:1][Lyric...][PhoLen:1][Phoneme...]
}

// 画面表示用音符
type EnotChunk struct {
	Header   ChunkHeader // "ENOT"
	Tick     uint32
	UnkTick  uint32
	Duration uint32
	// ... その他の表示フラグや VSQS 子チャンク
	SubChunks []byte // VSQS や VSQA を含む生データ、またはパース済みリスト
}

// クリップデータ (ENOTを複数束ねる)
type EclsChunk struct {
	Header ChunkHeader // "ECLS"
	Notes  []EnotChunk
	Extra  []byte // RECT等
}

// トラックデータ (ECLSを束ねる)
type EtrsChunk struct {
	Header ChunkHeader // "ETRS"
	Clips  []EclsChunk
}

// エディタコンテナ
type EdtsChunk struct {
	Header ChunkHeader // "EDTS"
	RECT   []byte      // RECTチャンク
	Tracks []EtrsChunk
}

// 音響イベント (0x08イベント列)
type EvtsChunk struct {
	Header ChunkHeader // "EVTS"
	Events []byte      // 0x08 音符データのバイナリ列
}

// PPSFルート
type PpsfProject struct {
	Magic      [4]byte      // "PPSF"
	FileLength uint32       // 全体サイズ - 8
	Version    PascalString // "2.0.0"

	// 主要トップレベルチャンク
	meta []byte
	EVTS EvtsChunk
	PLGS []byte
	EDTS EdtsChunk
}

type Note struct {
	Tick     int32
	Pitch    byte
	Duration int32
	Lyric    string
	Phoneme  string
}

// 1. メロディの定義（ここで自由に遊べます）
var melody = []Note{
	{1920, 60, 480, "か", "k a"},
	{2400, 62, 480, "え", "e"},
	{2880, 64, 480, "る", "4 M"},
	{3360, 65, 480, "の", "n o"},
	{3840, 67, 480, "う", "M"},
	{4320, 69, 480, "た", "t a"},
	{4800, 71, 480, "が", "g a"},
}
var ppsf = PpsfProject{}

func findChank(raw []byte, name string) int {
	pos := bytes.Index(raw, []byte(name))
	return pos
}

func main() {
	// テンプレートファイルの読み込み
	templatePath := "raw_state_009c3-b3ドレミファソラシ.bin"
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		fmt.Println("テンプレート読み込み失敗:", err)
		return
	}

	rawOrigin := raw
	_ = rawOrigin

	pos := findChank(raw, "PPSF")
	if pos == -1 {
		println("err PPSF not found")
		return
	}

	ppsf.FileLength = binary.LittleEndian.Uint32(raw[pos+4 : pos+8])

	pos = findChank(raw, "EVTS")
	if pos == -1 {
		println("err evts not found")
		return
	}

	ppsf.meta = raw[8 : pos-1]
	raw = raw[pos:]

	len := binary.LittleEndian.Uint32(raw[pos+4 : pos+8])

	ppsf.EVTS = append(ppsf.EVTS)

	// 2. EVTS (音響エンジン用データ) の再構築
	// EVTS チャンク全体を検出し、その内部の 0x08 イベント列を書き換える

	raw = patchChunk(raw, "EVTS", func(data []byte) []byte {
		res := new(bytes.Buffer)
		res.WriteByte(data[0]) // プレフィックス (0x07等) を保持
		noteIdx := 0
		pos := 1
		for pos < len(data)-10 {
			if data[pos] == 0x08 {
				// 0x08イベントのサイズ取得 (LE 2バイト)
				size := binary.LittleEndian.Uint16(data[pos+1 : pos+3])
				origBody := data[pos+3 : pos+3+int(size)]

				if noteIdx < len(melody) {
					n := melody[noteIdx]
					newBody := make([]byte, len(origBody))
					copy(newBody, origBody)

					// オフセット指定で上書き (構造解析結果に基づく)
					binary.LittleEndian.PutUint32(newBody[0:4], uint32(n.Tick))
					newBody[6] = n.Pitch
					binary.LittleEndian.PutUint32(newBody[9:13], uint32(n.Duration))

					// 歌詞・発音記号の上書き (3バイト枠固定)
					patchStringsInPlace(newBody, n.Lyric, n.Phoneme)

					res.WriteByte(0x08)
					binary.Write(res, binary.LittleEndian, uint16(len(newBody)))
					res.Write(newBody)
				}
				pos += 3 + int(size)
				noteIdx++
			} else {
				res.WriteByte(data[pos])
				pos++
			}
		}
		return res.Bytes()
	})

	// 3. EDTS (画面表示用データ) の再構築
	// EDTS -> ETRS -> ECLS -> ENOT という階層を辿り、ENOT を書き換える
	raw = patchChunk(raw, "EDTS", func(data []byte) []byte {
		noteIdx := 0
		return patchSubChunks(data, "ENOT", func(enotData []byte) []byte {
			if noteIdx < len(melody) {
				n := melody[noteIdx]
				// ENOTチャンク先頭の Tick, Pitch, Duration を同期
				binary.LittleEndian.PutUint32(enotData[0:4], uint32(n.Tick))
				enotData[6] = n.Pitch
				binary.LittleEndian.PutUint32(enotData[7:11], uint32(n.Duration))
				patchStringsInPlace(enotData, n.Lyric, n.Phoneme)
			} else {
				// 余った音符は 2,000,000 Tick (画面外) へ飛ばして隠す
				binary.LittleEndian.PutUint32(enotData[0:4], 2000000)
			}
			noteIdx++
			return enotData
		})
	})

	// 4. 全体サイズの更新 (PPSF チャンクの 4-8バイト目)
	// ファイルサイズ - 8 を LE で書き込む
	if len(raw) > 8 {
		binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
	}

	// 保存
	outputPath := "built_v24_documented.bin"
	os.WriteFile(outputPath, raw, 0644)
	fmt.Printf("生成完了: %s (Size: %d)\n", outputPath, len(raw))
}

// FourCC チャンクを検出し、パッチを当ててサイズを更新する汎用関数
func patchChunk(fileData []byte, magic string, patcher func([]byte) []byte) []byte {
	pos := bytes.Index(fileData, []byte(magic))
	if pos == -1 {
		return fileData
	}

	oldSize := binary.LittleEndian.Uint32(fileData[pos+4 : pos+8])
	oldEnd := pos + 8 + int(oldSize)

	newContent := patcher(fileData[pos+8 : oldEnd])

	// 前半 + Magic + 新サイズ + 新データ + 後半
	res := append([]byte{}, fileData[:pos+4]...)
	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(newContent)))
	res = append(res, sizeBuf...)
	res = append(res, newContent...)
	res = append(res, fileData[oldEnd:]...)
	return res
}

// 入れ子になったチャンクを再帰的にパッチする関数
func patchSubChunks(data []byte, magic string, patcher func([]byte) []byte) []byte {
	var res []byte
	pos := 0
	for pos < len(data)-8 {
		m := string(data[pos : pos+4])
		s := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		nextPos := pos + 8 + int(s)

		if nextPos > len(data) {
			res = append(res, data[pos:]...)
			break
		}

		if m == magic {
			// 対象チャンク (ENOT等) を発見
			newContent := patcher(data[pos+8 : nextPos])
			res = append(res, []byte(m)...)
			sizeBuf := make([]byte, 4)
			binary.LittleEndian.PutUint32(sizeBuf, uint32(len(newContent)))
			res = append(res, sizeBuf...)
			res = append(res, newContent...)
		} else if m == "ETRS" || m == "ECLS" {
			// コンテナチャンクなら再帰
			patchedSub := patchSubChunks(data[pos+8:nextPos], magic, patcher)
			res = append(res, []byte(m)...)
			sizeBuf := make([]byte, 4)
			binary.LittleEndian.PutUint32(sizeBuf, uint32(len(patchedSub)))
			res = append(res, sizeBuf...)
			res = append(res, patchedSub...)
		} else {
			// そのままコピー
			res = append(res, data[pos:nextPos]...)
		}
		pos = nextPos
	}
	return res
}

// 既存の「ドレミ...」の場所を探して歌詞・発音記号を上書きする
func patchStringsInPlace(block []byte, lyric, phoneme string) {
	targets := [][]byte{
		{0xe3, 0x83, 0x89}, // ど
		{0xe3, 0x83, 0xac}, // れ
		{0xe3, 0x83, 0x9f}, // み
		{0xe3, 0x83, 0x95}, // ふぁ
		{0xe3, 0x82, 0xbd}, // そ
		{0xe3, 0x83, 0xa9}, // ら
		{0xe3, 0x82, 0xb7}, // し
	}
	lyricB := []byte(lyric)
	for _, t := range targets {
		idx := bytes.Index(block, t)
		if idx != -1 {
			// 歌詞 (3バイト枠)
			if len(lyricB) >= 3 {
				copy(block[idx:idx+3], lyricB[:3])
			}
			// 発音記号 (pos + 5 から 3バイト枠)
			pB := []byte(phoneme)
			pIdx := idx + 5
			if pIdx+3 <= len(block) {
				copy(block[pIdx:pIdx+3], []byte("   ")) // 一旦クリア
				maxLen := 3
				if len(pB) < maxLen {
					maxLen = len(pB)
				}
				copy(block[pIdx:pIdx+maxLen], pB)
			}
			break
		}
	}
}
