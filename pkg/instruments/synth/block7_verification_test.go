package synth

import (
	"math"
	"testing"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
)

func TestBlock7VoiceStereoPanSpreading(t *testing.T) {
	sMono := New(48000)
	bufMono := dsp.NewStereoBuffer(512)
	bufMono.SetLen(512)
	sMono.NoteOn(60, 0.8)
	sMono.Process(bufMono, 1.0/48000)
	for i := range bufMono.Left {
		if bufMono.Left[i] != bufMono.Right[i] {
			t.Fatalf("frame %d differed in default pan_spread=0", i)
		}
	}

	for i := 0; i < 8; i++ {
		pL, pR := computeVoicePan(1.0, i)
		pwr := pL*pL + pR*pR
		if math.Abs(pwr-2.0) > 1e-4 {
			t.Fatalf("voice %d power = %v, want 2.0", i, pwr)
		}
	}

	pL0, pR0 := computeVoicePan(1.0, 0)
	if pL0 <= pR0 || math.Abs(pR0) > 1e-6 {
		t.Fatalf("voice 0 expected hard left: L=%v R=%v", pL0, pR0)
	}

	pL7, pR7 := computeVoicePan(1.0, 7)
	if pR7 <= pL7 || math.Abs(pL7) > 1e-6 {
		t.Fatalf("voice 7 expected hard right: L=%v R=%v", pL7, pR7)
	}

	sSpread := New(48000)
	sSpread.SetParameters(map[string]float32{"pan_spread": 1.0})
	bufSpread := dsp.NewStereoBuffer(512)
	bufSpread.SetLen(512)
	sSpread.NoteOn(60, 0.8)
	sSpread.Process(bufSpread, 1.0/48000)

	var sumSqL, sumSqR float64
	for i := range bufSpread.Left {
		sumSqL += float64(bufSpread.Left[i]) * float64(bufSpread.Left[i])
		sumSqR += float64(bufSpread.Right[i]) * float64(bufSpread.Right[i])
	}
	if sumSqL == 0 {
		t.Fatal("voice 0 left channel silent with pan_spread=1")
	}
	if sumSqR > 1e-8 {
		t.Fatalf("voice 0 right channel not silent with pan_spread=1, got sumSq=%v", sumSqR)
	}
}

func TestBlock7SupersawBeatingWithoutClipping(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr)

	sSingle := New(sr)
	sSingle.SetParameters(map[string]float32{
		"waveform1": float32(WaveSaw),
		"waveform2": float32(WaveSaw),
		"sustain":   1.0,
	})
	bufSingle := dsp.NewStereoBuffer(frames)
	bufSingle.SetLen(frames)
	sSingle.NoteOn(60, 0.8)
	sSingle.Process(bufSingle, dt)

	sSuper := New(sr)
	sSuper.SetParameters(map[string]float32{
		"waveform1":     float32(WaveSaw),
		"waveform2":     float32(WaveSaw),
		"unison_voices": 7,
		"unison_detune": 0.35,
		"sustain":       1.0,
	})
	bufSuper := dsp.NewStereoBuffer(frames)
	bufSuper.SetLen(frames)
	sSuper.NoteOn(60, 0.8)
	sSuper.Process(bufSuper, dt)

	maxPeak := float32(0)
	var sumSqSingle, sumSqSuper float64
	for i := 4800; i < frames; i++ {
		sVal := bufSuper.Left[i]
		if math.IsNaN(float64(sVal)) || math.IsInf(float64(sVal), 0) {
			t.Fatalf("supersaw produced NaN/Inf at frame %d", i)
		}
		if a := float32(math.Abs(float64(sVal))); a > maxPeak {
			maxPeak = a
		}
		sumSqSingle += float64(bufSingle.Left[i]) * float64(bufSingle.Left[i])
		sumSqSuper += float64(sVal) * float64(sVal)
	}

	if maxPeak > 3.0 {
		t.Fatalf("supersaw peak = %v, exceeds stability limit 3.0", maxPeak)
	}

	rmsSingle := math.Sqrt(sumSqSingle / float64(frames-4800))
	rmsSuper := math.Sqrt(sumSqSuper / float64(frames-4800))
	ratio := rmsSuper / rmsSingle
	if ratio < 0.5 || ratio > 2.0 {
		t.Fatalf("supersaw RMS ratio = %v, expected ~0.5..2.0", ratio)
	}

	winSize := 480
	var minWinRMS, maxWinRMS float64
	minWinRMS = 1e9
	for i := 4800; i+winSize < frames; i += winSize {
		var winSq float64
		for j := 0; j < winSize; j++ {
			v := float64(bufSuper.Left[i+j])
			winSq += v * v
		}
		wRMS := math.Sqrt(winSq / float64(winSize))
		if wRMS < minWinRMS {
			minWinRMS = wRMS
		}
		if wRMS > maxWinRMS {
			maxWinRMS = wRMS
		}
	}
	beatRatio := maxWinRMS / minWinRMS
	if beatRatio < 1.3 {
		t.Fatalf("supersaw beat ratio = %v, expected > 1.3 for beating", beatRatio)
	}
}

