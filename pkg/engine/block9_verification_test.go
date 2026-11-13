package engine

import (
	"math"
	"testing"

	"barbiton/pkg/score"
)

func TestBlock9MultiTrackSystemRender(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{
			Title:         "Barbiton Block 9 Symphony",
			BPM:           120,
			TimeSignature: "4/4",
		},
		Tracks: []score.Track{
			{
				ID:         "sampler_piano",
				Name:       "Piano",
				Instrument: score.InstrumentDef{Type: "sampler", File: "../../test_data/samples/piano.sfz"},
				Volume:     0.8,
				Pan:        -0.3,
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 0.5, Pitch: 60, Velocity: 0.8},
					{Time: 0.5, Duration: 0.5, Pitch: 64, Velocity: 0.7},
				},
			},
			{
				ID:         "ocarina_flute",
				Name:       "Flute",
				Instrument: score.InstrumentDef{Type: "ocarina", Parameters: map[string]float32{"breath_noise": 0.2, "brightness": 0.8}},
				Volume:     0.7,
				Pan:        0.3,
				Notes: []score.NoteEvent{
					{Time: 0.25, Duration: 0.75, Pitch: 72, Velocity: 0.8},
				},
			},
			{
				ID:   "synth_subtractive",
				Name: "Strings Supersaw",
				Instrument: score.InstrumentDef{
					Type: "synth",
					Parameters: map[string]float32{
						"synth_mode":       0,
						"waveform1":        0,
						"waveform2":        0,
						"unison_voices":    5,
						"unison_detune":    0.2,
						"unison_spread":    0.6,
						"filter_type":      2,
						"cutoff":           4000,
						"resonance":        0.3,
						"filter_env_depth": 2.0,
					},
				},
				Volume: 0.75,
				Pan:    -0.5,
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 1.0, Pitch: 55, Velocity: 0.85},
				},
			},
			{
				ID:   "synth_fm",
				Name: "FM Slap Bass",
				Instrument: score.InstrumentDef{
					Type: "synth",
					Parameters: map[string]float32{
						"synth_mode":  1,
						"fm_ratio":    1.0,
						"fm_depth":    3.5,
						"fm_feedback": 0.2,
					},
				},
				Volume: 0.85,
				Pan:    0.0,
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 0.4, Pitch: 36, Velocity: 0.9},
					{Time: 0.5, Duration: 0.4, Pitch: 38, Velocity: 0.9},
				},
			},
			{
				ID:   "synth_pluck",
				Name: "Pluck Guitar",
				Instrument: score.InstrumentDef{
					Type: "synth",
					Parameters: map[string]float32{
						"synth_mode":          2,
						"pluck_damping":       0.3,
						"pluck_decay":         0.99,
						"pluck_brightness":    12000,
						"pluck_pick_position": 0.25,
					},
				},
				Volume: 0.8,
				Pan:    0.5,
				Notes: []score.NoteEvent{
					{Time: 0.1, Duration: 0.6, Pitch: 67, Velocity: 0.8},
				},
			},
			{
				ID:   "synth_lead",
				Name: "Sync PWM Lead",
				Instrument: score.InstrumentDef{
					Type: "synth",
					Parameters: map[string]float32{
						"synth_mode":    0,
						"waveform1":     1,
						"waveform2":     0,
						"pulse_width":   0.35,
						"portamento":    0.08,
						"unison_voices": 3,
						"unison_detune": 0.1,
					},
				},
				Volume: 0.7,
				Pan:    0.2,
				Notes: []score.NoteEvent{
					{Time: 0.3, Duration: 0.3, Pitch: 62, Velocity: 0.8},
					{Time: 0.6, Duration: 0.4, Pitch: 69, Velocity: 0.8},
				},
			},
			{
				ID:   "synth_pad",
				Name: "7-Voice Supersaw Pad",
				Instrument: score.InstrumentDef{
					Type: "synth",
					Parameters: map[string]float32{
						"synth_mode":    0,
						"waveform1":     0,
						"waveform2":     0,
						"unison_voices": 7,
						"unison_detune": 0.25,
						"unison_spread": 0.8,
						"filter_type":   2,
						"cutoff":        3500,
						"resonance":     0.2,
					},
				},
				Volume: 0.65,
				Pan:    -0.2,
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 1.2, Pitch: 48, Velocity: 0.75},
				},
			},
		},
	}

	if err := score.Validate(s); err != nil {
		t.Fatalf("score validation failed: %v", err)
	}

	sr := 48000.0
	eng, err := New(s, sr, 512)
	if err != nil {
		t.Fatalf("engine New failed: %v", err)
	}

	totalFrames := eng.TotalFrames()
	framesRendered := 0
	hasAudio := false

	eng.ProcessFrames(totalFrames, func(master []float32) {
		framesRendered += len(master) / 2
		for i, sample := range master {
			if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
				t.Fatalf("master sample %d is NaN/Inf", i)
			}
			if math.Abs(float64(sample)) > 0.001 {
				hasAudio = true
			}
		}
	})

	if framesRendered != totalFrames {
		t.Fatalf("expected %d frames rendered, got %d", totalFrames, framesRendered)
	}
	if !hasAudio {
		t.Fatal("rendered output is completely silent")
	}
	peakL, peakR := eng.Peak()
	if peakL <= 0 || peakR <= 0 {
		t.Fatalf("expected positive peaks, got L=%v R=%v", peakL, peakR)
	}
}

