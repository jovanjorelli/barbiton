package sampler

import (
	"math"
	"testing"

	"barbiton/pkg/instruments"
)

func generateSineSamples(freq float64, sampleRate float64, frames int) []float32 {
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * freq * float64(i) / sampleRate))
	}
	return samples
}

func measurePeak(buffer []float32, skipFrames int) float32 {
	peak := float32(0)
	for i := skipFrames; i < len(buffer); i++ {
		v := buffer[i]
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	return peak
}

func TestSFZFilterOrdersLowpassSlopes(t *testing.T) {
	fs := 48000.0
	frames := 2048
	rawSine := generateSineSamples(6000, fs, frames*2)
	sd := &SampleData{
		Samples:    rawSine,
		SampleRate: int(fs),
		Channels:   1,
	}

	buildVoice := func(filType string) *Voice {
		rg := newRegion()
		rg.Sample = sd
		rg.Cutoff = 1000
		rg.FilType = filType
		rg.Resonance = 0
		v := &Voice{}
		initVoice(v, &rg, 60, 1.0, fs, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
		v.Envelope.Trigger()
		return v
	}

	v1p := buildVoice("lpf_1p")
	v2p := buildVoice("lpf_2p")
	v4p := buildVoice("lpf_4p")
	v6p := buildVoice("lpf_6p")

	buf1p := make([]float32, frames)
	buf2p := make([]float32, frames)
	buf4p := make([]float32, frames)
	buf6p := make([]float32, frames)

	dt := float64(frames) / fs
	v1p.ProcessStereo(buf1p, nil, dt)
	v2p.ProcessStereo(buf2p, nil, dt)
	v4p.ProcessStereo(buf4p, nil, dt)
	v6p.ProcessStereo(buf6p, nil, dt)

	peak1p := measurePeak(buf1p, 200)
	peak2p := measurePeak(buf2p, 200)
	peak4p := measurePeak(buf4p, 200)
	peak6p := measurePeak(buf6p, 200)

	if peak1p <= 0 || peak2p <= 0 {
		t.Fatalf("peak is zero")
	}
	if !(peak6p < peak4p && peak4p < peak2p && peak2p < peak1p) {
		t.Fatalf("expected peak6p < peak4p < peak2p < peak1p, got 6p=%v, 4p=%v, 2p=%v, 1p=%v", peak6p, peak4p, peak2p, peak1p)
	}
	if peak2p > peak1p*0.6 {
		t.Fatalf("2-pole attenuation insufficient relative to 1-pole: 2p=%v, 1p=%v", peak2p, peak1p)
	}
	if peak4p > peak2p*0.5 {
		t.Fatalf("4-pole attenuation insufficient relative to 2-pole: 4p=%v, 2p=%v", peak4p, peak2p)
	}
	if peak6p > peak4p*0.5 {
		t.Fatalf("6-pole attenuation insufficient relative to 4-pole: 6p=%v, 4p=%v", peak6p, peak4p)
	}
}

func TestSFZFilterOrdersHighpassSlopes(t *testing.T) {
	fs := 48000.0
	frames := 2048
	rawSine := generateSineSamples(200, fs, frames*2)
	sd := &SampleData{
		Samples:    rawSine,
		SampleRate: int(fs),
		Channels:   1,
	}

	buildVoice := func(filType string) *Voice {
		rg := newRegion()
		rg.Sample = sd
		rg.Cutoff = 1000
		rg.FilType = filType
		rg.Resonance = 0
		v := &Voice{}
		initVoice(v, &rg, 60, 1.0, fs, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
		v.Envelope.Trigger()
		return v
	}

	v1p := buildVoice("hpf_1p")
	v2p := buildVoice("hpf_2p")
	v4p := buildVoice("hpf_4p")
	v6p := buildVoice("hpf_6p")

	buf1p := make([]float32, frames)
	buf2p := make([]float32, frames)
	buf4p := make([]float32, frames)
	buf6p := make([]float32, frames)

	dt := float64(frames) / fs
	v1p.ProcessStereo(buf1p, nil, dt)
	v2p.ProcessStereo(buf2p, nil, dt)
	v4p.ProcessStereo(buf4p, nil, dt)
	v6p.ProcessStereo(buf6p, nil, dt)

	peak1p := measurePeak(buf1p, 200)
	peak2p := measurePeak(buf2p, 200)
	peak4p := measurePeak(buf4p, 200)
	peak6p := measurePeak(buf6p, 200)

	if peak1p <= 0 || peak2p <= 0 {
		t.Fatalf("peak is zero")
	}
	if !(peak6p < peak4p && peak4p < peak2p && peak2p < peak1p) {
		t.Fatalf("expected HP peak6p < peak4p < peak2p < peak1p, got 6p=%v, 4p=%v, 2p=%v, 1p=%v", peak6p, peak4p, peak2p, peak1p)
	}
}

func TestSFZFilterNotchAndBandpassModes(t *testing.T) {
	fs := 48000.0
	frames := 2048
	rawSine := generateSineSamples(1000, fs, frames*2)
	sd := &SampleData{
		Samples:    rawSine,
		SampleRate: int(fs),
		Channels:   1,
	}

	buildVoice := func(filType string) *Voice {
		rg := newRegion()
		rg.Sample = sd
		rg.Cutoff = 1000
		rg.FilType = filType
		rg.Resonance = 0
		v := &Voice{}
		initVoice(v, &rg, 60, 1.0, fs, 0.001, 0.001, 1.0, 0.001, 0, instruments.NoteParams{})
		v.Envelope.Trigger()
		return v
	}

	for _, filType := range []string{"brf", "brf_2p", "notch", "notch_2p"} {
		v := buildVoice(filType)
		buf := make([]float32, frames)
		v.ProcessStereo(buf, nil, float64(frames)/fs)
		pk := measurePeak(buf, 200)
		if pk > 0.15 {
			t.Fatalf("filter type %q did not reject center frequency: peak=%v", filType, pk)
		}
	}

	for _, filType := range []string{"bpf", "bpf_2p"} {
		v := buildVoice(filType)
		buf := make([]float32, frames)
		v.ProcessStereo(buf, nil, float64(frames)/fs)
		pk := measurePeak(buf, 200)
		if pk < 0.7 {
			t.Fatalf("filter type %q did not pass center frequency: peak=%v", filType, pk)
		}
	}
}

func TestSFZFilterZeroAllocationsInProcessStereo(t *testing.T) {
	fs := 48000.0
	frames := 512
	rawSine := generateSineSamples(1000, fs, frames*10)
	sd := &SampleData{
		Samples:    rawSine,
		SampleRate: int(fs),
		Channels:   1,
	}

	rg := newRegion()
	rg.Sample = sd
	rg.Cutoff = 800
	rg.FilType = "lpf_6p"
	rg.Resonance = 3

	v := &Voice{}
	initVoice(v, &rg, 60, 1.0, fs, 0.001, 0.001, 1.0, 10.0, 0, instruments.NoteParams{})
	v.Envelope.Trigger()

	bufL := make([]float32, frames)
	bufR := make([]float32, frames)
	dt := float64(frames) / fs

	v.ProcessStereo(bufL, bufR, dt)

	allocs := testing.AllocsPerRun(1000, func() {
		v.ProcessStereo(bufL, bufR, dt)
	})
	if allocs != 0 {
		t.Fatalf("Voice.ProcessStereo allocated %v times, want 0", allocs)
	}
}
