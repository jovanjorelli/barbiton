package engine

import (
	"math"
	"testing"

	"barbiton/pkg/score"
)

func TestEngineAutomationCutoffAndPitchBend(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "AutoTest", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{
			{
				ID:         "lead",
				Instrument: score.InstrumentDef{Type: "synth"},
				Volume:     0.8,
				Pan:        0,
				Automation: score.AutomationMap{
					"cutoff": {
						{Time: 0.0, Value: 200.0},
						{Time: 0.5, Value: 5000.0, Curve: "bezier", Tension: 0.5},
					},
					"pitch_bend": {
						{Time: 0.0, Value: 0.0},
						{Time: 0.25, Value: 12.0, Curve: "linear"},
					},
				},
				Distortion: &score.TrackDistortion{
					Mode:  "tube",
					Drive: 2.0,
					Mix:   0.8,
				},
				OTT: &score.TrackOTT{
					Depth:     0.9,
					TimeScale: 1.0,
				},
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 1.0, Pitch: 60, Velocity: 0.8},
				},
			},
		},
	}

	eng, err := New(s, 48000, 256)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	var totalRendered int
	total := int(s.Duration() * 48000)
	for totalRendered < total {
		eng.ProcessBlock()
		totalRendered += 256
	}

	pkL, pkR := eng.Peak()
	if pkL <= 0 || pkR <= 0 {
		t.Fatalf("expected non-zero peaks with automation & effects: left=%g, right=%g", pkL, pkR)
	}
}

func TestEngineAutomationZeroAllocations(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "AutoAlloc", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{
			{
				ID:         "synth",
				Instrument: score.InstrumentDef{Type: "synth"},
				Volume:     0.8,
				Automation: score.AutomationMap{
					"cutoff": {
						{Time: 0.0, Value: 200.0},
						{Time: 1.0, Value: 5000.0, Curve: "exponential"},
					},
					"distortion_drive": {
						{Time: 0.0, Value: 1.0},
						{Time: 1.0, Value: 10.0, Curve: "linear"},
					},
				},
				Distortion: &score.TrackDistortion{
					Mode:  "diode",
					Drive: 1.0,
					Mix:   1.0,
				},
				OTT: &score.TrackOTT{
					Depth: 1.0,
				},
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 2.0, Pitch: 60, Velocity: 0.9},
				},
			},
		},
	}

	eng, err := New(s, 48000, 256)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	eng.ProcessBlock()

	allocs := testing.AllocsPerRun(100, func() {
		eng.ProcessBlock()
	})
	if allocs != 0 {
		t.Fatalf("ProcessBlock with active automation allocated %g objects, want 0", allocs)
	}
}

func TestEngineMasterOTT(t *testing.T) {
	s := &score.Score{
		Metadata: score.Metadata{
			Title:         "MasterOTT",
			BPM:           120,
			TimeSignature: "4/4",
			OTT: &score.TrackOTT{
				Depth:     1.0,
				TimeScale: 1.0,
			},
		},
		Tracks: []score.Track{
			{
				ID:         "inst",
				Instrument: score.InstrumentDef{Type: "ocarina"},
				Volume:     0.7,
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 0.5, Pitch: 69, Velocity: 0.8},
				},
			},
		},
	}

	eng, err := New(s, 48000, 256)
	if err != nil {
		t.Fatalf("failed to create engine with master OTT: %v", err)
	}
	if !eng.Mixer().OTTEnabled() {
		t.Fatal("expected master OTT to be enabled on mixer")
	}

	eng.ProcessBlock()
	pL, pR := eng.Peak()
	if math.IsNaN(float64(pL)) || math.IsNaN(float64(pR)) || pL <= 0 || pR <= 0 {
		t.Fatalf("invalid output with master OTT: left=%g, right=%g", pL, pR)
	}
}

func BenchmarkEngineAutomationProcess(b *testing.B) {
	s := &score.Score{
		Metadata: score.Metadata{Title: "BenchAuto", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{
			{
				ID:         "lead",
				Instrument: score.InstrumentDef{Type: "synth"},
				Volume:     0.8,
				Automation: score.AutomationMap{
					"cutoff": {
						{Time: 0.0, Value: 200.0},
						{Time: 2.0, Value: 8000.0, Curve: "bezier", Tension: 0.5},
					},
				},
				Distortion: &score.TrackDistortion{Mode: "tube", Drive: 3.0, Mix: 0.7},
				OTT:        &score.TrackOTT{Depth: 0.8},
				Notes: []score.NoteEvent{
					{Time: 0.0, Duration: 5.0, Pitch: 60, Velocity: 0.9},
				},
			},
		},
	}
	eng, err := New(s, 48000, 256)
	if err != nil {
		b.Fatal(err)
	}
	eng.ProcessBlock()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng.ProcessBlock()
	}
}
