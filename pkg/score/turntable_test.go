package score

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTurntableScoreDSLRoundtrip(t *testing.T) {
	jsonScore := `{
  "metadata": {
    "title": "Turntable DSL Test",
    "bpm": 128,
    "time_signature": "4/4",
    "turntable": {
      "enabled": true,
      "inertia_ms": 25,
      "triggers": [
        {"time": 1.0, "type": "brake", "brake_time_ms": 250, "curve": "exponential"},
        {"time": 2.0, "type": "scratch", "speed": -2.0},
        {"time": 3.0, "type": "beat_roll", "subdivision": "1/16"},
        {"time": 4.0, "type": "backspin", "speed": -3.0, "decay_ms": 800}
      ]
    }
  },
  "tracks": [
    {
      "id": "tr1",
      "name": "Deck A",
      "instrument": {"type": "synth"},
      "pan": 0,
      "volume": 0.9,
      "turntable": true,
      "turntable_triggers": [
        {"time": 0.5, "type": "tape_stop", "brake_time_ms": 100},
        {"time": 1.5, "type": "stutter", "subdivision": "1/32"}
      ],
      "notes": [
        {"time": 0.0, "duration": 4.0, "pitch": 60, "velocity": 0.8}
      ]
    }
  ]
}`

	sc, err := ParseJSON([]byte(jsonScore))
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}
	if err := Validate(sc); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if sc.Metadata.Turntable == nil || !sc.Metadata.Turntable.Enabled {
		t.Fatal("expected metadata turntable enabled")
	}
	if len(sc.Metadata.Turntable.Triggers) != 4 {
		t.Fatalf("expected 4 metadata triggers, got %d", len(sc.Metadata.Turntable.Triggers))
	}
	if sc.Tracks[0].Turntable == nil || !sc.Tracks[0].Turntable.Enabled {
		t.Fatal("expected track turntable enabled")
	}
	if len(sc.Tracks[0].TurntableTriggers) != 2 {
		t.Fatalf("expected 2 track triggers, got %d", len(sc.Tracks[0].TurntableTriggers))
	}

	cloned := sc.Clone()
	if cloned.Metadata.Turntable == nil || len(cloned.Metadata.Turntable.Triggers) != 4 {
		t.Fatal("clone failed for metadata turntable")
	}
	if cloned.Tracks[0].Turntable == nil || len(cloned.Tracks[0].TurntableTriggers) != 2 {
		t.Fatal("clone failed for track turntable")
	}

	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	sc2, err := ParseJSON(data)
	if err != nil {
		t.Fatalf("ParseJSON roundtrip failed: %v", err)
	}
	if len(sc2.Metadata.Turntable.Triggers) != 4 {
		t.Fatalf("roundtrip triggers count mismatch: %d", len(sc2.Metadata.Turntable.Triggers))
	}
}

