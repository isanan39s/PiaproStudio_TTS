package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"pipelined.dev/audio/vst2"
	"pipelined.dev/signal"

	"github.com/ebitengine/oto/v3"
)

const (
	OpaqueChunkID = 0x436e634b
	FxMagic       = 0x4678426b
	FxVersion     = 1
)

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
}

func NewVstHost(bus *BusHQdat, sync func(func())) *VstHost {
	toBus := make(chan MsgBus, 100)
	bus.registAddr("vst_host", toBus)

	host := &VstHost{
		bus:        bus,
		toBus:      toBus,
		syncFunc:   sync,
		sampleRate: 44100,
		bufferSize: 1024,
		startTime:  time.Now(),
	}

	// oto の初期化 (32bit Float, Stereo, Hostのサンプルレートと同期)
	op := &oto.NewContextOptions{
		SampleRate:   int(host.sampleRate),
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
	}
	println("starting oto audio lib")

	otoCtx, ready, err := oto.NewContext(op)
	if err != nil {
		log.Printf("[Oto] Context creation error: %v", err)
	} else {
		println("waiting ready")

		<-ready
		println("done")

		host.otoContext = otoCtx
		// パイプを作成して、読み取り側を Player に渡し、書き込み側を保持する
		pr, pw := io.Pipe()
		host.otoWriter = pw
		host.otoPlayer = otoCtx.NewPlayer(pr)
		go func() {

			host.otoPlayer.Play() // 再生準備完了
		}()
	}

	println("inited oto")

	host.timeInfo = vst2.TimeInfo{
		SampleRate:         host.sampleRate,
		Tempo:              120.0,
		TimeSigNumerator:   4,
		TimeSigDenominator: 4,
		Flags:              vst2.TransportChanged,
	}

	host.inBuffer = vst2.NewFloatBuffer(32, host.bufferSize)
	host.outBuffer = vst2.NewFloatBuffer(32, host.bufferSize)

	go host.loop()
	return host
}

func (h *VstHost) loop() {
	for msg := range h.toBus {
		if msg.Cmd != "idle" && msg.Cmd != "get_TimeInfo" {
			log.Printf("[Bus] Received: Cmd=%s", msg.Cmd)
		}
		switch msg.Cmd {
		case "load":
			if len(msg.Option) > 1 {
				h.syncFunc(func() { h.loadPlugin(msg.Option[0], msg.Option[1]) })
			}
		case "set_output_conf":
			tmp := msg.Option[0]
			if tmp == "false" {
				h.is_fileOut = false
			} else {
				h.is_fileOut = true
			}

			tmp = msg.Option[1]
			if tmp == "false" {
				h.is_speakerOut = false
			} else {
				h.is_speakerOut = true
			}

		case "play":
			h.startRecording()
			h.mu.Lock()
			h.playing = true
			h.timeInfo.Flags |= (vst2.TransportPlaying | vst2.TransportChanged)
			h.mu.Unlock()
		case "stop":
			h.mu.Lock()
			h.playing = false
			h.timeInfo.Flags &= ^vst2.TransportPlaying
			h.timeInfo.Flags |= vst2.TransportChanged
			h.mu.Unlock()
			h.stopRecording()
		case "seek_ppq": // PPQを指定してシーク移動
			if len(msg.Option) > 0 {
				var targetPpq float64
				fmt.Sscanf(msg.Option[0], "%f", &targetPpq)
				newSamplePos := h.PpqToSample(targetPpq)

				h.mu.Lock()
				h.timeInfo.SamplePos = newSamplePos
				h.timeInfo.PpqPos = targetPpq
				h.timeInfo.Flags |= vst2.TransportChanged
				h.mu.Unlock()
				log.Printf("[Host] Seek to PPQ: %.3f (Sample: %.0f)", targetPpq, newSamplePos)
			}
		case "get_TimeInfo":
			go func() {
				h.bus.sendMsg(MsgBus{
					From:   "vst_host",
					To:     msg.From,
					Cmd:    "reply_timeinfo",
					Option: []string{h.TranscribeTimeInfo()},
				})
			}()

		case "save_fxb":
			h.syncFunc(func() { h.saveFxb(msg.Option[0]) })
		case "load_fxb":
			h.syncFunc(func() { h.loadFxb(msg.Option[0]) })
		case "dump_raw":
			h.syncFunc(func() { h.dumpRaw(msg.Option[0]) })
		case "load_raw": // 生バイナリを直接プラグインにセット
			if len(msg.Option) > 0 {
				h.syncFunc(func() { h.loadRaw(msg.Option[0]) })
			}
		case "patch": // 指定オフセットを16進数で書き換え
			if len(msg.Option) > 2 {
				h.syncFunc(func() { h.patchRaw(msg.Option[0], msg.Option[1], msg.Option[2]) })
			}
		case "compare":
			if len(msg.Option) > 1 {
				h.compareBins(msg.Option[0], msg.Option[1])
			}
		case "idle":
			h.onIdle()
		case "close":
			h.syncFunc(func() { h.unloadPlugin() })
		}
	}
}

