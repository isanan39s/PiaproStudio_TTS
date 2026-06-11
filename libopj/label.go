package libopj

import (
	"regexp"
	"strconv"
	"strings"
)

// Label represents a parsed HTS full-context label.
type Label struct {
	Full string

	// Phonemes
	PrevPrevPhoneme string // p1
	PrevPhoneme     string // p2
	Phoneme         string // p3 (Current)
	NextPhoneme     string // p4
	NextNextPhoneme string // p5

	// /A: Accent phrase context
	DistToAccent int // a1: distance from accent nucleus
	MoraPos      int // a2: forward mora position in accent phrase
	MoraRevPos   int // a3: backward mora position in accent phrase

	// /F: Current accent phrase
	PhraseMoraCount int // f1: number of moras
	PhraseAccentPos int // f2: accent type (position)

	// /G: Next accent phrase (useful for end-of-sentence intonation)
	NextPhraseMoraCount int // g1
	NextPhraseAccentPos int // g2

	// /I: Current breath group
	BreathGroupMoraCount int // i2

	// Meta info
	IsShortPause bool // true if it's a comma-like pause
}

var labelRegex = regexp.MustCompile(`^([^\\^]+)\^([^\\-]+)-([^\\+]+)\+([^\\=]+)=([^\\/]+)/A:([-0-9xx]+)\+([0-9xx]+)\+([0-9xx]+).*/F:([0-9xx]+)_([0-9xx]+).*/G:([0-9xx]+)_([0-9xx]+).*/I:([0-9xx]+)-([0-9xx]+)`)

func atoi(s string) int {
	if s == "xx" || s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

// ParseHTSLabels parses a slice of HTS label strings into Label structs.
func ParseHTSLabels(rawLabels []string) []Label {
	var labels []Label
	for _, raw := range rawLabels {
		match := labelRegex.FindStringSubmatch(raw)
		if match == nil {
			// Basic phoneme extract if regex fails (simplified)
			parts := strings.Split(strings.Split(raw, "/")[0], "-")
			p3 := ""
			if len(parts) > 1 {
				p3 = strings.Split(parts[1], "+")[0]
			}
			labels = append(labels, Label{Full: raw, Phoneme: p3})
			continue
		}

		// ポーズの種類の判定: 次の句のアクセント情報がある場合は「途中のポーズ(、)」
		// 全く情報がない(xx)場合は「文末(。)」に近い
		isShort := match[11] != "xx"

		labels = append(labels, Label{
			Full:                 raw,
			PrevPrevPhoneme:      match[1],
			PrevPhoneme:          match[2],
			Phoneme:              match[3],
			NextPhoneme:          match[4],
			NextNextPhoneme:      match[5],
			DistToAccent:         atoi(match[6]),
			MoraPos:              atoi(match[7]),
			MoraRevPos:           atoi(match[8]),
			PhraseMoraCount:      atoi(match[9]),
			PhraseAccentPos:      atoi(match[10]),
			NextPhraseMoraCount:  atoi(match[11]),
			NextPhraseAccentPos:  atoi(match[12]),
			BreathGroupMoraCount: atoi(match[14]),
			IsShortPause:         isShort,
		})
	}
	return labels
}
