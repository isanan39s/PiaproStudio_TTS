package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"github.com/ebitengine/oto/v3"
	"pipelined.dev/audio/vst2"
	"pipelined.dev/signal"
)

// SetSpeakerEnabled toggles speaker output
func (h *VstHost) SetSpeakerEnabled(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.is_speakerOut = on
}

// SetWavEnabled toggles wav file output and manages file creation/closure
func (h *VstHost) SetWavEnabled(on bool, path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// No state change needed
	if h.is_fileOut == on && (!on || (h.wavFile != nil && path == h.wavFile.Name())) {
		return nil
	}

	// Close existing file if any
	if h.wavFile != nil {
		h.writeWavHeader(h.wavDataSize)
		h.wavFile.Close()
		h.wavFile = nil
		h.wavDataSize = 0
	}

	h.is_fileOut = on
	if on {
		if path == "" {
			return fmt.Errorf("wav output requires file path")
		}
		f, err := os.Create(path)
		if err != nil {
			h.is_fileOut = false
			return err
		}
		h.wavFile = f
		h.wavDataSize = 0
		h.writeWavHeader(0)
	}
	return nil
}

var activeVstHost *VstHost

func NewVstHost(bus *BusHQdat, sync func(func())) *VstHost {
	toBus := make(chan MsgBus, 100)
	bus.registAddr("vst_host", toBus)

	host := &VstHost{
		bus:           bus,
		toBus:         toBus,
		syncFunc:      sync,
		sampleRate:    48000,
		bufferSize:    1024,
		startTime:     time.Now(),
		is_fileOut:    true,
		is_speakerOut: true,
	}
	activeVstHost = host

	// oto の初期化 (32bit Float, Stereo, Hostのサンプルレートと同期)
	op := &oto.NewContextOptions{
		SampleRate:   int(host.sampleRate),
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
		//BufferSize:   4096/4, // 小さめのバッファ(約512サンプル)で遅延を抑制
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
		Tempo:              180.0,
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

		case "set_output":
			// Option[0] = "speaker" | "wav"
			// Option[1] = "on" | "off"
			// Option[2] (wav only) = output file path
			if len(msg.Option) < 2 {
				log.Printf("[Host] set_output: insufficient options")
				break
			}
			target, state := msg.Option[0], msg.Option[1]
			switch target {
			case "speaker":
				h.SetSpeakerEnabled(state == "on")
			case "wav":
				path := ""
				if state == "on" && len(msg.Option) > 2 {
					path = msg.Option[2]
				}
				if err := h.SetWavEnabled(state == "on", path); err != nil {
					log.Printf("[Host] set_output wav error: %v", err)
				}
			default:
				log.Printf("[Host] set_output: unknown target %s", target)
			}

		case "capture":
			// 指定されたTickまで再生しながらキャプチャし、WAVを返す
			if msg.ReplyChan == nil {
				continue
			}
			endTick := int32(2880) // デフォルトは4小節分 (480 ticks per quarter)
			if len(msg.Option) > 0 {
				if v, err := strconv.Atoi(msg.Option[0]); err == nil {
					endTick += int32(v)
				}
			}

			println("END_TICK:", endTick)
			h.mu.Lock()
			h.captureBuffer = new(bytes.Buffer)

			// 開始時刻(1920)と終了時刻(endTick)をサンプルに変換
			startPos := h.ppqToSample(0)
			endPos := h.ppqToSample(float64(endTick) / 480.0)
			endPos*=1.5
			// 余裕サンプルを追加し、指定 Tick まで確実にキャプチャ
			marginSamples := h.sampleRate // 1 秒分の余裕
			h.captureEndSample = endPos + marginSamples
			h.captureReply = msg.ReplyChan
			h.isCapturing = true

			// 再生開始位置をセット
			h.timeInfo.SamplePos = startPos
			h.timeInfo.PpqPos = 0 //1920.0 / 480.0
			h.playing = true
			h.timeInfo.Flags |= (vst2.TransportPlaying | vst2.TransportChanged)
			h.mu.Unlock()

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
		case "load_fxb2":
			h.syncFunc(func() { h.load_fxb2(msg.Option[0]) })
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

func (h *VstHost) load_fxb2(bytsarr string) {
	if h.plugin == nil {
		return
	}
	bin := []byte(bytsarr)
	h.plugin.SetBankData(bin)

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

			// VSTプロセス実行
			p.ProcessFloat(h.inBuffer, h.outBuffer)

			// 出力データをインターリーブ形式のスライスにまとめる
			l, r := h.outBuffer.Channel(0), h.outBuffer.Channel(1)
			audioBuf := make([]float32, h.bufferSize*2)
			for i := 0; i < h.bufferSize; i++ {
				audioBuf[i*2] = l[i] * 5
				audioBuf[i*2+1] = r[i] * 5
			}

			// --- スピーカー出力 ---
			if h.otoWriter != nil && h.is_speakerOut {
				binary.Write(h.otoWriter, binary.LittleEndian, audioBuf)
			}

			h.mu.Lock()
			if playing {
				// --- キャプチャ処理 ---
				if h.isCapturing {
					binary.Write(h.captureBuffer, binary.LittleEndian, audioBuf)

					// 目標サンプル数に達したかチェック
					if h.timeInfo.SamplePos >= h.captureEndSample {
						log.Printf("[Host] Capture finished at SamplePos: %.0f", h.timeInfo.SamplePos)
						h.isCapturing = false
						// 自動停止：再生も同時に止める
						h.playing = false
						h.timeInfo.Flags &^= vst2.TransportPlaying
						wavData := h.finalizeCapture()
						go func(c chan []byte, d []byte) { c <- d }(h.captureReply, wavData)
					}
				}

				// --- ファイル保存 (output.wav用) ---
				if h.is_fileOut && h.timeInfo.SamplePos >= 160000 {
					// 上記ループで書き込んだバイト数は len(audioBuf) * 2 (float32 -> int16)
					h.wavDataSize += uint32(len(audioBuf) * 2)
				}
				h.timeInfo.SamplePos += float64(h.bufferSize)
			}
			h.timeInfo.Flags &= ^vst2.TransportChanged
			h.mu.Unlock()
		}()
	}
}

