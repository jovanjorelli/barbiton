package synth

import (
	"math"
	"testing"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
)

func dftMagnitude(signal []float32, freq float64, sampleRate float64) float64 {
	var re, im float64
	w := 2 * math.Pi * freq / sampleRate
	for n, s := range signal {
		val := float64(s)
		re += val * math.Cos(w*float64(n))
		im -= val * math.Sin(w*float64(n))
	}
	return math.Hypot(re, im) / float64(len(signal))
}

func measurePitchAutocorr(signal []float32, targetFreq, sampleRate float64) float64 {
	nominalPeriod := sampleRate / targetFreq
	nominalLag := int(math.Round(nominalPeriod))

	start := int(sampleRate * 0.1)
	if start+nominalLag*4 > len(signal) {
		start = 0
	}
	windowLen := nominalLag * 8
	if start+windowLen+nominalLag+10 > len(signal) {
		windowLen = len(signal) - start - nominalLag - 10
	}

	bestLag := nominalLag
	bestR := -1e18
	var rVals [7]float64
	lags := [7]int{
		nominalLag - 3,
		nominalLag - 2,
		nominalLag - 1,
		nominalLag,
		nominalLag + 1,
		nominalLag + 2,
		nominalLag + 3,
	}

	for idx, lag := range lags {
		if lag <= 0 {
			continue
		}
		var r float64
		for k := 0; k < windowLen; k++ {
			r += float64(signal[start+k]) * float64(signal[start+k+lag])
		}
		rVals[idx] = r
		if r > bestR {
			bestR = r
			bestLag = lag
		}
	}

	bestIdx := 3
	for idx, lag := range lags {
		if lag == bestLag {
			bestIdx = idx
			break
		}
	}

	if bestIdx > 0 && bestIdx < len(lags)-1 {
		y0 := rVals[bestIdx-1]
		y1 := rVals[bestIdx]
		y2 := rVals[bestIdx+1]
		denom := 2 * (2*y1 - y0 - y2)
		if math.Abs(denom) > 1e-12 {
			delta := (y2 - y0) / denom
			fracLag := float64(bestLag) + delta
			return sampleRate / fracLag
		}
	}

	return sampleRate / float64(bestLag)
}

func TestBlock8FMHarmonicSidebands(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr * 0.2)

	sZero := New(sr)
	sZero.SetParameters(map[string]float32{
		"synth_mode": 1,
		"fm_ratio":   1.0,
		"fm_depth":   0.0,
		"sustain":    1.0,
		"attack":     0.001,
	})
	bufZero := dsp.NewStereoBuffer(frames)
	bufZero.SetLen(frames)
	sZero.NoteOn(69, 0.8)
	sZero.Process(bufZero, dt)

	sigZero := bufZero.Left[int(sr*0.05):]
	magCarrierZero := dftMagnitude(sigZero, 440.0, sr)
	magSide1Zero := dftMagnitude(sigZero, 880.0, sr)
	magSide2Zero := dftMagnitude(sigZero, 1320.0, sr)

	if magCarrierZero < 0.2 {
		t.Fatalf("expected carrier magnitude > 0.2 at depth 0, got %v", magCarrierZero)
	}
	if magSide1Zero > 0.01 {
		t.Fatalf("expected sideband 1 magnitude ~ 0 at depth 0, got %v", magSide1Zero)
	}
	if magSide2Zero > 0.01 {
		t.Fatalf("expected sideband 2 magnitude ~ 0 at depth 0, got %v", magSide2Zero)
	}

	sFM := New(sr)
	sFM.SetParameters(map[string]float32{
		"synth_mode": 1,
		"fm_ratio":   1.0,
		"fm_depth":   2.0,
		"sustain":    1.0,
		"attack":     0.001,
		"fm_sustain": 1.0,
	})
	bufFM := dsp.NewStereoBuffer(frames)
	bufFM.SetLen(frames)
	sFM.NoteOn(69, 0.8)
	sFM.Process(bufFM, dt)

	sigFM := bufFM.Left[int(sr*0.05):]
	magSide1FM := dftMagnitude(sigFM, 880.0, sr)
	magSide2FM := dftMagnitude(sigFM, 1320.0, sr)

	if magSide1FM < 0.05 {
		t.Fatalf("expected strong sideband 1 at depth 2.0, got %v", magSide1FM)
	}
	if magSide2FM < 0.02 {
		t.Fatalf("expected strong sideband 2 at depth 2.0, got %v", magSide2FM)
	}

	sRatio2 := New(sr)
	sRatio2.SetParameters(map[string]float32{
		"synth_mode": 1,
		"fm_ratio":   2.0,
		"fm_depth":   2.5,
		"sustain":    1.0,
		"attack":     0.001,
		"fm_sustain": 1.0,
	})
	bufRatio2 := dsp.NewStereoBuffer(frames)
	bufRatio2.SetLen(frames)
	sRatio2.NoteOn(69, 0.8)
	sRatio2.Process(bufRatio2, dt)

	sigRatio2 := bufRatio2.Left[int(sr*0.05):]
	magHarmonic3 := dftMagnitude(sigRatio2, 1320.0, sr)
	if magHarmonic3 < 0.015 {
		t.Fatalf("expected ratio 2.0 sideband at 1320 Hz, got %v", magHarmonic3)
	}
}

