package sampler

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
	"barbiton/pkg/wav"
)

func TestBlock1VoiceStealingPriority(t *testing.T) {
	s := New(48000)
	for i := range s.voices {
		v := &s.voices[i]
		v.Active = true
		v.startTime = float64(i)
		v.isStealing = false
		v.stealFree = false
		v.releasing = false
	}
	s.voices[5].stealFree = true
	s.voices[5].isStealing = true

	allocated := s.allocVoice()
	if allocated != &s.voices[5] {
		t.Fatalf("expected allocVoice to steal stealFree voice 5, got voice with startTime %v", allocated.startTime)
	}
}

func TestBlock1StereoWavPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stereo.wav")
	frames := 100
	left := make([]float32, frames)
	right := make([]float32, frames)
	for i := 0; i < frames; i++ {
		left[i] = 1.0
		right[i] = -1.0
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := wav.NewWriter(f, wav.Float32, 48000, 2)
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	if _, err := enc.WriteFrames(left, right); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	sd, err := loadSample(path)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Channels != 2 {
		t.Fatalf("expected 2 channels, got %d", sd.Channels)
	}
	if len(sd.Samples) != frames || len(sd.SamplesR) != frames {
		t.Fatalf("expected %d frames in L and R, got %d and %d", frames, len(sd.Samples), len(sd.SamplesR))
	}
	if sd.Samples[0] != 1.0 || sd.SamplesR[0] != -1.0 {
		t.Fatalf("stereo samples altered: L=%v R=%v", sd.Samples[0], sd.SamplesR[0])
	}
}