// finalizeCapture: キャプチャしたPCMデータにWAVヘッダーを付けてバイナリを生成します
func (h *VstHost) finalizeCapture() []byte {
	pcmData := h.captureBuffer.Bytes()
	dataSize := uint32(len(pcmData))

	wavBuf := new(bytes.Buffer)
	// RIFFヘッダー (16bit PCM, Stereo, 48kHz)
	binary.Write(wavBuf, binary.LittleEndian, []byte("RIFF"))
	binary.Write(wavBuf, binary.LittleEndian, uint32(dataSize+36))
	binary.Write(wavBuf, binary.LittleEndian, []byte("WAVEfmt "))
	binary.Write(wavBuf, binary.LittleEndian, uint32(16))
	binary.Write(wavBuf, binary.LittleEndian, uint16(1)) // 1 = PCM
	binary.Write(wavBuf, binary.LittleEndian, uint16(2)) // Stereo
	binary.Write(wavBuf, binary.LittleEndian, uint32(h.sampleRate))
	binary.Write(wavBuf, binary.LittleEndian, uint32(h.sampleRate*2*2)) // 2bytes * 2ch
	binary.Write(wavBuf, binary.LittleEndian, uint16(2*2))
	binary.Write(wavBuf, binary.LittleEndian, uint16(16)) // 16bit
	binary.Write(wavBuf, binary.LittleEndian, []byte("data"))
	binary.Write(wavBuf, binary.LittleEndian, dataSize)
	wavBuf.Write(pcmData)

	return wavBuf.Bytes()
}

func (h *VstHost) startRecording() {
	if h.is_fileOut {
		h.wavFile, _ = os.Create("output.wav")
		h.wavDataSize = 0
		h.writeWavHeader(0)
	}
}
func (h *VstHost) writeToWav() {
	l, r := h.outBuffer.Channel(0), h.outBuffer.Channel(1)
	buf := make([]float32, h.bufferSize*2)
	for i := 0; i < h.bufferSize; i++ {
		buf[i*2], buf[i*2+1] = l[i], r[i]
	}

	// ファイル保存 (WAVへ)
	binary.Write(h.wavFile, binary.LittleEndian, buf)
	h.wavDataSize += uint32(len(buf) * 4)
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
	// IEEE Float format: 32bit stereo (format 3)
	binary.Write(h.wavFile, binary.LittleEndian, uint16(3))
	binary.Write(h.wavFile, binary.LittleEndian, uint16(2))
	binary.Write(h.wavFile, binary.LittleEndian, uint32(48000))
	binary.Write(h.wavFile, binary.LittleEndian, uint32(48000*8)) // ByteRate = SampleRate * NumChannels * BitsPerSample/8
	binary.Write(h.wavFile, binary.LittleEndian, uint16(2*4))     // BlockAlign = NumChannels * BitsPerSample/8
	binary.Write(h.wavFile, binary.LittleEndian, uint16(32))      // BitsPerSample
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
		if op != 7 {
			println("[hostcallback] opcode ", op, index, value)
		}
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
	return h.ppqToSample(ppq)
}

// ppqToSample (Internal): ロックを保持している場合に使用する内部計算用
func (h *VstHost) ppqToSample(ppq float64) float64 {
	tempo := h.timeInfo.Tempo
	if tempo <= 0 {
		tempo = 180.0
	}
	return ppq * (60.0 / tempo) * h.sampleRate
}

// GetTempo: 現在のプラグインのテンポを返します
func (h *VstHost) GetTempo() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.timeInfo.Tempo <= 0 {
		return 180.0
	}
	return h.timeInfo.Tempo
}

// TickToSeconds: MIDI tick を秒数に変換します
func (h *VstHost) TickToSeconds(tick int32) float64 {
	tempo := h.GetTempo()
	// (tick / 480) = PPQ
	// PPQ * (60 / tempo) = 秒数
	return (float64(tick) / 480.0) * (60.0 / tempo)
}

// TickToDuration: MIDI tick を time.Duration に変換します（現在のテンポを参照、未ロード時は180 BPMフォールバック）
func TickToDuration(tick int32) time.Duration {
	if activeVstHost == nil {
		// フォールバック: 180 BPM / 480 TPQN (1 tick = 0.6944 ms)
		return time.Duration(float64(tick)/1.44) * time.Millisecond
	}
	return time.Duration(activeVstHost.TickToSeconds(tick) * float64(time.Second))
}
