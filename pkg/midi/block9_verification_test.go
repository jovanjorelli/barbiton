package midi

import (
	"math"
	"testing"

	"barbiton/pkg/score"
)

func TestBlock9GM128Presets(t *testing.T) {
	for prog := 0; prog <= 127; prog++ {
		eng := GMEngineForProgram(prog, false)
		inst := mapGMInstrument(prog, false)
		if inst.Type != eng {
			t.Fatalf("program %d: expected type %s, got %s", prog, eng, inst.Type)
		}
		if eng == "synth" {
			params := synthVoicing(prog)
			if params == nil {
				t.Fatalf("program %d: nil synth parameters", prog)
			}
			switch {
			case prog >= 16 && prog <= 23:
				if params["synth_mode"] != 0 || params["unison_voices"] != 3 || params["lfo_pitch_depth"] != 0.15 {
					t.Fatalf("program %d (organ): unexpected voicing %+v", prog, params)
				}
			case prog >= 24 && prog <= 31:
				if params["synth_mode"] != 2 || params["pluck_decay"] != 0.99 {
					t.Fatalf("program %d (guitar): unexpected pluck voicing %+v", prog, params)
				}
			case prog >= 32 && prog <= 37:
				if params["synth_mode"] != 0 || params["filter_type"] != 2 || params["cutoff"] != 800 {
					t.Fatalf("program %d (bass): unexpected bass voicing %+v", prog, params)
				}
			case prog == 38 || prog == 39:
				if params["synth_mode"] != 1 || params["fm_depth"] != 3.5 || params["fm_ratio"] != 1.0 {
					t.Fatalf("program %d (synth bass): unexpected FM voicing %+v", prog, params)
				}
			case prog >= 40 && prog <= 55:
				if params["synth_mode"] != 0 || params["unison_voices"] != 5 || params["unison_detune"] != 0.2 {
					t.Fatalf("program %d (strings): unexpected supersaw voicing %+v", prog, params)
				}
			case prog >= 56 && prog <= 63:
				if params["filter_type"] != 1 || params["resonance"] != 0.45 || params["filter_env_depth"] != 3.5 {
					t.Fatalf("program %d (brass): unexpected brass voicing %+v", prog, params)
				}
			case prog >= 80 && prog <= 87:
				if params["pulse_width"] != 0.4 || params["portamento"] != 0.05 || params["unison_voices"] != 3 {
					t.Fatalf("program %d (lead): unexpected lead voicing %+v", prog, params)
				}
			case prog >= 88 && prog <= 95:
				if params["unison_voices"] != 7 || params["unison_detune"] != 0.25 || params["filter_type"] != 2 {
					t.Fatalf("program %d (pad): unexpected pad voicing %+v", prog, params)
				}
			case prog >= 96 && prog <= 103:
				if params["synth_mode"] != 1 || params["fm_ratio"] != 3.5 || params["fm_depth"] != 4.0 {
					t.Fatalf("program %d (fx): unexpected fx voicing %+v", prog, params)
				}
			case (prog >= 104 && prog <= 108) || prog == 110:
				if params["synth_mode"] != 2 || params["pluck_decay"] != 0.988 {
					t.Fatalf("program %d (ethnic): unexpected ethnic pluck voicing %+v", prog, params)
				}
			case prog >= 120 && prog <= 127:
				if params["synth_mode"] != 1 || params["fm_ratio"] != 7.13 || params["fm_depth"] != 6.0 {
					t.Fatalf("program %d (sfx): unexpected sfx voicing %+v", prog, params)
				}
			}
		}
	}
}

