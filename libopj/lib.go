package libopj

/*
#cgo CFLAGS: -I. -DHAVE_CONFIG_H -DCHARSET_UTF_8 -O2
#cgo CXXFLAGS: -I. -DHAVE_CONFIG_H -DCHARSET_UTF_8 -O2
#cgo LDFLAGS: -static -static-libgcc -static-libstdc++

#include <stdlib.h>
#include "mecab.h"
#include "njd.h"
#include "text2mecab.h"
#include "mecab2njd.h"
#include "jpcommon.h"
#include "njd2jpcommon.h"
#include "njd_set_accent_phrase.h"
#include "njd_set_accent_type.h"
#include "njd_set_digit.h"
#include "njd_set_long_vowel.h"
#include "njd_set_pronunciation.h"
#include "njd_set_unvoiced_vowel.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// NULLポインタを安全に扱うためのヘルパー
func cGoString(c *C.char) string {
	if c == nil {
		return ""
	}
	return C.GoString(c)
}

// OpenJTalkから抽出したいデータ構造（全フィールド版）
type Morpheme struct {
	Surface       string // 表層形
	POS           string // 品詞
	POSGroup1     string // 品詞細分類1
	POSGroup2     string // 品詞細分類2
	POSGroup3     string // 品詞細分類3
	CType         string // 活用型
	CForm         string // 活用形
	Original      string // 原型
	Read          string // 読み
	Pronunciation string // 発音
	Accent        int    // アクセント型
	MoraSize      int    // モーラ数
	ChainRule     string // アクセント結合規則
	ChainFlag     int    // アクセント結合フラグ
}

// OpenJTalk言語解析エンジン構造体
type OpenJTalkEngine struct {
	mecab    C.Mecab
	njd      C.NJD
	jpcommon C.JPCommon
	dicPath  *C.char
}

func NewOpenJTalkEngine(dictDir string) (*OpenJTalkEngine, error) {
	engine := &OpenJTalkEngine{}
	C.Mecab_initialize(&engine.mecab)
	C.NJD_initialize(&engine.njd)
	C.JPCommon_initialize(&engine.jpcommon)
	engine.dicPath = C.CString(dictDir)
	ret := C.Mecab_load(&engine.mecab, engine.dicPath)
	if ret == 0 {
		C.free(unsafe.Pointer(engine.dicPath))
		return nil, fmt.Errorf("辞書の読み込みに失敗しました。パスを確認してください: %s", dictDir)
	}
	return engine, nil
}

func (e *OpenJTalkEngine) Analyze(text string) ([]Morpheme, error) {
	// 前回の解析結果をクリア
	C.Mecab_refresh(&e.mecab)
	C.NJD_refresh(&e.njd)
	C.JPCommon_refresh(&e.jpcommon)

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	cBuff := (*C.char)(C.malloc(8192))
	defer C.free(unsafe.Pointer(cBuff))

	C.text2mecab(cBuff, cText)
	if C.Mecab_analysis(&e.mecab, cBuff) == 0 {
		return nil, fmt.Errorf("MeCabでの解析に失敗しました")
	}

	C.mecab2njd(&e.njd, C.Mecab_get_feature(&e.mecab), C.Mecab_get_size(&e.mecab))
	C.njd_set_pronunciation(&e.njd)
	C.njd_set_digit(&e.njd)
	C.njd_set_accent_phrase(&e.njd)
	C.njd_set_accent_type(&e.njd)
	C.njd_set_unvoiced_vowel(&e.njd)
	C.njd_set_long_vowel(&e.njd)

	// NJDをJPCommonへ変換
	C.njd2jpcommon(&e.jpcommon, &e.njd)
	C.JPCommon_make_label(&e.jpcommon)

	var results []Morpheme
	currentNode := e.njd.head
	for currentNode != nil {
		results = append(results, Morpheme{
			Surface:       cGoString(C.NJDNode_get_string(currentNode)),
			POS:           cGoString(C.NJDNode_get_pos(currentNode)),
			POSGroup1:     cGoString(C.NJDNode_get_pos_group1(currentNode)),
			POSGroup2:     cGoString(C.NJDNode_get_pos_group2(currentNode)),
			POSGroup3:     cGoString(C.NJDNode_get_pos_group3(currentNode)),
			CType:         cGoString(C.NJDNode_get_ctype(currentNode)),
			CForm:         cGoString(C.NJDNode_get_cform(currentNode)),
			Original:      cGoString(C.NJDNode_get_orig(currentNode)),
			Read:          cGoString(C.NJDNode_get_read(currentNode)),
			Pronunciation: cGoString(C.NJDNode_get_pron(currentNode)),
			Accent:        int(C.NJDNode_get_acc(currentNode)),
			MoraSize:      int(C.NJDNode_get_mora_size(currentNode)),
			ChainRule:     cGoString(C.NJDNode_get_chain_rule(currentNode)),
			ChainFlag:     int(C.NJDNode_get_chain_flag(currentNode)),
		})
		currentNode = currentNode.next
	}
	return results, nil
}

// GetLabels は最新の解析結果からフルコンテキストラベル（HTS形式）を取得します
func (e *OpenJTalkEngine) GetLabels() []string {
	size := int(C.JPCommon_get_label_size(&e.jpcommon))
	features := C.JPCommon_get_label_feature(&e.jpcommon)
	if features == nil {
		return nil
	}

	// C側のポインタ配列をGoのスライスに変換
	slice := (*[1 << 28]*C.char)(unsafe.Pointer(features))[:size:size]
	labels := make([]string, size)
	for i := 0; i < size; i++ {
		labels[i] = C.GoString(slice[i])
	}
	return labels
}

func (e *OpenJTalkEngine) Close() {
	C.Mecab_clear(&e.mecab)
	C.NJD_clear(&e.njd)
	C.JPCommon_clear(&e.jpcommon)
	C.free(unsafe.Pointer(e.dicPath))
}