func TestBlock1StereoSamplerProcess(t *testing.T) {
	frames := 256
	sd := &SampleData{
		Samples:    make([]float32, frames),
		SamplesR:   make([]float32, frames),
		SampleRate: 48000,
		Channels:   2,
	}
	for i := 0; i < frames; i++ {
		sd.Samples[i] = 1.0
		sd.SamplesR[i] = -1.0
	}
	rg := newRegion()
	rg.Sample = sd
	rg.Pan = 0

	s := New(48000)
	s.voices[0].Active = true
	initVoice(&s.voices[0], &rg, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	s.voices[0].Envelope.Trigger()

	buf := dsp.NewStereoBuffer(64)
	buf.SetLen(64)
	s.Process(buf, 64.0/48000)

	leftAvg := float32(0)
	rightAvg := float32(0)
	for i := 10; i < 50; i++ {
		leftAvg += buf.Left[i]
		rightAvg += buf.Right[i]
	}
	if leftAvg <= 0 || rightAvg >= 0 {
		t.Fatalf("stereo separation lost: leftAvg=%v, rightAvg=%v", leftAvg, rightAvg)
	}
}

func TestBlock1SFZFilterApplied(t *testing.T) {
	frames := 4800
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 10000 * float64(i) / 48000))
	}
	sd := &SampleData{
		Samples:    samples,
		SampleRate: 48000,
		Channels:   1,
	}

	rgNoFil := newRegion()
	rgNoFil.Sample = sd
	rgNoFil.Cutoff = -1

	rgFil := newRegion()
	rgFil.Sample = sd
	rgFil.Cutoff = 500
	rgFil.FilType = "lpf_2p"
	rgFil.Resonance = 0

	vNoFil := &Voice{}
	initVoice(vNoFil, &rgNoFil, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	vNoFil.Envelope.Trigger()

	vFil := &Voice{}
	initVoice(vFil, &rgFil, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	vFil.Envelope.Trigger()

	bufNoFil := make([]float32, 512)
	bufFil := make([]float32, 512)

	vNoFil.Process(bufNoFil, 512.0/48000)
	vFil.Process(bufFil, 512.0/48000)

	maxNoFil := float32(0)
	maxFil := float32(0)
	for i := 50; i < 500; i++ {
		a := bufNoFil[i]
		if a < 0 {
			a = -a
		}
		if a > maxNoFil {
			maxNoFil = a
		}
		b := bufFil[i]
		if b < 0 {
			b = -b
		}
		if b > maxFil {
			maxFil = b
		}
	}
	if maxNoFil < 0.5 {
		t.Fatalf("unfiltered signal too quiet: %v", maxNoFil)
	}
	if maxFil > maxNoFil*0.1 {
		t.Fatalf("filter did not attenuate 10kHz sine: maxFil=%v, maxNoFil=%v", maxFil, maxNoFil)
	}
}

func TestBlock1SFZEQApplied(t *testing.T) {
	frames := 4800
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
	}
	sd := &SampleData{
		Samples:    samples,
		SampleRate: 48000,
		Channels:   1,
	}

	rgNoEQ := newRegion()
	rgNoEQ.Sample = sd
	rgNoEQ.Cutoff = -1

	rgEQ := newRegion()
	rgEQ.Sample = sd
	rgEQ.Cutoff = -1
	rgEQ.EQ[0] = EQBand{
		Freq: 1000,
		BW:   1.0,
		Gain: -20,
	}

	vNoEQ := &Voice{}
	initVoice(vNoEQ, &rgNoEQ, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	vNoEQ.Envelope.Trigger()

	vEQ := &Voice{}
	initVoice(vEQ, &rgEQ, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	vEQ.Envelope.Trigger()

	bufNoEQ := make([]float32, 512)
	bufEQ := make([]float32, 512)

	vNoEQ.Process(bufNoEQ, 512.0/48000)
	vEQ.Process(bufEQ, 512.0/48000)

	maxNoEQ := float32(0)
	maxEQ := float32(0)
	for i := 50; i < 500; i++ {
		a := bufNoEQ[i]
		if a < 0 {
			a = -a
		}
		if a > maxNoEQ {
			maxNoEQ = a
		}
		b := bufEQ[i]
		if b < 0 {
			b = -b
		}
		if b > maxEQ {
			maxEQ = b
		}
	}
	if maxEQ > maxNoEQ*0.3 {
		t.Fatalf("EQ cut not applied: maxEQ=%v, maxNoEQ=%v", maxEQ, maxNoEQ)
	}
}

func TestBlock1StereoToMonoStealTransition(t *testing.T) {
	frames := 256
	sdStereo := &SampleData{
		Samples:    make([]float32, frames),
		SamplesR:   make([]float32, frames),
		SampleRate: 48000,
		Channels:   2,
	}
	sdMono := &SampleData{
		Samples:    make([]float32, frames),
		SampleRate: 48000,
		Channels:   1,
	}
	for i := 0; i < frames; i++ {
		sdStereo.Samples[i] = 0.5
		sdStereo.SamplesR[i] = -0.5
		sdMono.Samples[i] = 1.0
	}
	rgStereo := newRegion()
	rgStereo.Sample = sdStereo
	rgStereo.Cutoff = -1

	rgMono := newRegion()
	rgMono.Sample = sdMono
	rgMono.Cutoff = -1

	s := New(48000)
	v := &s.voices[0]
	initVoice(v, &rgStereo, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	v.isStealing = true
	v.stealFadeProgress = 1.0
	v.pendingRegion = &rgMono
	v.pendingPitch = 60
	v.pendingVelocity = 1.0
	v.stealSampleRate = 48000
	v.pendingAttack = 0.001
	v.pendingDecay = 0.001
	v.pendingSustain = 1.0
	v.pendingReleaseEnv = 0.001

	buf := dsp.NewStereoBuffer(64)
	buf.SetLen(64)
	s.Process(buf, 64.0/48000)

	for i := 10; i < 60; i++ {
		if buf.Left[i] <= 0 || buf.Right[i] <= 0 {
			t.Fatalf("frame %d: mono note after steal dropped in stereo mix: L=%v R=%v", i, buf.Left[i], buf.Right[i])
		}
		if math.Abs(float64(buf.Left[i]-buf.Right[i])) > 1e-4 {
			t.Fatalf("frame %d: mono note panned asymmetrically after steal: L=%v R=%v", i, buf.Left[i], buf.Right[i])
		}
	}
}

func TestBlock1StealFilterFlush(t *testing.T) {
	frames := 2048
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 10000 * float64(i) / 48000))
	}
	sd := &SampleData{
		Samples:    samples,
		SampleRate: 48000,
		Channels:   1,
	}
	rgFil := newRegion()
	rgFil.Sample = sd
	rgFil.Cutoff = 400
	rgFil.FilType = "lpf_2p"

	rgNoFil := newRegion()
	rgNoFil.Sample = sd
	rgNoFil.Cutoff = -1

	v := &Voice{}
	initVoice(v, &rgFil, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	v.Envelope.Trigger()

	v.isStealing = true
	v.stealFadeProgress = 0.95
	v.fadeStep = 0.05
	v.pendingRegion = &rgNoFil
	v.pendingPitch = 60
	v.pendingVelocity = 1.0
	v.stealSampleRate = 48000
	v.pendingAttack = 0.001
	v.pendingDecay = 0.001
	v.pendingSustain = 1.0
	v.pendingReleaseEnv = 0.001

	bufL := make([]float32, 64)
	v.Process(bufL, 64.0/48000)

	firstFrameMax := math.Abs(float64(bufL[0]))
	if firstFrameMax > 0.2 {
		t.Fatalf("filter was not flushed on old note before steal handoff: %v", firstFrameMax)
	}
}

func TestBlock1EQVeltrackZeroBaseGain(t *testing.T) {
	frames := 2048
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
	}
	sd := &SampleData{
		Samples:    samples,
		SampleRate: 48000,
		Channels:   1,
	}
	rg := newRegion()
	rg.Sample = sd
	rg.Cutoff = -1
	rg.EQ[0] = EQBand{
		Freq:     1000,
		BW:       1.0,
		Gain:     0,
		Veltrack: -20,
	}

	v := &Voice{}
	initVoice(v, &rg, 60, 0.5, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	if !v.hasEQ[0] {
		t.Fatal("hasEQ[0] should be true when Veltrack is active even if base Gain is zero")
	}

	vNoEQ := &Voice{}
	rgFlat := newRegion()
	rgFlat.Sample = sd
	rgFlat.Cutoff = -1
	initVoice(vNoEQ, &rgFlat, 60, 0.5, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})

	bufEQ := make([]float32, 256)
	bufNoEQ := make([]float32, 256)
	v.Process(bufEQ, 256.0/48000)
	vNoEQ.Process(bufNoEQ, 256.0/48000)

	maxEQ := float32(0)
	maxNoEQ := float32(0)
	for i := 20; i < 200; i++ {
		if math.Abs(float64(bufEQ[i])) > float64(maxEQ) {
			maxEQ = float32(math.Abs(float64(bufEQ[i])))
		}
		if math.Abs(float64(bufNoEQ[i])) > float64(maxNoEQ) {
			maxNoEQ = float32(math.Abs(float64(bufNoEQ[i])))
		}
	}
	if maxEQ <= maxNoEQ*1.5 {
		t.Fatalf("Veltrack boost not applied: maxEQ=%v, maxNoEQ=%v", maxEQ, maxNoEQ)
	}
}