func TestBlock8FMFeedbackPeakBounds(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr)

	sFB := New(sr)
	sFB.SetParameters(map[string]float32{
		"synth_mode":  1,
		"fm_ratio":    1.0,
		"fm_depth":    8.0,
		"fm_feedback": 1.0,
		"sustain":     1.0,
		"attack":      0.001,
		"fm_sustain":  1.0,
	})
	buf := dsp.NewStereoBuffer(frames)
	buf.SetLen(frames)
	sFB.NoteOn(60, 1.0)
	sFB.Process(buf, dt)

	maxPeak := float32(0)
	for i := 0; i < frames; i++ {
		valL := buf.Left[i]
		valR := buf.Right[i]
		if math.IsNaN(float64(valL)) || math.IsInf(float64(valL), 0) {
			t.Fatalf("NaN/Inf in Left at frame %d", i)
		}
		if math.IsNaN(float64(valR)) || math.IsInf(float64(valR), 0) {
			t.Fatalf("NaN/Inf in Right at frame %d", i)
		}
		absL := float32(math.Abs(float64(valL)))
		if absL > maxPeak {
			maxPeak = absL
		}
	}
	if maxPeak > 1.02 {
		t.Fatalf("FM with max feedback exceeded bounded limit: %v", maxPeak)
	}
	if maxPeak < 0.2 {
		t.Fatalf("FM with max feedback produced silence: %v", maxPeak)
	}

	sClamp := New(sr)
	sClamp.SetParameters(map[string]float32{
		"synth_mode":  1,
		"fm_feedback": 3.5,
		"fm_ratio":    50.0,
		"fm_depth":    30.0,
	})
	if sClamp.fmFeedback > 1.0 {
		t.Fatalf("fm_feedback not clamped to 1.0: %v", sClamp.fmFeedback)
	}
	if sClamp.fmRatio > 32.0 {
		t.Fatalf("fm_ratio not clamped to 32.0: %v", sClamp.fmRatio)
	}
	if sClamp.fmDepth > 16.0 {
		t.Fatalf("fm_depth not clamped to 16.0: %v", sClamp.fmDepth)
	}
}