func TestBlock9CCMappingToScore(t *testing.T) {
	f := &File{
		Header: Header{Format: 1, Tracks: 1, TicksPerQuarter: 480},
		Tracks: [][]Event{{
			{Tick: 0, Type: EventMeta, Data1: MetaTempo, Data: be24(500000)},
			{Tick: 0, Type: EventProgramChange, Channel: 0, Data1: 80},
			{Tick: 0, Type: EventControlChange, Channel: 0, Data1: 1, Data2: 64},
			{Tick: 0, Type: EventControlChange, Channel: 0, Data1: 11, Data2: 96},
			{Tick: 0, Type: EventControlChange, Channel: 0, Data1: 74, Data2: 80},
			{Tick: 0, Type: EventControlChange, Channel: 0, Data1: 71, Data2: 50},
			{Tick: 0, Type: EventControlChange, Channel: 0, Data1: 64, Data2: 127},
			{Tick: 240, Type: EventControlChange, Channel: 0, Data1: 64, Data2: 0},
			{Tick: 0, Type: EventNoteOn, Channel: 0, Data1: 60, Data2: 100},
			{Tick: 240, Type: EventNoteOff, Channel: 0, Data1: 60, Data2: 64},
			{Tick: 480, Type: EventMeta, Data1: MetaEndOfTrack},
		}},
	}

	s, err := ToScore(f)
	if err != nil {
		t.Fatalf("ToScore: %v", err)
	}
	if len(s.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(s.Tracks))
	}
	tr := s.Tracks[0]
	if tr.Expression == nil || math.Abs(*tr.Expression-96.0/127.0) > 1e-4 {
		t.Fatalf("expression CC11 not mapped properly: %v", tr.Expression)
	}
	if tr.Vibrato == nil || tr.Vibrato.Depth <= 0 {
		t.Fatalf("vibrato CC1 not mapped properly: %+v", tr.Vibrato)
	}
	if tr.Instrument.Parameters["cutoff"] < 1000 {
		t.Fatalf("cutoff CC74 not mapped properly: %v", tr.Instrument.Parameters["cutoff"])
	}
	if math.Abs(float64(tr.Instrument.Parameters["resonance"])-50.0/127.0) > 1e-4 {
		t.Fatalf("resonance CC71 not mapped properly: %v", tr.Instrument.Parameters["resonance"])
	}
	if len(tr.Pedal) != 2 || !tr.Pedal[0].Down || tr.Pedal[1].Down {
		t.Fatalf("pedal CC64 moves not mapped properly: %+v", tr.Pedal)
	}
	if len(tr.CC) < 4 {
		t.Fatalf("expected at least 4 CC events in track, got %d", len(tr.CC))
	}
	if err := score.Validate(s); err != nil {
		t.Fatalf("score validation failed on CC mapped score: %v", err)
	}
}

func TestBlock9CCMappingFromScore(t *testing.T) {
	exprVal := 0.75
	s := &score.Score{
		Metadata: score.Metadata{Title: "CC Export Test", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{{
			ID:         "t1",
			Name:       "SynthLead",
			Instrument: score.InstrumentDef{Type: "synth", Parameters: map[string]float32{"cutoff": 5000, "resonance": 0.6}},
			Volume:     0.8,
			Expression: &exprVal,
			Vibrato:    &score.Vibrato{Rate: 5.0, Depth: 0.05},
			Pedal: []score.PedalEvent{
				{Time: 0.0, Down: true},
				{Time: 0.5, Down: false},
			},
			CC: []score.CCEvent{
				{Time: 0.25, Controller: 74, Value: 100},
			},
			Notes: []score.NoteEvent{
				{Time: 0.0, Duration: 0.5, Pitch: 60, Velocity: 0.8},
			},
		}},
	}

	f := FromScore(s, DefaultTicksPerQuarter)
	if len(f.Tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(f.Tracks))
	}
	trackEvents := f.Tracks[1]
	foundCC1 := false
	foundCC11 := false
	foundCC64Down := false
	foundCC64Up := false
	foundCC74 := false
	for _, e := range trackEvents {
		if e.Type == EventControlChange {
			switch e.Data1 {
			case 1:
				foundCC1 = true
			case 11:
				if e.Data2 != int(math.Round(0.75*127)) {
					t.Fatalf("expected CC11 value %d, got %d", int(math.Round(0.75*127)), e.Data2)
				}
				foundCC11 = true
			case 64:
				if e.Data2 == 127 {
					foundCC64Down = true
				} else if e.Data2 == 0 {
					foundCC64Up = true
				}
			case 74:
				if e.Data2 == 100 {
					foundCC74 = true
				}
			}
		}
	}
	if !foundCC1 {
		t.Fatal("CC1 (ModWheel) not emitted")
	}
	if !foundCC11 {
		t.Fatal("CC11 (Expression) not emitted")
	}
	if !foundCC64Down || !foundCC64Up {
		t.Fatal("CC64 (Pedal) moves not emitted")
	}
	if !foundCC74 {
		t.Fatal("CC74 automation not emitted")
	}
}

func TestBlock9RegressionPreservation(t *testing.T) {
	if v := familyVoicing(32, false)["brightness"]; v != 0.3 {
		t.Fatalf("familyVoicing bass brightness want 0.3, got %v", v)
	}
	if v := familyVoicing(80, false)["brightness"]; v != 0.55 {
		t.Fatalf("familyVoicing lead brightness want 0.55, got %v", v)
	}
	instStrings := mapGMInstrument(48, false)
	if instStrings.Parameters["brightness"] != 0.45 {
		t.Fatalf("mapGMInstrument strings brightness want 0.45, got %v", instStrings.Parameters["brightness"])
	}
	if instStrings.Parameters["unison_voices"] != 5 {
		t.Fatalf("mapGMInstrument strings unison_voices want 5, got %v", instStrings.Parameters["unison_voices"])
	}
	if instStrings.Parameters["unison_detune"] != 0.2 {
		t.Fatalf("mapGMInstrument strings unison_detune want 0.2, got %v", instStrings.Parameters["unison_detune"])
	}
	if instPiano := mapGMInstrument(0, false); instPiano.Type != "sampler" {
		t.Fatalf("piano type mismatch: %s", instPiano.Type)
	}
	if instOcarina := mapGMInstrument(79, false); instOcarina.Type != "ocarina" {
		t.Fatalf("ocarina type mismatch: %s", instOcarina.Type)
	}
}