func (h *VstHost) loadRaw(filename string) {
	if h.plugin == nil {
		return
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("[Host] Read file error: %v", err)
		return
	}
	h.plugin.SetBankData(data)
	log.Printf("[Host] Loaded Raw Chunk (%d bytes)", len(data))
}

// patchRaw: templateファイルを読み込み、オフセット位置を書き換えて、プラグインにセットする
func (h *VstHost) patchRaw(templateFile, offsetStr, hexData string) {
	if h.plugin == nil {
		return
	}
	data, _ := os.ReadFile(templateFile)

	var offset int
	fmt.Sscanf(offsetStr, "%v", &offset)

	patch, _ := hex.DecodeString(hexData)

	if offset+len(patch) > len(data) {
		log.Printf("[Patcher] Offset out of range")
		return
	}

	copy(data[offset:], patch)
	h.plugin.SetBankData(data)
	log.Printf("[Patcher] Patched %s at 0x%X with %s and loaded", templateFile, offset, hexData)
}

func (h *VstHost) compareBins(fileA, fileB string) {
	log.Printf("[Inspector] Comparing %s and %s...", fileA, fileB)
	a, _ := os.ReadFile(fileA)
	b, _ := os.ReadFile(fileB)
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	diffCount := 0
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			if diffCount < 20 {
				log.Printf("[Diff] Offset: 0x%X ( %d ) | %02X -> %02X", i, i, a[i], b[i])
			}
			diffCount++
		}
	}
	if len(a) != len(b) {
		log.Printf("[Diff] Size changed: %d -> %d", len(a), len(b))
	}
	log.Printf("[Diff] Total differences found: %d bytes", diffCount)
}

func (h *VstHost) dumpRaw(filename string) {
	if h.plugin == nil {
		return
	}
	data := h.plugin.GetBankData()
	if len(data) == 0 {
		return
	}
	os.WriteFile(filename, data, 0644)
	log.Printf("[Host] Dumped Raw Chunk (%d bytes) to %s", len(data), filename)
}

func (h *VstHost) saveFxb(filename string) {
	if h.plugin == nil {
		return
	}
	chunk := h.plugin.GetBankData()
	if len(chunk) == 0 {
		return
	}
	file, _ := os.Create(filename)
	defer file.Close()
	// binary.Write(file, binary.BigEndian, int32(OpaqueChunkID))
	// binary.Write(file, binary.BigEndian, int32(156-8+4+len(chunk)))
	// binary.Write(file, binary.BigEndian, int32(FxMagic))
	// binary.Write(file, binary.BigEndian, int32(FxVersion))
	// binary.Write(file, binary.BigEndian, int32(0x50535469))
	// binary.Write(file, binary.BigEndian, int32(1))
	// binary.Write(file, binary.BigEndian, int32(1))
	// file.Write(make([]byte, 128))
	// binary.Write(file, binary.BigEndian, int32(len(chunk)))
	file.Write(chunk)
}