func TestBlock8KarplusStrongPitchIntonation(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr * 0.4)

	testPitches := []int{45, 52, 57, 60, 69, 76}

	for _, pitch := range testPitches {
		expectedFreq := midiFreq(pitch)

		s := New(sr)
		s.SetParameters(map[string]float32{
			"synth_mode":          2,
			"pluck_damping":       0.5,
			"pluck_decay":         0.995,
			"pluck_brightness":    12000.0,
			"pluck_pick_position": 0.25,
		})
		buf := dsp.NewStereoBuffer(frames)
		buf.SetLen(frames)
		s.NoteOn(pitch, 0.9)
		s.Process(buf, dt)

		measuredFreq := measurePitchAutocorr(buf.Left, expectedFreq, sr)
		centDiff := 1200.0 * math.Abs(math.Log2(measuredFreq/expectedFreq))

		if centDiff > 0.5 {
			t.Fatalf("pitch %d expected %v Hz, measured %v Hz, diff = %v cents (exceeds 0.5 cent limit)",
				pitch, expectedFreq, measuredFreq, centDiff)
		}
	}
}

func TestBlock8KarplusStrongDampingDecay(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := int(sr * 0.5)

	sBright := New(sr)
	sBright.SetParameters(map[string]float32{
		"synth_mode":          2,
		"pluck_damping":       0.0,
		"pluck_decay":         0.995,
		"pluck_brightness":    20000.0,
		"pluck_pick_position": 0.0,
	})
	bufBright := dsp.NewStereoBuffer(frames)
	bufBright.SetLen(frames)
	sBright.NoteOn(69, 0.8)
	sBright.Process(bufBright, dt)

	sDamped := New(sr)
	sDamped.SetParameters(map[string]float32{
		"synth_mode":          2,
		"pluck_damping":       1.0,
		"pluck_decay":         0.995,
		"pluck_brightness":    20000.0,
		"pluck_pick_position": 0.0,
	})
	bufDamped := dsp.NewStereoBuffer(frames)
	bufDamped.SetLen(frames)
	sDamped.NoteOn(69, 0.8)
	sDamped.Process(bufDamped, dt)

	tailStart := int(sr * 0.2)
	sigBrightTail := bufBright.Left[tailStart:]
	sigDampedTail := bufDamped.Left[tailStart:]

	f0 := midiFreq(69)
	magHarmonic8Bright := dftMagnitude(sigBrightTail, f0*8, sr)
	magHarmonic8Damped := dftMagnitude(sigDampedTail, f0*8, sr)

	if magHarmonic8Damped >= magHarmonic8Bright*0.1 {
		t.Fatalf("expected damped 8th harmonic < 0.1 * bright, got damped=%v bright=%v",
			magHarmonic8Damped, magHarmonic8Bright)
	}

	sPick := New(sr)
	sPick.SetParameters(map[string]float32{
		"synth_mode":          2,
		"pluck_damping":       0.0,
		"pluck_decay":         0.998,
		"pluck_brightness":    20000.0,
		"pluck_pick_position": 0.5,
	})
	bufPick := dsp.NewStereoBuffer(frames)
	bufPick.SetLen(frames)
	sPick.NoteOn(69, 0.8)
	sPick.Process(bufPick, dt)

	mag2ndHarmonicPick := dftMagnitude(bufPick.Left[:int(sr*0.1)], f0*2, sr)
	mag1stHarmonicPick := dftMagnitude(bufPick.Left[:int(sr*0.1)], f0, sr)

	if mag2ndHarmonicPick > mag1stHarmonicPick*0.3 {
		t.Fatalf("pick position 0.5 did not notch 2nd harmonic: 1st=%v 2nd=%v",
			mag1stHarmonicPick, mag2ndHarmonicPick)
	}
}

