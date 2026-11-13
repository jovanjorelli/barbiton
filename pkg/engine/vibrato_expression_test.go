package engine

import (
	"testing"

	"barbiton/pkg/score"
)

func fptr(v float64) *float64 { return &v }

func vibScore() *score.Score {
	return &score.Score{
		Metadata: score.Metadata{Title: "T", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{{
			ID:         "t",
			Name:       "Solo",
			Instrument: score.InstrumentDef{Type: "ocarina"},
			Volume:     0.8,
			Vibrato:    &score.Vibrato{Rate: 4.0, Depth: 0.05},
			Expression: fptr(0.6),
			Notes: []score.NoteEvent{
				{Time: 0, Duration: 0.5, Pitch: 69, Velocity: 0.8,
					Vibrato:    &score.Vibrato{Rate: 7.0, Depth: 0.08},
					Expression: fptr(0.9)},
				{Time: 0.5, Duration: 0.5, Pitch: 71, Velocity: 0.8},
			},
		}},
	}
}

func TestVibratoOverride(t *testing.T) {
	e, err := New(vibScore(), 48000, 256)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var first, second *noteEvent
	for i := range e.events {
		ev := &e.events[i]
		if !ev.on {
			continue
		}
		if first == nil {
			first = ev
		} else {
			second = ev
		}
	}
	if first == nil || second == nil {
		t.Fatal("missing scheduled note-on events")
	}
	if !first.params.HasVibrato || first.params.VibratoRate != 7.0 || first.params.VibratoDepth != 0.08 {
		t.Errorf("note vibrato = %+v, want 7.0/0.08", first.params)
	}
	if !second.params.HasVibrato || second.params.VibratoRate != 4.0 || second.params.VibratoDepth != 0.05 {
		t.Errorf("inherited vibrato = %+v, want 4.0/0.05", second.params)
	}
	e.ProcessFrames(e.TotalFrames(), nil)
}

func TestExpressionOverride(t *testing.T) {
	e, err := New(vibScore(), 48000, 256)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var first, second *noteEvent
	for i := range e.events {
		ev := &e.events[i]
		if !ev.on {
			continue
		}
		if first == nil {
			first = ev
		} else {
			second = ev
		}
	}
	if first == nil || second == nil {
		t.Fatal("missing scheduled note-on events")
	}
	if !first.params.HasExpression || first.params.Expression != 0.9 {
		t.Errorf("note expression = %+v, want 0.9", first.params)
	}
	if !second.params.HasExpression || second.params.Expression != 0.6 {
		t.Errorf("inherited expression = %+v, want 0.6", second.params)
	}
	e.ProcessFrames(e.TotalFrames(), nil)
}
