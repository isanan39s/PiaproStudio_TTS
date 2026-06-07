module github.com/isanan39s/PiaproStudio_TTS.git

go 1.25.5

require (
	github.com/ebitengine/oto/v3 v3.4.0
	github.com/lxn/walk v0.0.0-20210112085537-c389da54e794
	openjtalk-go/libopj v0.0.0-00010101000000-000000000000
	pipelined.dev/audio/vst2 v0.11.0
	pipelined.dev/signal v0.10.0
)

require (
	github.com/ebitengine/purego v0.9.0 // indirect
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	golang.org/x/sys v0.36.0 // indirect
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	pipelined.dev/pipe v0.11.0 // indirect
)

replace openjtalk-go/libopj => ./libopj