func TestBlock7UnisonStereoSpread(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := 2048

	s := New(sr)
	s.SetParameters(map[string]float32{
		"waveform1":     float32(WaveSaw),
		"waveform2":     float32(WaveSaw),
		"unison_voices": 5,
		"unison_detune": 0.25,
		"unison_spread": 0.9,
		"sustain":       1.0,
	})
	buf := dsp.NewStereoBuffer(frames)
	buf.SetLen(frames)
	s.NoteOn(60, 0.8)
	s.Process(buf, dt)

	diffFound := false
	for i := 256; i < frames; i++ {
		if math.Abs(float64(buf.Left[i]-buf.Right[i])) > 1e-4 {
			diffFound = true
			break
		}
	}
	if !diffFound {
		t.Fatal("unison_spread > 0 produced identical left and right channels")
	}
}

func TestBlock7LFOPitchVibrato(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr)

	s := New(sr)
	s.SetParameters(map[string]float32{
		"waveform1":       float32(WaveSine),
		"waveform2":       float32(WaveSine),
		"cutoff":          20000,
		"sustain":         1.0,
		"lfo_waveform":    0,
		"lfo_rate":        5.0,
		"lfo_pitch_depth": 2.0,
	})
	buf := dsp.NewStereoBuffer(frames)
	buf.SetLen(frames)
	s.NoteOn(69, 0.8)
	s.Process(buf, dt)

	var zeroCrossings []int
	for i := 1; i < frames; i++ {
		if buf.Left[i-1] <= 0 && buf.Left[i] > 0 {
			zeroCrossings = append(zeroCrossings, i)
		}
	}
	if len(zeroCrossings) < 100 {
		t.Fatalf("insufficient zero crossings: %d", len(zeroCrossings))
	}

	var minPeriod, maxPeriod float64
	minPeriod = 1e9
	for i := 1; i < len(zeroCrossings); i++ {
		period := float64(zeroCrossings[i] - zeroCrossings[i-1])
		if period < minPeriod {
			minPeriod = period
		}
		if period > maxPeriod {
			maxPeriod = period
		}
	}

	periodVariation := (maxPeriod - minPeriod) / minPeriod
	if periodVariation < 0.05 {
		t.Fatalf("pitch vibrato period variation = %v, expected > 0.05", periodVariation)
	}

	sVib := New(sr)
	sVib.SetParameters(map[string]float32{
		"waveform1": float32(WaveSine),
		"waveform2": float32(WaveSine),
		"cutoff":    20000,
		"sustain":   1.0,
	})
	bufVib := dsp.NewStereoBuffer(frames)
	bufVib.SetLen(frames)
	sVib.NoteOnParams(69, 0.8, instruments.NoteParams{
		HasVibrato:   true,
		VibratoRate:  5.0,
		VibratoDepth: 2.0,
	})
	sVib.Process(bufVib, dt)

	var zcVib []int
	for i := 1; i < frames; i++ {
		if bufVib.Left[i-1] <= 0 && bufVib.Left[i] > 0 {
			zcVib = append(zcVib, i)
		}
	}
	var minVibP, maxVibP float64
	minVibP = 1e9
	for i := 1; i < len(zcVib); i++ {
		p := float64(zcVib[i] - zcVib[i-1])
		if p < minVibP {
			minVibP = p
		}
		if p > maxVibP {
			maxVibP = p
		}
	}
	varVib := (maxVibP - minVibP) / minVibP
	if varVib < 0.05 {
		t.Fatalf("NoteParams.HasVibrato variation = %v, expected > 0.05", varVib)
	}
}