func TestBlock9PluckBrightnessCC74(t *testing.T) {
	f := &File{
		Header: Header{Format: 1, Tracks: 1, TicksPerQuarter: 480},
		Tracks: [][]Event{{
			{Tick: 0, Type: EventMeta, Data1: MetaTempo, Data: be24(500000)},
			{Tick: 0, Type: EventProgramChange, Channel: 0, Data1: 24},
			{Tick: 0, Type: EventControlChange, Channel: 0, Data1: 74, Data2: 100},
			{Tick: 0, Type: EventNoteOn, Channel: 0, Data1: 64, Data2: 90},
			{Tick: 240, Type: EventNoteOff, Channel: 0, Data1: 64, Data2: 0},
			{Tick: 480, Type: EventMeta, Data1: MetaEndOfTrack},
		}},
	}
	s, err := ToScore(f)
	if err != nil {
		t.Fatalf("ToScore: %v", err)
	}
	if len(s.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(s.Tracks))
	}
	pb, ok := s.Tracks[0].Instrument.Parameters["pluck_brightness"]
	if !ok || pb < 10000 {
		t.Fatalf("expected pluck_brightness mapped from CC74, got %v", pb)
	}
}

func TestBlock9ContinuousCCFadeoutInitialExpression(t *testing.T) {
	f := &File{
		Header: Header{Format: 1, Tracks: 1, TicksPerQuarter: 480},
		Tracks: [][]Event{{
			{Tick: 0, Type: EventMeta, Data1: MetaTempo, Data: be24(500000)},
			{Tick: 0, Type: EventProgramChange, Channel: 0, Data1: 40},
			{Tick: 0, Type: EventControlChange, Channel: 0, Data1: 11, Data2: 120},
			{Tick: 0, Type: EventNoteOn, Channel: 0, Data1: 60, Data2: 90},
			{Tick: 240, Type: EventControlChange, Channel: 0, Data1: 11, Data2: 0},
			{Tick: 240, Type: EventNoteOff, Channel: 0, Data1: 60, Data2: 0},
			{Tick: 480, Type: EventMeta, Data1: MetaEndOfTrack},
		}},
	}
	s, err := ToScore(f)
	if err != nil {
		t.Fatalf("ToScore: %v", err)
	}
	tr := s.Tracks[0]
	if tr.Expression == nil || *tr.Expression < 0.9 {
		t.Fatalf("expected initial expression near 120/127 (~0.94), got %v", tr.Expression)
	}
	if len(tr.CC) != 2 {
		t.Fatalf("expected 2 CC events in track, got %d", len(tr.CC))
	}
}

func TestBlock9FromScoreInitialCCWithLaterAutomation(t *testing.T) {
	exprVal := 0.6
	s := &score.Score{
		Metadata: score.Metadata{Title: "Initial CC Test", BPM: 120, TimeSignature: "4/4"},
		Tracks: []score.Track{{
			ID:         "t1",
			Instrument: score.InstrumentDef{Type: "synth"},
			Volume:     0.8,
			Expression: &exprVal,
			Vibrato:    &score.Vibrato{Rate: 5.0, Depth: 0.05},
			CC: []score.CCEvent{
				{Time: 1.0, Controller: 11, Value: 30},
				{Time: 1.0, Controller: 1, Value: 100},
			},
			Notes: []score.NoteEvent{
				{Time: 0.0, Duration: 2.0, Pitch: 60, Velocity: 0.8},
			},
		}},
	}
	f := FromScore(s, DefaultTicksPerQuarter)
	if len(f.Tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(f.Tracks))
	}
	foundTick0CC11 := false
	foundTick0CC1 := false
	foundTick1CC11 := false
	foundTick1CC1 := false
	for _, e := range f.Tracks[1] {
		if e.Type == EventControlChange {
			if e.Tick == 0 && e.Data1 == 11 && e.Data2 == int(math.Round(0.6*127)) {
				foundTick0CC11 = true
			}
			if e.Tick == 0 && e.Data1 == 1 {
				foundTick0CC1 = true
			}
			if e.Tick > 0 && e.Data1 == 11 && e.Data2 == 30 {
				foundTick1CC11 = true
			}
			if e.Tick > 0 && e.Data1 == 1 && e.Data2 == 100 {
				foundTick1CC1 = true
			}
		}
	}
	if !foundTick0CC11 {
		t.Fatal("initial CC11 at tick 0 missing when later CC11 exists")
	}
	if !foundTick0CC1 {
		t.Fatal("initial CC1 at tick 0 missing when later CC1 exists")
	}
	if !foundTick1CC11 {
		t.Fatal("later CC11 automation missing")
	}
	if !foundTick1CC1 {
		t.Fatal("later CC1 automation missing")
	}
}
