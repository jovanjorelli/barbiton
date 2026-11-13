package score

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScoreApplyTimingOffsetsBeyondEnd(t *testing.T) {
	s := &Score{
		Tracks: []Track{
			{
				Notes: []NoteEvent{
					{Pitch: 60, Time: 0.5, Duration: 0.5, TimingOffsetMs: 1000},
				},
			},
		},
	}
	s.ApplyTimingOffsets()
	if s.Tracks[0].Notes[0].Time != 1.0 {
		t.Fatalf("expected clamped time 1.0, got %v", s.Tracks[0].Notes[0].Time)
	}
}

func TestScoreParseFileReadError(t *testing.T) {
	_, err := ParseFile(".")
	if err == nil {
		t.Fatal("expected error reading directory as file")
	}
}

func TestScoreValidateTrackSwingAndBreath(t *testing.T) {
	s1 := &Score{
		Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4"},
		Tracks: []Track{
			{
				ID:         "t1",
				Instrument: InstrumentDef{Type: "ocarina"},
				Swing:      -0.5,
				Notes:      []NoteEvent{{Pitch: 60, Time: 0, Duration: 1}},
			},
		},
	}
	if err := Validate(s1); err == nil {
		t.Fatal("expected error for swing < 0")
	}

	s2 := &Score{
		Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4"},
		Tracks: []Track{
			{
				ID:         "t1",
				Instrument: InstrumentDef{Type: "ocarina"},
				BreathMs:   -10,
				Notes:      []NoteEvent{{Pitch: 60, Time: 0, Duration: 1}},
			},
		},
	}
	if err := Validate(s2); err == nil {
		t.Fatal("expected error for breath < 0")
	}

	s3 := &Score{
		Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4"},
		Tracks: []Track{
			{
				ID:         "t1",
				Instrument: InstrumentDef{Type: "ocarina"},
				BreathMs:   math.Inf(1),
				Notes:      []NoteEvent{{Pitch: 60, Time: 0, Duration: 1}},
			},
		},
	}
	if err := Validate(s3); err == nil {
		t.Fatal("expected error for infinite breath")
	}
}

