package engine

import (
	"testing"

	"barbiton/pkg/score"
)

func TestEngineTurntableTriggers(t *testing.T) {
	sc := &score.Score{
		Metadata: score.Metadata{
			Title:         "Turntable Trigger Test",
			BPM:           120,
			TimeSignature: "4/4",
			Turntable: &score.TrackTurntable{
				Enabled:   true,
				InertiaMs: 20,
				Triggers: []score.TurntableTrigger{
					{Time: 0.1, Type: "scratch", Speed: -1.5},
					{Time: 0.2, Type: "beat_roll", Subdivision: "1/8"},
					{Time: 0.3, Type: "brake", BrakeTimeMs: 100, Curve: "exponential"},
					{Time: 0.45, Type: "resume", Duration: 0.05, Curve: "linear"},
					{Time: 0.5, Type: "backspin", Speed: -3.0, DecayMs: 100},
				},
			},
		},
		Tracks: []score.Track{
			{
				ID:         "lead",
				Instrument: score.InstrumentDef{Type: "synth"},
				Volume:     0.8,
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 0.8, Pitch: 60, Velocity: 0.8},
				},
				Turntable: &score.TrackTurntable{Enabled: true, InertiaMs: 15},
				TurntableTriggers: []score.TurntableTrigger{
					{Time: 0.15, Type: "scratch", Speed: 2.0},
					{Time: 0.25, Type: "beat_roll", Subdivision: "1/16"},
					{Time: 0.35, Type: "tape_stop", BrakeTimeMs: 80},
				},
			},
		},
	}

	eng, err := New(sc, 48000, 128)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	frames := int(48000 * 0.7)
	rendered := 0
	eng.ProcessFrames(frames, func(master []float32) {
		rendered += len(master) / 2
	})
	if rendered != frames {
		t.Fatalf("rendered %d frames, want %d", rendered, frames)
	}
}

func TestEngineTurntableAutomation(t *testing.T) {
	sc := &score.Score{
		Metadata: score.Metadata{
			Title:         "Turntable Automation Test",
			BPM:           120,
			TimeSignature: "4/4",
			Turntable:     &score.TrackTurntable{Enabled: true},
		},
		Tracks: []score.Track{
			{
				ID:         "bass",
				Instrument: score.InstrumentDef{Type: "synth"},
				Volume:     0.8,
				Turntable:  &score.TrackTurntable{Enabled: true},
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 0.7, Pitch: 48, Velocity: 0.8},
				},
				Automation: score.AutomationMap{
					"scratch_speed": score.AutomationTrack{
						{Time: 0.1, Value: 0.0},
						{Time: 0.3, Value: -1.5},
					},
				},
			},
		},
	}

	eng, err := New(sc, 48000, 128)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	frames := int(48000 * 0.7)
	eng.ProcessFrames(frames, nil)
}

func TestEngineTurntableZeroAllocations(t *testing.T) {
	sc := &score.Score{
		Metadata: score.Metadata{
			Title:         "Turntable Zero Alloc",
			BPM:           120,
			TimeSignature: "4/4",
			Turntable: &score.TrackTurntable{
				Enabled: true,
				Triggers: []score.TurntableTrigger{
					{Time: 0.05, Type: "scratch", Speed: -1.0},
				},
			},
		},
		Tracks: []score.Track{
			{
				ID:         "lead",
				Instrument: score.InstrumentDef{Type: "synth"},
				Volume:     0.8,
				Turntable:  &score.TrackTurntable{Enabled: true},
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 2.0, Pitch: 60, Velocity: 0.8},
				},
			},
		},
	}

	eng, err := New(sc, 48000, 128)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	eng.ProcessFrames(256, nil)

	allocs := testing.AllocsPerRun(100, func() {
		eng.ProcessBlock()
	})
	if allocs != 0 {
		t.Fatalf("ProcessBlock with turntable allocated %g objects, want 0", allocs)
	}
}

func TestEngineTurntableTimedDurationRelease(t *testing.T) {
	sc := &score.Score{
		Metadata: score.Metadata{
			Title:         "Turntable Timed Test",
			BPM:           120,
			TimeSignature: "4/4",
			Turntable: &score.TrackTurntable{
				Enabled: true,
				Triggers: []score.TurntableTrigger{
					{Time: 0.1, Type: "beat_roll", Subdivision: "1/16", Duration: 0.1},
				},
			},
		},
		Tracks: []score.Track{
			{
				ID:         "lead",
				Instrument: score.InstrumentDef{Type: "synth"},
				Volume:     0.8,
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 1.0, Pitch: 60, Velocity: 0.8},
				},
			},
		},
	}

	eng, err := New(sc, 48000, 128)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	eng.ProcessFrames(int(48000*0.15), nil)
	tt := eng.mix.Turntable()
	if tt == nil {
		t.Fatal("turntable is nil")
	}
	if tt.Mode() != 4 {
		t.Fatalf("expected beat_roll mode (4) during active duration, got %v", tt.Mode())
	}

	eng.ProcessFrames(int(48000*0.1), nil)
	if tt.Mode() != 0 {
		t.Fatalf("expected bypass mode (0) after duration expired, got %v", tt.Mode())
	}
}