func TestBlock8SustainPedalHoldAndRelease(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr

	modes := []int{0, 1, 2}
	for _, mode := range modes {
		s := New(sr)
		var _ instruments.PedalVoicer = s
		s.SetParameters(map[string]float32{
			"synth_mode": float32(mode),
			"release":    0.05,
		})

		buf1 := dsp.NewStereoBuffer(512)
		buf1.SetLen(512)

		s.SetPedal(true)
		if !s.PedalDown() {
			t.Fatalf("mode %d: expected PedalDown == true", mode)
		}

		s.NoteOn(60, 0.8)
		s.NoteOff(60)

		s.Process(buf1, dt)
		if s.ActiveVoices() != 1 {
			t.Fatalf("mode %d: voice released while pedal held down", mode)
		}

		var sumSq float64
		for i := 0; i < 512; i++ {
			sumSq += float64(buf1.Left[i]) * float64(buf1.Left[i])
		}
		if sumSq < 1e-6 {
			t.Fatalf("mode %d: buffer silent while pedal held down", mode)
		}

		s.SetPedal(false)
		if s.PedalDown() {
			t.Fatalf("mode %d: expected PedalDown == false", mode)
		}

		buf2 := dsp.NewStereoBuffer(int(sr * 0.2))
		buf2.SetLen(int(sr * 0.2))
		s.Process(buf2, dt)

		if s.ActiveVoices() != 0 {
			t.Fatalf("mode %d: voice not freed after pedal release, got %d active", mode, s.ActiveVoices())
		}
	}
}

func TestBlock8ZeroAllocations(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	buf := dsp.NewStereoBuffer(512)
	buf.SetLen(512)

	modes := []int{0, 1, 2}
	for _, mode := range modes {
		s := New(sr)
		var _ instruments.PedalVoicer = s
		s.SetParameters(map[string]float32{
			"synth_mode":          float32(mode),
			"pan_spread":          0.5,
			"fm_depth":            3.0,
			"fm_ratio":            2.0,
			"pluck_damping":       0.5,
			"pluck_decay":         0.99,
			"pluck_brightness":    8000.0,
			"pluck_pick_position": 0.2,
		})

		for p := 0; p < 16; p++ {
			s.NoteOn(40+p, 0.8)
		}

		allocs := testing.AllocsPerRun(100, func() {
			s.Process(buf, dt)
		})

		if allocs != 0 {
			t.Fatalf("mode %d: expected 0 allocs per Process call, got %v", mode, allocs)
		}
	}
}

func TestBlock8BackwardCompatibility(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	frames := 1024

	sDefault := New(sr)
	if sDefault.synthMode != 0 {
		t.Fatalf("expected default synth_mode == 0, got %d", sDefault.synthMode)
	}

	bufDefault := dsp.NewStereoBuffer(frames)
	bufDefault.SetLen(frames)
	sDefault.NoteOn(60, 0.8)
	sDefault.Process(bufDefault, dt)

	var sumSq float64
	for i := 0; i < frames; i++ {
		sumSq += float64(bufDefault.Left[i]) * float64(bufDefault.Left[i])
		if bufDefault.Left[i] != bufDefault.Right[i] {
			t.Fatalf("default synth output not center panned at frame %d", i)
		}
	}
	if sumSq == 0 {
		t.Fatal("default synth produced silence")
	}
}

func TestBlock8PluckInitialTransientContinuity(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	pitches := []int{21, 36, 45, 52, 60, 69, 72, 84, 96, 108}
	for _, pitch := range pitches {
		s := New(sr)
		s.SetParameters(map[string]float32{
			"synth_mode":          2,
			"pluck_damping":       0.5,
			"pluck_decay":         0.99,
			"pluck_brightness":    10000.0,
			"pluck_pick_position": 0.2,
		})
		buf := dsp.NewStereoBuffer(128)
		buf.SetLen(128)
		s.NoteOn(pitch, 0.8)
		s.Process(buf, dt)

		if math.Abs(float64(buf.Left[0])) < 1e-7 {
			t.Fatalf("pitch %d: initial sample is silent (%v)", pitch, buf.Left[0])
		}
		for i := 0; i < 128; i++ {
			if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
				t.Fatalf("pitch %d: frame %d is NaN/Inf", pitch, i)
			}
		}
	}
}