func TestBlock7LFODelay(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr * 0.5)

	s := New(sr)
	s.SetParameters(map[string]float32{
		"waveform1":       float32(WaveSine),
		"waveform2":       float32(WaveSine),
		"cutoff":          20000,
		"sustain":         1.0,
		"lfo_rate":        10.0,
		"lfo_pitch_depth": 4.0,
		"lfo_delay":       0.2,
	})
	buf := dsp.NewStereoBuffer(frames)
	buf.SetLen(frames)
	s.NoteOn(69, 0.8)
	s.Process(buf, dt)

	var earlyZC, lateZC []int
	for i := 1; i < frames; i++ {
		if buf.Left[i-1] <= 0 && buf.Left[i] > 0 {
			if i < int(sr*0.15) {
				earlyZC = append(earlyZC, i)
			} else if i > int(sr*0.25) {
				lateZC = append(lateZC, i)
			}
		}
	}

	var minEarlyP, maxEarlyP float64
	minEarlyP = 1e9
	for i := 1; i < len(earlyZC); i++ {
		p := float64(earlyZC[i] - earlyZC[i-1])
		if p < minEarlyP {
			minEarlyP = p
		}
		if p > maxEarlyP {
			maxEarlyP = p
		}
	}
	earlyVar := (maxEarlyP - minEarlyP) / minEarlyP
	if earlyVar > 0.02 {
		t.Fatalf("early period variation during delay = %v, expected < 0.02", earlyVar)
	}

	var minLateP, maxLateP float64
	minLateP = 1e9
	for i := 1; i < len(lateZC); i++ {
		p := float64(lateZC[i] - lateZC[i-1])
		if p < minLateP {
			minLateP = p
		}
		if p > maxLateP {
			maxLateP = p
		}
	}
	lateVar := (maxLateP - minLateP) / minLateP
	if lateVar < 0.08 {
		t.Fatalf("late period variation after delay = %v, expected > 0.08", lateVar)
	}
}

func TestBlock7LFOFilterWah(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr)

	filters := []FilterType{Filter1Pole, Filter12dBSVF, Filter24dBCascade, Filter24dBLadder}

	for _, ft := range filters {
		s := New(sr)
		s.SetParameters(map[string]float32{
			"waveform1":        float32(WaveSaw),
			"waveform2":        float32(WaveSaw),
			"filter_type":      float32(ft),
			"cutoff":           1000,
			"resonance":        0.7,
			"sustain":          1.0,
			"lfo_rate":         4.0,
			"lfo_filter_depth": 3.0,
		})
		buf := dsp.NewStereoBuffer(frames)
		buf.SetLen(frames)
		s.NoteOn(60, 0.8)
		s.Process(buf, dt)

		maxPeak := float32(0)
		for i := range buf.Left {
			v := buf.Left[i]
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Fatalf("filter %v wah produced NaN/Inf at frame %d", ft, i)
			}
			if a := float32(math.Abs(float64(v))); a > maxPeak {
				maxPeak = a
			}
		}
		if maxPeak > 3.5 {
			t.Fatalf("filter %v wah peak = %v, expected < 3.5", ft, maxPeak)
		}

		winSize := 1200
		var minRMS, maxRMS float64
		minRMS = 1e9
		for i := 4800; i+winSize < frames; i += winSize {
			var sq float64
			for j := 0; j < winSize; j++ {
				val := float64(buf.Left[i+j])
				sq += val * val
			}
			r := math.Sqrt(sq / float64(winSize))
			if r < minRMS {
				minRMS = r
			}
			if r > maxRMS {
				maxRMS = r
			}
		}
		ratio := maxRMS / minRMS
		if ratio < 1.3 {
			t.Fatalf("filter %v wah RMS modulation ratio = %v, expected > 1.3", ft, ratio)
		}
	}
}