func TestBlock9MIDIAutomationCCLive(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "CC Live Test", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{{
			ID:         "synth_lead",
			Name:       "Lead",
			Instrument: score.InstrumentDef{Type: "synth", Parameters: map[string]float32{"cutoff": 2000, "resonance": 0.2}},
			Volume:     0.8,
			Notes: []score.NoteEvent{
				{Time: 0.0, Duration: 1.0, Pitch: 60, Velocity: 0.8},
			},
		}},
	}

	eng, err := New(s, 48000, 256)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	eng.ApplyCC(0, 1, 100)
	eng.ApplyCC(0, 74, 110)
	eng.ApplyCC(0, 71, 90)
	eng.ApplyCC(0, 11, 64)
	eng.ApplyCC(0, 64, 127)

	eng.ProcessFrames(256, nil)
	eng.ApplyCC(0, 64, 0)
	eng.ProcessFrames(256, nil)

	eng.ApplyCC(-1, 1, 50)
	eng.ApplyCC(99, 1, 50)
	eng.ApplyCC(0, 1, -10)
	eng.ApplyCC(0, 1, 200)

	allocs := testing.AllocsPerRun(100, func() {
		eng.ApplyCC(0, 1, 100)
		eng.ApplyCC(0, 74, 110)
		eng.ApplyCC(0, 71, 90)
		eng.ApplyCC(0, 11, 64)
		eng.ApplyCC(0, 64, 127)
	})
	if allocs != 0 {
		t.Fatalf("ApplyCC allocated %v objects, want 0", allocs)
	}

	peakL, peakR := eng.Peak()
	if peakL < 0 || peakR < 0 {
		t.Fatalf("invalid negative peak: %v, %v", peakL, peakR)
	}
}

func TestBlock9CCScheduledEvents(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "Scheduled CC", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{{
			ID:         "t1",
			Instrument: score.InstrumentDef{Type: "synth", Parameters: map[string]float32{"cutoff": 1000}},
			Volume:     0.8,
			CC: []score.CCEvent{
				{Time: 0.0, Controller: 1, Value: 80},
				{Time: 0.1, Controller: 74, Value: 120},
				{Time: 0.2, Controller: 11, Value: 60},
			},
			Notes: []score.NoteEvent{
				{Time: 0.0, Duration: 0.5, Pitch: 60, Velocity: 0.8},
			},
		}},
	}

	eng, err := New(s, 48000, 256)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	eng.ProcessFrames(eng.TotalFrames(), nil)
	if err := eng.Err(); err != nil {
		t.Fatalf("render error: %v", err)
	}
}

