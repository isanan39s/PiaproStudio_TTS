package main

import (
	"bytes"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/lxn/walk"
	"pipelined.dev/audio/vst2"
)

const (
	OpaqueChunkID = 0x436e634b
	FxMagic       = 0x4678426b
	FxVersion     = 1
)

type MyMainWindow struct {
	*walk.MainWindow
	vstContainer  *walk.CustomWidget
	bus           *BusHQdat
	plugptahLabel *walk.Label
	statusLabel   *walk.Label // 追加: 再生位置などの表示用
	ppqLineEdit   *walk.LineEdit
	is_fOut       *walk.CheckBox
	is_sOut       *walk.CheckBox
	toBus         chan MsgBus
	closing       chan struct{}
	dumpCount     int
	pulgpath      string
	ttsinput      *walk.LineEdit
}

type VstHost struct {
	bus           *BusHQdat
	toBus         chan MsgBus
	plugin        *vst2.Plugin
	vst           *vst2.VST
	active        bool
	playing       bool
	syncFunc      func(func())
	mu            sync.RWMutex
	sampleRate    float64
	bufferSize    int
	timeInfo      vst2.TimeInfo
	startTime     time.Time
	inBuffer      vst2.FloatBuffer
	outBuffer     vst2.FloatBuffer
	is_fileOut    bool
	wavFile       *os.File
	wavDataSize   uint32
	is_speakerOut bool
	otoContext    *oto.Context
	otoPlayer     *oto.Player
	otoWriter     io.WriteCloser // 追加: スピーカーへの書き込み口

	// リアルタイムキャプチャ用
	isCapturing      bool
	captureBuffer    *bytes.Buffer
	captureEndSample float64
	captureReply     chan []byte
}

type NoteReq struct {
	Tick    int32  `json:"tick"`
	Pitch   int    `json:"pitch"`
	Dur     int32  `json:"dur"`
	Lyric   string `json:"lyric"`
	Phoneme string `json:"phoneme"`
}

type APIserver struct {
	bus   *BusHQdat
	toBus chan MsgBus
}