func TestBlock7LFOAmpTremolo(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr)

	s := New(sr)
	s.SetParameters(map[string]float32{
		"waveform1":     float32(WaveSine),
		"waveform2":     float32(WaveSine),
		"cutoff":        20000,
		"sustain":       1.0,
		"lfo_rate":      4.0,
		"lfo_amp_depth": 0.9,
	})
	buf := dsp.NewStereoBuffer(frames)
	buf.SetLen(frames)
	s.NoteOn(60, 0.8)
	s.Process(buf, dt)

	winSize := 1200
	var minPeak, maxPeak float32
	minPeak = 1000.0
	for i := 4800; i+winSize < frames; i += winSize {
		var pk float32
		for j := 0; j < winSize; j++ {
			a := float32(math.Abs(float64(buf.Left[i+j])))
			if a > pk {
				pk = a
			}
		}
		if pk < minPeak {
			minPeak = pk
		}
		if pk > maxPeak {
			maxPeak = pk
		}
	}

	if maxPeak <= 0 {
		t.Fatal("tremolo signal silent")
	}
	ratio := minPeak / maxPeak
	if ratio > 0.4 {
		t.Fatalf("tremolo min/max ratio = %v, expected < 0.4 for amp depth 0.9", ratio)
	}
}

func TestBlock7LFOSampleAndHold(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr

	lfo := voiceLFO{}
	lfo.trigger(987654321)

	var values []float64
	stepFrames := int(sr / 10.0)
	for i := 0; i < stepFrames*5; i++ {
		val := lfo.step(10.0, 0, LFOSampleAndHold, dt)
		if i%stepFrames == 10 {
			values = append(values, val)
		}
	}

	if len(values) < 5 {
		t.Fatalf("expected 5 samples, got %d", len(values))
	}
	allEqual := true
	for i := 1; i < len(values); i++ {
		if math.Abs(values[i]-values[0]) > 1e-4 {
			allEqual = false
			break
		}
	}
	if allEqual {
		t.Fatal("S&H produced identical values across cycles")
	}

	for _, v := range values {
		if v < -1.0 || v > 1.0 {
			t.Fatalf("S&H value out of range [-1, 1]: %v", v)
		}
	}
}

func TestBlock7BackwardCompatibility(t *testing.T) {
	s1 := New(48000)
	buf1 := dsp.NewStereoBuffer(512)
	buf1.SetLen(512)
	s1.NoteOn(60, 0.8)
	s1.Process(buf1, 1.0/48000)

	s2 := New(48000)
	s2.SetParameters(map[string]float32{
		"pan_spread":       0.0,
		"unison_voices":    1,
		"unison_detune":    0.0,
		"unison_spread":    0.0,
		"lfo_pitch_depth":  0.0,
		"lfo_filter_depth": 0.0,
		"lfo_amp_depth":    0.0,
	})
	buf2 := dsp.NewStereoBuffer(512)
	buf2.SetLen(512)
	s2.NoteOn(60, 0.8)
	s2.Process(buf2, 1.0/48000)

	for i := range buf1.Left {
		if buf1.Left[i] != buf2.Left[i] || buf1.Right[i] != buf2.Right[i] {
			t.Fatalf("backward compatibility mismatch at frame %d", i)
		}
	}
}

func TestBlock7ZeroAllocationsAllModes(t *testing.T) {
	buf := dsp.NewStereoBuffer(256)
	buf.SetLen(256)

	configs := []map[string]float32{
		{"pan_spread": 0.5},
		{"unison_voices": 7, "unison_detune": 0.3},
		{"unison_voices": 5, "unison_detune": 0.2, "unison_spread": 0.8},
		{"lfo_waveform": 0, "lfo_pitch_depth": 2.0},
		{"lfo_waveform": 1, "lfo_filter_depth": 3.0},
		{"lfo_waveform": 2, "lfo_amp_depth": 0.6},
		{"lfo_waveform": 3, "lfo_pitch_depth": 1.0},
		{"lfo_waveform": 4, "lfo_filter_depth": 2.0},
		{"lfo_delay": 0.5, "lfo_pitch_depth": 1.0},
		{
			"pan_spread":       0.7,
			"unison_voices":    5,
			"unison_detune":    0.4,
			"unison_spread":    0.9,
			"lfo_waveform":     0,
			"lfo_pitch_depth":  1.5,
			"lfo_filter_depth": 2.0,
			"lfo_amp_depth":    0.5,
			"filter_type":      float32(Filter24dBLadder),
			"filter_env_depth": 2.0,
		},
	}

	for idx, cfg := range configs {
		s := New(48000)
		s.SetParameters(cfg)
		s.NoteOn(60, 0.9)
		s.Process(buf, 1.0/48000)

		allocs := testing.AllocsPerRun(50, func() {
			s.Process(buf, 1.0/48000)
		})
		if allocs != 0 {
			t.Fatalf("config %d allocated %v, want 0", idx, allocs)
		}
	}
}