func TestBlock1NotchFilterResponse(t *testing.T) {
	frames := 4800
	sine1k := make([]float32, frames)
	sine100 := make([]float32, frames)
	for i := range sine1k {
		sine1k[i] = float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
		sine100[i] = float32(math.Sin(2 * math.Pi * 100 * float64(i) / 48000))
	}
	sd1k := &SampleData{Samples: sine1k, SampleRate: 48000, Channels: 1}
	sd100 := &SampleData{Samples: sine100, SampleRate: 48000, Channels: 1}

	rg1k := newRegion()
	rg1k.Sample = sd1k
	rg1k.Cutoff = 1000
	rg1k.FilType = "brf_2p"

	rg100 := newRegion()
	rg100.Sample = sd100
	rg100.Cutoff = 1000
	rg100.FilType = "brf_2p"

	v1k := &Voice{}
	initVoice(v1k, &rg1k, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	v1k.Envelope.Trigger()

	v100 := &Voice{}
	initVoice(v100, &rg100, 60, 1.0, 48000, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
	v100.Envelope.Trigger()

	buf1k := make([]float32, 512)
	buf100 := make([]float32, 512)
	v1k.Process(buf1k, 512.0/48000)
	v100.Process(buf100, 512.0/48000)

	max1k := float32(0)
	max100 := float32(0)
	for i := 50; i < 500; i++ {
		if math.Abs(float64(buf1k[i])) > float64(max1k) {
			max1k = float32(math.Abs(float64(buf1k[i])))
		}
		if math.Abs(float64(buf100[i])) > float64(max100) {
			max100 = float32(math.Abs(float64(buf100[i])))
		}
	}
	if max1k > 0.15 {
		t.Fatalf("notch did not reject 1kHz center: %v", max1k)
	}
	if max100 < 0.6 {
		t.Fatalf("notch unexpectedly attenuated 100Hz passband: %v", max100)
	}
}

func TestBlock1PolyphonyOverflowTieBreaking(t *testing.T) {
	s := New(48000)
	for i := range s.voices {
		v := &s.voices[i]
		v.Active = true
		v.startTime = float64(i * 10)
		v.isStealing = false
		v.stealFree = false
		v.releasing = false
	}
	s.voices[1].releasing = true
	s.voices[1].ReleaseTime = 5.0

	s.voices[2].releasing = true
	s.voices[2].ReleaseTime = 2.0

	s.voices[3].stealFree = true
	s.voices[3].isStealing = true
	s.voices[3].startTime = 50.0

	s.voices[4].stealFree = true
	s.voices[4].isStealing = true
	s.voices[4].startTime = 20.0

	first := s.allocVoice()
	if first != &s.voices[4] {
		t.Fatalf("want older stealFree voice 4, got %v", first)
	}
	s.voices[4].stealFree = false

	second := s.allocVoice()
	if second != &s.voices[3] {
		t.Fatalf("want second stealFree voice 3, got %v", second)
	}
	s.voices[3].stealFree = false

	third := s.allocVoice()
	if third != &s.voices[2] {
		t.Fatalf("want older releasing voice 2, got %v", third)
	}
	s.voices[2].releasing = false

	fourth := s.allocVoice()
	if fourth != &s.voices[1] {
		t.Fatalf("want second releasing voice 1, got %v", fourth)
	}
	s.voices[1].releasing = false

	fifth := s.allocVoice()
	if fifth != &s.voices[0] {
		t.Fatalf("want oldest active voice 0, got %v", fifth)
	}
}
