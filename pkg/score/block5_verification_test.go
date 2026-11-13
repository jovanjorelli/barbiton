package score

import (
	"strings"
	"testing"
)

func TestTranspositionNonDestructiveAndIdempotent(t *testing.T) {
	origNotes := []NoteEvent{
		{Time: 0, Duration: 0.5, Pitch: 60, Velocity: 0.8},
		{Time: 0.5, Duration: 0.5, Pitch: 64, Velocity: 0.8},
	}
	s := &Score{
		Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4", Transpose: 5},
		Tracks: []Track{
			{
				ID:         "t1",
				Instrument: InstrumentDef{Type: "ocarina"},
				Transpose:  2,
				Notes:      origNotes,
			},
		},
	}

	if origNotes[0].Pitch != 60 || origNotes[1].Pitch != 64 {
		t.Fatalf("unexpected initial pitch")
	}

	s.ApplyTransposition()

	if origNotes[0].Pitch != 60 || origNotes[1].Pitch != 64 {
		t.Fatalf("original notes slice was mutated in-place: %v, %v", origNotes[0].Pitch, origNotes[1].Pitch)
	}

	if s.Tracks[0].Notes[0].Pitch != 67 || s.Tracks[0].Notes[1].Pitch != 71 {
		t.Fatalf("expected transposed pitches 67 and 71, got %d, %d", s.Tracks[0].Notes[0].Pitch, s.Tracks[0].Notes[1].Pitch)
	}
	if s.Tracks[0].Notes[0].OriginalPitch != 60 || s.Tracks[0].Notes[1].OriginalPitch != 64 {
		t.Fatalf("OriginalPitch not preserved: %d, %d", s.Tracks[0].Notes[0].OriginalPitch, s.Tracks[0].Notes[1].OriginalPitch)
	}
	if !s.Tracks[0].Notes[0].IsTransposed || !s.Tracks[0].Notes[1].IsTransposed {
		t.Fatalf("IsTransposed flag not set")
	}

	s.ApplyTransposition()

	if s.Tracks[0].Notes[0].Pitch != 67 || s.Tracks[0].Notes[1].Pitch != 71 {
		t.Fatalf("repeated ApplyTransposition compounded pitches: %d, %d", s.Tracks[0].Notes[0].Pitch, s.Tracks[0].Notes[1].Pitch)
	}
}

func TestValidationOrderIndependence(t *testing.T) {
	buildInvalid := func() *Score {
		return &Score{
			Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4", Transpose: 10},
			Tracks: []Track{
				{
					ID:         "t1",
					Name:       "Lead",
					Instrument: InstrumentDef{Type: "ocarina"},
					Transpose:  5,
					Notes: []NoteEvent{
						{Time: 0, Duration: 0.5, Pitch: 120, Velocity: 0.8},
					},
				},
			},
		}
	}

	sBefore := buildInvalid()
	errBefore := Validate(sBefore)
	if errBefore == nil {
		t.Fatal("expected error validating before ApplyTransposition")
	}
	if !strings.Contains(errBefore.Error(), "exceeds maximum 127") || !strings.Contains(errBefore.Error(), "resulting pitch 135") {
		t.Fatalf("unexpected error message: %v", errBefore)
	}

	sAfter := buildInvalid()
	sAfter.ApplyTransposition()
	errAfter := Validate(sAfter)
	if errAfter == nil {
		t.Fatal("expected error validating after ApplyTransposition")
	}
	if !strings.Contains(errAfter.Error(), "exceeds maximum 127") || !strings.Contains(errAfter.Error(), "resulting pitch 135") {
		t.Fatalf("unexpected error message: %v", errAfter)
	}

	buildValid := func() *Score {
		return &Score{
			Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4", Transpose: 5},
			Tracks: []Track{
				{
					ID:         "t1",
					Name:       "Lead",
					Instrument: InstrumentDef{Type: "ocarina"},
					Transpose:  2,
					Notes: []NoteEvent{
						{Time: 0, Duration: 0.5, Pitch: 60, Velocity: 0.8},
					},
				},
			},
		}
	}

	sValidBefore := buildValid()
	if err := Validate(sValidBefore); err != nil {
		t.Fatalf("valid score rejected before transposition: %v", err)
	}
	sValidBefore.ApplyTransposition()
	if err := Validate(sValidBefore); err != nil {
		t.Fatalf("valid score rejected after transposition: %v", err)
	}

	buildOriginallyInvalid := func(pitch, shift int) *Score {
		return &Score{
			Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4", Transpose: shift},
			Tracks: []Track{
				{
					ID:         "t1",
					Name:       "Lead",
					Instrument: InstrumentDef{Type: "ocarina"},
					Notes: []NoteEvent{
						{Time: 0, Duration: 0.5, Pitch: pitch, Velocity: 0.8},
					},
				},
			},
		}
	}

	sNegBefore := buildOriginallyInvalid(-5, 10)
	errNegBefore := Validate(sNegBefore)
	if errNegBefore == nil || !strings.Contains(errNegBefore.Error(), "got -5") {
		t.Fatalf("expected rejection of pitch -5 before transposition, got %v", errNegBefore)
	}
	sNegAfter := buildOriginallyInvalid(-5, 10)
	sNegAfter.ApplyTransposition()
	errNegAfter := Validate(sNegAfter)
	if errNegAfter == nil || !strings.Contains(errNegAfter.Error(), "got -5") {
		t.Fatalf("expected rejection of pitch -5 after transposition, got %v", errNegAfter)
	}

	sHighBefore := buildOriginallyInvalid(130, -10)
	errHighBefore := Validate(sHighBefore)
	if errHighBefore == nil || !strings.Contains(errHighBefore.Error(), "got 130") {
		t.Fatalf("expected rejection of pitch 130 before transposition, got %v", errHighBefore)
	}
	sHighAfter := buildOriginallyInvalid(130, -10)
	sHighAfter.ApplyTransposition()
	errHighAfter := Validate(sHighAfter)
	if errHighAfter == nil || !strings.Contains(errHighAfter.Error(), "got 130") {
		t.Fatalf("expected rejection of pitch 130 after transposition, got %v", errHighAfter)
	}
}

func TestScoreCloneAndTransposedCopy(t *testing.T) {
	orig := &Score{
		Metadata: Metadata{Title: "Original", BPM: 120, TimeSignature: "4/4", Transpose: 3},
		Tracks: []Track{
			{
				ID:         "t1",
				Name:       "Synth",
				Instrument: InstrumentDef{Type: "synth", Parameters: map[string]float32{"cutoff": 2000}},
				Transpose:  4,
				Pedal:      []PedalEvent{{Time: 0, Down: true}},
				Notes:      []NoteEvent{{Time: 0, Duration: 1, Pitch: 60, Velocity: 0.7}},
			},
		},
	}

	cp := orig.Clone()
	if cp == orig {
		t.Fatal("Clone returned same pointer")
	}
	if &cp.Tracks[0].Notes[0] == &orig.Tracks[0].Notes[0] {
		t.Fatal("Notes slice was shallow copied")
	}

	trans := orig.TransposedCopy()
	if orig.Tracks[0].Notes[0].Pitch != 60 {
		t.Fatalf("TransposedCopy mutated original score: got %d", orig.Tracks[0].Notes[0].Pitch)
	}
	if trans.Tracks[0].Notes[0].Pitch != 67 {
		t.Fatalf("TransposedCopy note pitch expected 67, got %d", trans.Tracks[0].Notes[0].Pitch)
	}
}