func TestBlock7EdgeCasesAndRobustness(t *testing.T) {
	for _, idx := range []int{-16, -7, -1, 0, 1, 7, 8, 15, 16, 100} {
		pL, pR := computeVoicePan(0.8, idx)
		if math.IsNaN(pL) || math.IsNaN(pR) || pL < 0 || pR < 0 {
			t.Fatalf("computeVoicePan returned invalid gains for index %d: L=%v R=%v", idx, pL, pR)
		}
		pwr := pL*pL + pR*pR
		if math.Abs(pwr-2.0) > 1e-4 {
			t.Fatalf("computeVoicePan power for index %d = %v, want 2.0", idx, pwr)
		}
	}

	pLNan, pRNan := computeVoicePan(float32(math.NaN()), 3)
	if pLNan != 1.0 || pRNan != 1.0 {
		t.Fatalf("computeVoicePan NaN spread expected 1,1: got %v,%v", pLNan, pRNan)
	}

	for _, sub := range []int{-5, -1, 0, 3, 6, 7, 10} {
		r := unisonDetuneRatio(sub, 7, 0.5)
		if math.IsNaN(r) || r < 0.5 || r > 2.0 {
			t.Fatalf("unisonDetuneRatio invalid for sub %d: %v", sub, r)
		}
		sL, sR := unisonSubPan(sub, 7, 0.5)
		if math.IsNaN(sL) || math.IsNaN(sR) || sL < 0 || sR < 0 {
			t.Fatalf("unisonSubPan invalid for sub %d: L=%v R=%v", sub, sL, sR)
		}
	}

	if LFOSquare != LFOSSquare {
		t.Fatal("LFOSquare != LFOSSquare")
	}

	lfo := voiceLFO{}
	lfo.trigger(12345)
	if val := lfo.step(math.NaN(), 0, LFOSine, 1.0/48000); math.IsNaN(val) {
		t.Fatal("lfo.step NaN rate returned NaN")
	}
	if val := lfo.step(5.0, math.NaN(), LFOSine, 1.0/48000); math.IsNaN(val) {
		t.Fatal("lfo.step NaN delay returned NaN")
	}
	if val := lfo.step(5.0, 0, LFOSine, 0); val != 0 {
		t.Fatalf("lfo.step dt=0 returned %v, want 0", val)
	}
	if val := lfo.step(5.0, 0, LFOSine, math.NaN()); val != 0 {
		t.Fatalf("lfo.step dt=NaN returned %v, want 0", val)
	}

	s := New(48000)
	s.SetParameters(map[string]float32{
		"unison_voices":    7,
		"unison_detune":    0.5,
		"unison_spread":    0.8,
		"pan_spread":       0.6,
		"lfo_waveform":     float32(LFOSquare),
		"lfo_rate":         20.0,
		"lfo_pitch_depth":  12.0,
		"lfo_filter_depth": 4.0,
		"lfo_amp_depth":    0.8,
		"filter_type":      float32(Filter24dBLadder),
		"sustain":          1.0,
	})
	buf := dsp.NewStereoBuffer(128)
	buf.SetLen(128)
	s.NoteOn(60, 0.9)
	for block := 0; block < 100; block++ {
		if block == 20 {
			s.SetParameters(map[string]float32{"unison_voices": 1})
		} else if block == 40 {
			s.SetParameters(map[string]float32{"unison_voices": 4, "unison_spread": 0.2})
		} else if block == 60 {
			s.SetParameters(map[string]float32{"lfo_pitch_depth": 0, "pan_spread": 0})
		}
		s.Process(buf, 1.0/48000)
		for i := 0; i < 128; i++ {
			if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
				t.Fatalf("dynamic automation produced NaN/Inf at block %d frame %d", block, i)
			}
			if math.IsNaN(float64(buf.Right[i])) || math.IsInf(float64(buf.Right[i]), 0) {
				t.Fatalf("dynamic automation right channel produced NaN/Inf at block %d frame %d", block, i)
			}
		}
	}
}