func (h *VstHost) loadFxb(filename string) {
	if h.plugin == nil {
		return
	}
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()
	file.Seek(156, 0)
	var chunkSize int32
	binary.Read(file, binary.BigEndian, &chunkSize)
	data := make([]byte, chunkSize)
	io.ReadFull(file, data)
	h.plugin.SetBankData(data)
}

func (h *VstHost) audioThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	interval := time.Duration(float64(h.bufferSize) / h.sampleRate * float64(time.Second))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.RLock()
		p, active, playing := h.plugin, h.active, h.playing
		h.mu.RUnlock()
		if !active || p == nil {
			return
		}
		func() {
			defer func() { recover() }()
			h.mu.Lock()
			if playing {
				// 音楽的な位置 (PPQ: Pulses Per Quarter note) の更新
				h.timeInfo.PpqPos = h.timeInfo.SamplePos / h.timeInfo.SampleRate * (h.timeInfo.Tempo / 60.0)
				// 小節の開始位置を計算 (Piapro Studio が小節の区切りを認識するために必要)
				beatsPerBar := float64(h.timeInfo.TimeSigNumerator)
				h.timeInfo.BarStartPos = float64(int(h.timeInfo.PpqPos/beatsPerBar)) * beatsPerBar
			}
			h.timeInfo.NanoSeconds = float64(time.Since(h.startTime).Nanoseconds())
			h.mu.Unlock()
			p.ProcessFloat(h.inBuffer, h.outBuffer)
			h.mu.Lock()
			if playing {
				h.writeToWav()
				h.timeInfo.SamplePos += float64(h.bufferSize)
			}
			h.timeInfo.Flags &= ^vst2.TransportChanged
			h.mu.Unlock()
		}()
	}
}

func (h *VstHost) startRecording() {
	h.wavFile, _ = os.Create("output.wav")
	h.wavDataSize = 0
	h.writeWavHeader(0)
}
func (h *VstHost) writeToWav() {
	l, r := h.outBuffer.Channel(0), h.outBuffer.Channel(1)
	buf := make([]float32, h.bufferSize*2)
	for i := 0; i < h.bufferSize; i++ {
		buf[i*2], buf[i*2+1] = l[i], r[i]
	}

	// 1. リアルタイム再生 (スピーカーへ)
	if h.otoWriter != nil {
		binary.Write(h.otoWriter, binary.LittleEndian, buf)
	}

	// 2. ファイル保存 (WAVへ)
	if h.wavFile != nil {
		binary.Write(h.wavFile, binary.LittleEndian, buf)
		h.wavDataSize += uint32(len(buf) * 4)
	}
}
func (h *VstHost) stopRecording() {
	if h.wavFile == nil {
		return
	}
	h.writeWavHeader(h.wavDataSize)
	h.wavFile.Close()
	h.wavFile = nil
}
func (h *VstHost) writeWavHeader(sz uint32) {
	if h.wavFile == nil {
		return
	}
	h.wavFile.Seek(0, 0)
	binary.Write(h.wavFile, binary.LittleEndian, []byte("RIFF"))
	binary.Write(h.wavFile, binary.LittleEndian, uint32(sz+36))
	binary.Write(h.wavFile, binary.LittleEndian, []byte("WAVEfmt "))
	binary.Write(h.wavFile, binary.LittleEndian, uint32(16))
	binary.Write(h.wavFile, binary.LittleEndian, uint16(3))
	binary.Write(h.wavFile, binary.LittleEndian, uint16(2))
	binary.Write(h.wavFile, binary.LittleEndian, uint32(44100))
	binary.Write(h.wavFile, binary.LittleEndian, uint32(44100*8))
	binary.Write(h.wavFile, binary.LittleEndian, uint16(8))
	binary.Write(h.wavFile, binary.LittleEndian, uint16(32))
	binary.Write(h.wavFile, binary.LittleEndian, []byte("data"))
	binary.Write(h.wavFile, binary.LittleEndian, sz)
}