func TestTurntableScoreValidation(t *testing.T) {
	baseScore := func() *Score {
		return &Score{
			Metadata: Metadata{Title: "Test", BPM: 120, TimeSignature: "4/4"},
			Tracks: []Track{
				{
					ID:         "t1",
					Instrument: InstrumentDef{Type: "synth"},
					Volume:     0.8,
					Notes:      []NoteEvent{{Time: 0, Duration: 1, Pitch: 60, Velocity: 0.8}},
				},
			},
		}
	}

	s1 := baseScore()
	s1.Metadata.Turntable = &TrackTurntable{
		Enabled:   true,
		InertiaMs: -5.0,
	}
	if err := Validate(s1); err == nil {
		t.Fatal("expected error for negative inertia_ms")
	}

	s2 := baseScore()
	s2.Metadata.Turntable = &TrackTurntable{
		Enabled: true,
		Triggers: []TurntableTrigger{
			{Time: -1.0, Type: "scratch"},
		},
	}
	if err := Validate(s2); err == nil {
		t.Fatal("expected error for negative trigger time")
	}

	s3 := baseScore()
	s3.Metadata.Turntable = &TrackTurntable{
		Enabled: true,
		Triggers: []TurntableTrigger{
			{Time: 1.0, Type: "invalid_type"},
		},
	}
	if err := Validate(s3); err == nil {
		t.Fatal("expected error for invalid trigger type")
	}

	s4 := baseScore()
	s4.Metadata.Turntable = &TrackTurntable{
		Enabled: true,
		Triggers: []TurntableTrigger{
			{Time: 1.0, Type: "scratch", Speed: 5.0},
		},
	}
	if err := Validate(s4); err == nil {
		t.Fatal("expected error for scratch speed > 3.0")
	}

	s5 := baseScore()
	s5.Tracks[0].TurntableTriggers = []TurntableTrigger{
		{Time: 1.0, Type: "brake", BrakeTimeMs: -10},
	}
	if err := Validate(s5); err == nil {
		t.Fatal("expected error for negative brake_time_ms")
	}

	s6 := baseScore()
	s6.Metadata.Turntable = &TrackTurntable{
		Enabled: true,
		Triggers: []TurntableTrigger{
			{Time: 1.0, Type: "brake", Curve: "invalid_curve"},
		},
	}
	if err := Validate(s6); err == nil {
		t.Fatal("expected error for invalid curve")
	}

	s7 := baseScore()
	s7.Metadata.Turntable = &TrackTurntable{
		Enabled: true,
		Triggers: []TurntableTrigger{
			{Time: 1.0, Type: "beat_roll", Subdivision: "unknown_subdivision"},
		},
	}
	if err := Validate(s7); err == nil {
		t.Fatal("expected error for invalid subdivision")
	}

	s8 := baseScore()
	s8.Metadata.Turntable = &TrackTurntable{
		Enabled: true,
		Triggers: []TurntableTrigger{
			{Time: 1.0, Type: "bypass"},
		},
	}
	if err := Validate(s8); err != nil {
		t.Fatalf("expected bypass to be valid, got %v", err)
	}
}

func TestTurntableActionAliasUnmarshal(t *testing.T) {
	jsonScore := `{
  "metadata": {
    "title": "Action Alias",
    "bpm": 120,
    "time_signature": "4/4",
    "turntable": {
      "enabled": true,
      "triggers": [
        {"at": 0.5, "action": "brake", "brake_time_ms": 200, "curve": "exponential"},
        {"at": 1.0, "action": "beat_roll", "subdivision": "1/16", "duration_ms": 500}
      ]
    }
  },
  "tracks": [
    {
      "id": "t",
      "instrument": {"type": "synth"},
      "volume": 0.8,
      "notes": [{"time": 0, "duration": 1, "pitch": 60, "velocity": 0.8}]
    }
  ]
}`
	sc, err := ParseJSON([]byte(jsonScore))
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}
	if err := Validate(sc); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if sc.Metadata.Turntable == nil || len(sc.Metadata.Turntable.Triggers) != 2 {
		t.Fatal("expected 2 triggers parsed from action alias")
	}
	tr0 := sc.Metadata.Turntable.Triggers[0]
	if tr0.Type != "brake" || tr0.Time != 0.5 {
		t.Fatalf("tr0 mismatch: type=%s, time=%f", tr0.Type, tr0.Time)
	}
	tr1 := sc.Metadata.Turntable.Triggers[1]
	if tr1.Type != "beat_roll" || tr1.Time != 1.0 || tr1.Duration != 0.5 {
		t.Fatalf("tr1 mismatch: type=%s, time=%f, dur=%f", tr1.Type, tr1.Time, tr1.Duration)
	}
}

func TestTurntableRootTriggersUnmarshal(t *testing.T) {
	jsonScore := `{
  "metadata": {
    "title": "Root Triggers",
    "bpm": 120,
    "time_signature": "4/4",
    "turntable_triggers": [
      {"time": 0.5, "type": "tape_stop", "brake_time_ms": 150}
    ]
  },
  "tracks": [
    {
      "id": "t",
      "instrument": {"type": "synth"},
      "volume": 0.8,
      "notes": [{"time": 0, "duration": 1, "pitch": 60, "velocity": 0.8}]
    }
  ]
}`
	sc, err := ParseJSON([]byte(jsonScore))
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}
	if sc.Metadata.Turntable == nil || len(sc.Metadata.Turntable.Triggers) != 1 {
		t.Fatal("turntable_triggers not parsed into Metadata.Turntable.Triggers")
	}
	if !strings.EqualFold(sc.Metadata.Turntable.Triggers[0].Type, "tape_stop") {
		t.Fatalf("trigger type mismatch: %s", sc.Metadata.Turntable.Triggers[0].Type)
	}
}