func TestBlock8NoteOnZeroAllocations(t *testing.T) {
	sr := 48000.0
	modes := []int{0, 1, 2}
	for _, mode := range modes {
		s := New(sr)
		s.SetParameters(map[string]float32{
			"synth_mode": float32(mode),
		})
		allocsNoteOn := testing.AllocsPerRun(100, func() {
			s.NoteOn(60, 0.8)
		})
		if allocsNoteOn != 0 {
			t.Fatalf("mode %d: NoteOn allocated %v objects", mode, allocsNoteOn)
		}
		allocsNoteOff := testing.AllocsPerRun(100, func() {
			s.NoteOff(60)
		})
		if allocsNoteOff != 0 {
			t.Fatalf("mode %d: NoteOff allocated %v objects", mode, allocsNoteOff)
		}
		allocsPedal := testing.AllocsPerRun(100, func() {
			s.SetPedal(true)
			s.SetPedal(false)
		})
		if allocsPedal != 0 {
			t.Fatalf("mode %d: SetPedal allocated %v objects", mode, allocsPedal)
		}
	}
}

func TestBlock8ParameterEdgeCases(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	s := New(sr)
	nan := float32(math.NaN())
	s.SetParameters(map[string]float32{
		"synth_mode":          nan,
		"fm_ratio":            nan,
		"fm_depth":            nan,
		"fm_feedback":         nan,
		"pluck_damping":       nan,
		"pluck_decay":         nan,
		"pluck_brightness":    nan,
		"pluck_pick_position": nan,
	})
	if s.synthMode != 0 {
		t.Fatalf("expected synth_mode 0 on NaN, got %d", s.synthMode)
	}
	if math.Abs(s.fmRatio-0.1) > 1e-6 {
		t.Fatalf("expected fm_ratio 0.1 on NaN, got %v", s.fmRatio)
	}
	if s.pluckDamping != 0.0 {
		t.Fatalf("expected pluck_damping 0.0 on NaN, got %v", s.pluckDamping)
	}

	buf := dsp.NewStereoBuffer(256)
	buf.SetLen(256)
	s.NoteOn(60, 0.8)
	s.Process(buf, dt)
	for i := 0; i < 256; i++ {
		if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
			t.Fatalf("NaN/Inf in output on NaN parameters at frame %d", i)
		}
	}

	s.SetParameters(map[string]float32{
		"mode":       2,
		"synth_mode": 1,
	})
	if s.synthMode != 1 {
		t.Fatalf("canonical synth_mode did not override alias mode, got %d", s.synthMode)
	}

	s.SetParameters(map[string]float32{
		"fm_index": 5.0,
		"fm_depth": 3.0,
	})
	if s.fmDepth != 3.0 {
		t.Fatalf("canonical fm_depth did not override alias fm_index, got %v", s.fmDepth)
	}

	s.SetParameters(map[string]float32{
		"pluck_damp":    0.8,
		"pluck_damping": 0.3,
	})
	if math.Abs(s.pluckDamping-0.3) > 1e-5 {
		t.Fatalf("canonical pluck_damping did not override alias, got %v", s.pluckDamping)
	}

	s.SetParameters(map[string]float32{
		"pluck_pick_pos":      0.9,
		"pluck_pick_position": 0.4,
	})
	if math.Abs(s.pluckPickPosition-0.4) > 1e-5 {
		t.Fatalf("canonical pluck_pick_position did not override alias, got %v", s.pluckPickPosition)
	}
}

func TestBlock8PolyphonicVoiceStealingModes(t *testing.T) {
	sr := 48000.0
	dt := 1.0 / sr
	buf := dsp.NewStereoBuffer(256)
	buf.SetLen(256)

	modes := []int{1, 2}
	for _, mode := range modes {
		s := New(sr)
		s.SetParameters(map[string]float32{
			"synth_mode": float32(mode),
		})
		for p := 0; p < 24; p++ {
			s.NoteOn(30+p, 0.7)
		}
		if s.ActiveVoices() != 16 {
			t.Fatalf("mode %d: expected 16 active voices after 24 NoteOns, got %d", mode, s.ActiveVoices())
		}
		s.Process(buf, dt)
		for i := 0; i < 256; i++ {
			if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
				t.Fatalf("mode %d: NaN/Inf after voice stealing at frame %d", mode, i)
			}
		}
	}
}