func TestBlock9ProcessFramesZeroAllocations(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "Zero Alloc", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{{
			ID: "t1",
			Instrument: score.InstrumentDef{
				Type: "synth",
				Parameters: map[string]float32{
					"synth_mode":    0,
					"unison_voices": 3,
					"filter_type":   2,
					"cutoff":        3000,
				},
			},
			Volume: 0.8,
			Notes: []score.NoteEvent{
				{Time: 0.0, Duration: 10.0, Pitch: 60, Velocity: 0.8},
			},
		}},
	}

	eng, err := New(s, 48000, 256)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	eng.ProcessBlock()
	eng.ProcessBlock()

	allocs := testing.AllocsPerRun(100, func() {
		eng.ProcessBlock()
	})

	if allocs != 0 {
		t.Fatalf("steady-state ProcessBlock allocated %v objects, want 0", allocs)
	}

	allocsFrames := testing.AllocsPerRun(100, func() {
		eng.ProcessFrames(256, nil)
	})

	if allocsFrames != 0 {
		t.Fatalf("steady-state ProcessFrames allocated %v objects, want 0", allocsFrames)
	}
}

func TestBlock9ParallelRenderingIdentity(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "Par Identity", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{
			{
				ID:         "t1",
				Instrument: score.InstrumentDef{Type: "ocarina", Parameters: map[string]float32{"brightness": 0.8}},
				Volume:     0.8,
				Pan:        -0.5,
				Notes:      []score.NoteEvent{{Time: 0.0, Duration: 0.5, Pitch: 60, Velocity: 0.8}},
			},
			{
				ID: "t2",
				Instrument: score.InstrumentDef{
					Type:       "synth",
					Parameters: map[string]float32{"synth_mode": 1, "fm_ratio": 2.0, "fm_depth": 3.0},
				},
				Volume: 0.7,
				Pan:    0.5,
				Notes:  []score.NoteEvent{{Time: 0.1, Duration: 0.5, Pitch: 67, Velocity: 0.8}},
			},
		},
	}

	seqEng, err := New(s, 48000, 512)
	if err != nil {
		t.Fatalf("seq New: %v", err)
	}
	parEng, err := New(s, 48000, 512)
	if err != nil {
		t.Fatalf("par New: %v", err)
	}
	parEng.Mixer().SetParallel(true)

	var seqAudio []float32
	seqEng.ProcessFrames(1024, func(chunk []float32) {
		seqAudio = append(seqAudio, chunk...)
	})

	var parAudio []float32
	parEng.ProcessFrames(1024, func(chunk []float32) {
		parAudio = append(parAudio, chunk...)
	})

	if len(seqAudio) != len(parAudio) {
		t.Fatalf("audio lengths differ: seq=%d par=%d", len(seqAudio), len(parAudio))
	}
	for i := range seqAudio {
		if math.Abs(float64(seqAudio[i]-parAudio[i])) > 1e-6 {
			t.Fatalf("sample %d mismatch: seq=%v par=%v", i, seqAudio[i], parAudio[i])
		}
	}
}

func TestBlock9CC74PluckBrightnessAutomation(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "Pluck Brightness Automation", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{{
			ID: "pluck1",
			Instrument: score.InstrumentDef{
				Type: "synth",
				Parameters: map[string]float32{
					"synth_mode":          2,
					"pluck_damping":       0.3,
					"pluck_decay":         0.99,
					"pluck_brightness":    500,
					"pluck_pick_position": 0.25,
				},
			},
			Volume: 0.8,
			Notes: []score.NoteEvent{
				{Time: 0.0, Duration: 0.5, Pitch: 60, Velocity: 0.8},
				{Time: 0.5, Duration: 0.5, Pitch: 60, Velocity: 0.8},
			},
			CC: []score.CCEvent{
				{Time: 0.49, Controller: 74, Value: 127},
			},
		}},
	}
	eng, err := New(s, 48000, 256)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	eng.ProcessFrames(eng.TotalFrames(), nil)
	if err := eng.Err(); err != nil {
		t.Fatalf("render error: %v", err)
	}
}