func (h *VstHost) onIdle() {
	h.mu.RLock()
	p, active := h.plugin, h.active
	h.mu.RUnlock()
	if active && p != nil {
		h.syncFunc(func() { p.Dispatch(vst2.PlugEditIdle, 0, 0, nil, 0) })
	}
}

func (h *VstHost) loadPlugin(path string, hwndStr string) {
	h.unloadPlugin()
	v, _ := vst2.Open(path)
	h.vst = v
	plugin := v.Plugin(func(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
		switch op {
		case vst2.HostVersion:
			return 2400
		case vst2.HostGetLanguage:
			return int64(vst2.HostLanguageJapanese)
		case vst2.HostGetSampleRate:
			return int64(h.sampleRate)
		case vst2.HostGetBufferSize:
			return int64(h.bufferSize)
		case vst2.HostGetTime:
			h.mu.RLock()
			defer h.mu.RUnlock()
			return int64(uintptr(unsafe.Pointer(&h.timeInfo)))
		case vst2.HostCanDo:
			return 1
		case vst2.HostGetVendorString:
			copyString("isanan39s with gemini", ptr)
			return 1
		case vst2.HostGetProductString:
			copyString("Piapro Studio TTS VST2Host", ptr)
			return 1
		case 6:
			return 1
		case vst2.HostIdle:
			return 1
		}
		return 0
	})
	h.plugin = plugin
	plugin.SetSampleRate(signal.Frequency(h.sampleRate))
	plugin.SetBufferSize(h.bufferSize)
	plugin.Start()
	var hwnd uintptr
	fmt.Sscanf(hwndStr, "%x", &hwnd)
	plugin.Dispatch(vst2.PlugEditOpen, 0, 0, unsafe.Pointer(hwnd), 0)
	plugin.Resume()
	h.mu.Lock()
	h.active = true
	h.mu.Unlock()
	go h.audioThread()
	h.bus.sendMsg(MsgBus{Cmd: "plugin_loaded", To: "gui", From: "vst_host"})
}

func (h *VstHost) unloadPlugin() {
	h.mu.Lock()
	h.active = false
	p, v := h.plugin, h.vst
	h.plugin, h.vst = nil, nil
	h.mu.Unlock()
	if p != nil {
		p.Dispatch(vst2.PlugEditClose, 0, 0, nil, 0)
		p.Suspend()
		p.Close()
	}
	if v != nil {
		v.Close()
	}
}

func copyString(s string, ptr unsafe.Pointer) {
	b := []byte(s)
	for i := 0; i < len(b) && i < 255; i++ {
		*(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(i))) = b[i]
	}
	*(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(len(b)))) = 0
}

// TranscribeTimeInfo: TimeInfo を音楽的な形式に変換する（文字起こし）
func (h *VstHost) TranscribeTimeInfo() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	t := h.timeInfo

	// 音楽的な位置の計算 (現在の拍子設定を使用)
	beatsPerBar := float64(t.TimeSigNumerator)
	if beatsPerBar == 0 {
		beatsPerBar = 4
	}
	totalBeats := t.PpqPos
	bar := int(totalBeats/beatsPerBar) + 1
	beat := int(totalBeats)%int(beatsPerBar) + 1
	// 1拍を480ティックとした場合の端数 (MIDI標準的な解像度)
	tick := int((totalBeats - float64(int(totalBeats))) * 480)

	return fmt.Sprintf("位置: %03d:%d:%03d | サンプル: %10.0f | PPQ: %8.3f",
		bar, beat, tick, t.SamplePos, t.PpqPos)
}

// PpqToSample: PPQ（音楽的拍数）をサンプル位置に変換する
func (h *VstHost) PpqToSample(ppq float64) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tempo := h.timeInfo.Tempo
	if tempo <= 0 {
		tempo = 120.0
	}

	// PPQ * (60s / tempo) = 秒数
	// 秒数 * sampleRate = サンプル数
	return ppq * (60.0 / tempo) * h.sampleRate
}
