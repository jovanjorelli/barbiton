package synth

import (
	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
	"math"
	"testing"
)

func TestSynthCoverageEdges(t *testing.T) {
	s := New(48000)

	s.NoteOnParams(60, 0.8, instruments.NoteParams{
		HasExpression: true,
		Expression:    float32(math.NaN()),
	})

	s.NoteOnParams(62, 0.8, instruments.NoteParams{
		HasExpression: true,
		Expression:    -0.5,
	})

	s.NoteOnParams(64, 0.8, instruments.NoteParams{
		HasExpression: true,
		Expression:    1.5,
		Env: instruments.EnvelopeOverride{
			HasDecay:   true,
			Decay:      0.2,
			HasRelease: true,
			Release:    0.3,
		},
	})

	buf := &dsp.Buffer{
		Left:  make([]float32, 128),
		Right: make([]float32, 64),
	}
	s.Process(buf, 1.0/48000.0)
}

func TestUnisonHelpersCoverage(t *testing.T) {
	for _, spread := range []float32{-1, 0, 0.5, 1.5, float32(math.NaN())} {
		for _, idx := range []int{-5, 0, 3, 7, 15} {
			l, r := computeVoicePan(spread, idx)
			if math.IsNaN(l) || math.IsNaN(r) {
				t.Fatalf("computeVoicePan returned NaN: %v, %v", l, r)
			}
		}
	}

	for _, v := range []int{0, 1, 3, 7, 10} {
		if norm := unisonPowerNorm(v); norm <= 0 || math.IsNaN(norm) {
			t.Fatalf("unisonPowerNorm(%d) = %v", v, norm)
		}
		for _, det := range []float32{-0.5, 0, 0.5, 1.5, float32(math.NaN())} {
			for _, sub := range []int{-2, 0, 2, v, v + 2} {
				ratio := unisonDetuneRatio(sub, v, det)
				if ratio <= 0 || math.IsNaN(ratio) {
					t.Fatalf("unisonDetuneRatio(%d, %d, %v) = %v", sub, v, det, ratio)
				}
			}
		}
		for _, sp := range []float32{-0.5, 0, 0.5, 1.5, float32(math.NaN())} {
			for _, sub := range []int{-2, 0, 2, v, v + 2} {
				l, r := unisonSubPan(sub, v, sp)
				if math.IsNaN(l) || math.IsNaN(r) {
					t.Fatalf("unisonSubPan(%d, %d, %v) = %v, %v", sub, v, sp, l, r)
				}
			}
		}
	}
}

func TestLFOHelpersCoverage(t *testing.T) {
	for _, val := range []float32{float32(math.NaN()), -2, 0, 1, 2, 3, 4, 10} {
		_ = lfoWaveParam(val)
	}

	var lfo voiceLFO
	lfo.trigger(0)
	lfo.trigger(0x12345)

	for _, dt := range []float64{-1, 0, math.NaN(), 1.0 / 48000.0} {
		for _, wave := range []LFOWaveform{LFOSine, LFOTriangle, LFOSaw, LFOSquare, LFOSampleAndHold} {
			_ = lfo.step(5.0, 0.1, wave, dt)
			_ = lfo.step(5.0, 0.0, wave, dt)
		}
	}
}

func TestSynthFMCoverageEdges(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"synth_mode":      1,
		"fm_ratio":        2.0,
		"fm_depth":        4.0,
		"fm_feedback":     0.5,
		"lfo_pitch_depth": 2.0,
		"lfo_amp_depth":   0.5,
		"lfo_rate":        6.0,
		"pan_spread":      0.8,
	})

	s.NoteOnParams(60, 0.8, instruments.NoteParams{
		HasVibrato:   true,
		VibratoRate:  5.0,
		VibratoDepth: 1.5,
	})
	s.NoteOn(64, 0.7)

	buf := &dsp.Buffer{
		Left:  make([]float32, 256),
		Right: make([]float32, 256),
	}
	for i := 0; i < 50; i++ {
		s.Process(buf, 1.0/48000.0)
	}
	s.NoteOff(60)
	s.NoteOff(64)
	for i := 0; i < 100; i++ {
		s.Process(buf, 1.0/48000.0)
	}
}

func TestSynthPluckCoverageEdges(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"synth_mode":       2,
		"pluck_damp":       0.8,
		"pluck_decay":      0.95,
		"pluck_brightness": 100,
		"pluck_pick_pos":   0.0,
	})
	s.NoteOn(10, 0.000001)

	s.SetParameters(map[string]float32{
		"pluck_brightness":    30000,
		"pluck_pick_position": 0.999,
	})
	s.NoteOn(120, 0.9)

	buf := &dsp.Buffer{
		Left:  make([]float32, 256),
		Right: make([]float32, 256),
	}
	for i := 0; i < 20; i++ {
		s.Process(buf, 1.0/48000.0)
	}
	s.NoteOff(10)
	s.NoteOff(120)
	for i := 0; i < 50; i++ {
		s.Process(buf, 1.0/48000.0)
	}
}

func TestSynthAliasesCoverage(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"mode":          0,
		"pw":            0.3,
		"pwm":           0.4,
		"pulse_width1":  0.35,
		"pulse_width2":  0.65,
		"fm_index":      3.0,
		"pluck_damping": 0.2,
		"glide":         0.1,
	})

	for _, modeVal := range []float32{-1, 5, float32(math.NaN())} {
		s.SetParameters(map[string]float32{"mode": modeVal})
		s.SetParameters(map[string]float32{"synth_mode": modeVal})
	}

	s.Process(nil, 1.0/48000.0)
	s.Process(&dsp.Buffer{}, -1)

	for _, fType := range []float32{0, 1, 2, 3} {
		s.SetParameters(map[string]float32{
			"filter_type":      fType,
			"filter_env_depth": 0,
			"lfo_filter_depth": 0,
		})
		s.NoteOn(60, 0.8)
		buf := &dsp.Buffer{Left: make([]float32, 64), Right: make([]float32, 64)}
		s.Process(buf, 1.0/48000.0)
		s.NoteOff(60)

		s.SetParameters(map[string]float32{
			"filter_type":      fType,
			"filter_env_depth": 2.0,
			"lfo_filter_depth": 1.5,
			"unison_voices":    5,
			"unison_spread":    0.7,
			"unison_detune":    0.2,
		})
		s.NoteOn(62, 0.8)
		s.Process(buf, 1.0/48000.0)
		s.NoteOff(62)
	}

	lowSR := New(1000)
	lowSR.SetParameters(map[string]float32{"cutoff": 10000})

	sg := New(48000)
	sg.SetParameters(map[string]float32{
		"portamento":    0.01,
		"lfo_amp_depth": 0.5,
		"lfo_rate":      5.0,
	})
	sg.NoteOn(60, 0.8)
	buf := &dsp.Buffer{Left: make([]float32, 256), Right: make([]float32, 256)}
	sg.Process(buf, 1.0/48000.0)
	sg.NoteOn(67, 0.8)
	for i := 0; i < 600; i++ {
		sg.Process(buf, 1.0/48000.0)
	}
}