func TestScoreCCEventUnmarshal(t *testing.T) {
	var cc1 CCEvent
	if err := cc1.UnmarshalJSON([]byte(`{"time": 1.5, "cc": 11, "value": 64}`)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if cc1.Controller != 11 || cc1.Time != 1.5 || cc1.Value != 64 {
		t.Fatalf("unexpected unmarshaled cc: %+v", cc1)
	}

	var cc2 CCEvent
	if err := cc2.UnmarshalJSON([]byte(`{"time": 0.5, "controller": 7, "value": 100}`)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if cc2.Controller != 7 || cc2.Time != 0.5 || cc2.Value != 100 {
		t.Fatalf("unexpected unmarshaled cc: %+v", cc2)
	}

	var cc3 CCEvent
	if err := cc3.UnmarshalJSON([]byte(`{invalid json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestScoreCloneComplete(t *testing.T) {
	var nilScore *Score
	if nilScore.Clone() != nil {
		t.Fatal("expected nil Clone for nil score")
	}
	if nilScore.TransposedCopy() != nil {
		t.Fatal("expected nil TransposedCopy for nil score")
	}

	expr := 0.75
	sc := &Score{
		Metadata: Metadata{Title: "Full", BPM: 140, TimeSignature: "3/4"},
		Tracks: []Track{
			{
				ID:         "tr1",
				Instrument: InstrumentDef{Type: "synth", Parameters: map[string]float32{"cutoff": 2000}},
				Notes:      []NoteEvent{{Pitch: 60, Time: 0, Duration: 1}},
				Pedal:      []PedalEvent{{Time: 0, Down: true}},
				CC:         []CCEvent{{Time: 0, Controller: 1, Value: 100}},
				Vibrato:    &Vibrato{Rate: 5, Depth: 1},
				Expression: &expr,
				Room:       &Room{IsPreset: true, Preset: "hall"},
				Delay:      &TrackDelay{Seconds: 0.25, Feedback: 0.5},
			},
		},
	}
	cp := sc.Clone()
	if cp == nil || len(cp.Tracks) != 1 {
		t.Fatal("Clone failed")
	}
	if cp.Tracks[0].Instrument.Parameters["cutoff"] != 2000 {
		t.Fatal("parameters not cloned")
	}
	if cp.Tracks[0].Vibrato == nil || cp.Tracks[0].Vibrato.Rate != 5 {
		t.Fatal("vibrato not cloned")
	}
	if cp.Tracks[0].Expression == nil || *cp.Tracks[0].Expression != 0.75 {
		t.Fatal("expression not cloned")
	}
	if cp.Tracks[0].Room == nil || cp.Tracks[0].Room.Preset != "hall" {
		t.Fatal("room not cloned")
	}
	if cp.Tracks[0].Delay == nil || cp.Tracks[0].Delay.Seconds != 0.25 {
		t.Fatal("delay not cloned")
	}
	if len(cp.Tracks[0].Pedal) != 1 || len(cp.Tracks[0].CC) != 1 {
		t.Fatal("pedal or cc not cloned")
	}

	tc := sc.TransposedCopy()
	if tc == nil {
		t.Fatal("TransposedCopy failed")
	}
}

func TestScoreValidateTrackErrors(t *testing.T) {
	cases := []struct {
		name string
		mod  func(s *Score)
	}{
		{
			name: "empty_track_id",
			mod: func(s *Score) {
				s.Tracks[0].ID = "   "
			},
		},
		{
			name: "duplicate_track_id",
			mod: func(s *Score) {
				s.Tracks = append(s.Tracks, s.Tracks[0])
			},
		},
		{
			name: "empty_instrument_type",
			mod: func(s *Score) {
				s.Tracks[0].Instrument.Type = ""
			},
		},
		{
			name: "sampler_empty_file",
			mod: func(s *Score) {
				s.Tracks[0].Instrument.Type = "sampler"
				s.Tracks[0].Instrument.File = ""
			},
		},
		{
			name: "pan_out_of_range",
			mod: func(s *Score) {
				s.Tracks[0].Pan = 2.5
			},
		},
		{
			name: "volume_out_of_range",
			mod: func(s *Score) {
				s.Tracks[0].Volume = -0.5
			},
		},
		{
			name: "reverb_send_out_of_range",
			mod: func(s *Score) {
				s.Tracks[0].ReverbSend = 1.5
			},
		},
	}

	for _, tc := range cases {
		s := &Score{
			Metadata: Metadata{Title: "T", BPM: 120, TimeSignature: "4/4"},
			Tracks: []Track{
				{
					ID:         "t1",
					Instrument: InstrumentDef{Type: "ocarina"},
					Volume:     0.8,
					Notes:      []NoteEvent{{Pitch: 60, Time: 0, Duration: 1}},
				},
			},
		}
		tc.mod(s)
		if err := Validate(s); err == nil {
			t.Fatalf("case %s: expected validation error, got nil", tc.name)
		}
	}
}

func TestGuideMarkdownJSONBlocksValidity(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	guidePath := filepath.Join(root, "..", "..", "GUIDE.md")
	data, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("failed to read GUIDE.md: %v", err)
	}

	content := string(data)
	const fence = "```json"
	pos := 0
	found := 0
	for {
		start := strings.Index(content[pos:], fence)
		if start == -1 {
			break
		}
		start += pos + len(fence)
		end := strings.Index(content[start:], "```")
		if end == -1 {
			break
		}
		raw := strings.TrimSpace(content[start : start+end])
		pos = start + end + 3
		found++

		var generic any
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			t.Fatalf("block %d failed json.Unmarshal: %v\ncontent:\n%s", found, err, raw)
		}

		if strings.Contains(raw, `"metadata"`) && strings.Contains(raw, `"tracks"`) {
			sc, err := ParseJSON([]byte(raw))
			if err != nil {
				t.Fatalf("block %d failed ParseJSON: %v\ncontent:\n%s", found, err, raw)
			}
			if err := Validate(sc); err != nil {
				t.Fatalf("block %d failed Validate: %v\ncontent:\n%s", found, err, raw)
			}
		}
	}

	if found == 0 {
		t.Fatal("no json blocks found in GUIDE.md")
	}
}

func TestExamplesDirectoryScoresValidity(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	examplesDir := filepath.Join(root, "..", "..", "test_data", "examples")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("failed to read examples dir: %v", err)
	}
	tested := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(examplesDir, entry.Name())
		sc, err := ParseFile(path)
		if err != nil {
			t.Fatalf("failed to parse example %s: %v", entry.Name(), err)
		}
		if err := Validate(sc); err != nil {
			t.Fatalf("failed to validate example %s: %v", entry.Name(), err)
		}
		tested++
	}
	if tested == 0 {
		t.Fatal("no example JSON scores found to validate")
	}
}
