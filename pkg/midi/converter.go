package midi

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"barbiton/pkg/score"
)

const DefaultTicksPerQuarter = 480

const (
	programOcarina = 79
	programPiano   = 0
)

type timeSeg struct {
	tick uint32
	sec  float64

	secPerTick float64
}

func buildTimeMap(tempos []Tempo, ppq int) []timeSeg {
	segs := make([]timeSeg, 0, len(tempos)+1)
	prev := timeSeg{tick: 0, sec: 0, secPerTick: 500000 / 1e6 / float64(ppq)}
	for _, t := range tempos {
		if t.Tick > prev.tick {
			segs = append(segs, timeSeg{
				tick:       prev.tick,
				sec:        prev.sec,
				secPerTick: prev.secPerTick,
			})
			prev.sec += float64(t.Tick-prev.tick) * prev.secPerTick
		}
		prev.tick = t.Tick
		prev.secPerTick = float64(t.MicrosPerQuarter) / 1e6 / float64(ppq)
	}
	segs = append(segs, prev)
	return segs
}

func tickTime(segs []timeSeg, tick uint32) float64 {
	seg := segs[0]
	for _, s := range segs {
		if s.tick > tick {
			break
		}
		seg = s
	}
	return seg.sec + float64(tick-seg.tick)*seg.secPerTick
}

func ToScore(f *File) (*score.Score, error) {
	if f == nil || len(f.Tracks) == 0 || f.Header.TicksPerQuarter <= 0 {
		return nil, errors.New("midi: empty or untimed file")
	}
	ppq := f.Header.TicksPerQuarter

	tempos := f.Tempos()
	if len(tempos) == 0 {
		tempos = []Tempo{{Tick: 0, MicrosPerQuarter: 500000}}
	}
	segs := buildTimeMap(tempos, ppq)

	bpm := BPMFromMicros(tempos[0].MicrosPerQuarter)
	if bpm > 1000 {
		bpm = 1000
	}

	ts := "4/4"
	if tss := f.TimeSignatures(); len(tss) > 0 && tss[0].Numerator > 0 {
		ts = fmt.Sprintf("%d/%d", tss[0].Numerator, tss[0].Denominator)
	}

	title := ""
	for _, tr := range f.Tracks {
		if n := strings.TrimSpace(TrackName(tr)); n != "" {
			title = n
			break
		}
	}
	if title == "" {
		title = "Imported MIDI"
	}

	out := &score.Score{
		Metadata: score.Metadata{Title: title, BPM: bpm, TimeSignature: ts},
	}

	for ti, tr := range f.Tracks {
		endTick := MaxTick(tr)
		notes := CollectNotes(tr, endTick)
		if len(notes) == 0 {
			continue
		}
		trackName := strings.TrimSpace(TrackName(tr))

		order := []int{}
		byCh := map[int][]Note{}
		for _, n := range notes {
			if _, ok := byCh[n.Channel]; !ok {
				order = append(order, n.Channel)
			}
			byCh[n.Channel] = append(byCh[n.Channel], n)
		}
		program, cc7, cc10, cc1, cc11, cc71, cc74 := channelState(tr)

		for _, ch := range order {
			percussion := ch == 9
			prog := program[ch]
			name := trackName
			if name == "" {
				if percussion {
					name = "Drums"
				} else {
					name = GMProgramNames[prog]
				}
			}
			if len(order) > 1 {
				name = fmt.Sprintf("%s (ch %d)", name, ch+1)
			}

			st := score.Track{
				ID:         fmt.Sprintf("t%d_c%d", ti, ch),
				Name:       name,
				Instrument: mapGMInstrument(prog, percussion),
				Pan:        0,
				Volume:     0.8,
				ReverbSend: 0.2,
			}
			if v, ok := cc10[ch]; ok {
				st.Pan = float32(clampF64(float64(v)/63.5-1, -1, 1))
			}
			if v, ok := cc7[ch]; ok {
				st.Volume = float32(clampF64(float64(v)/127*0.9, 0, 1))
			}
			if v, ok := cc11[ch]; ok {
				expr := clampF64(float64(v)/127.0, 0, 1)
				st.Expression = &expr
			}
			if v, ok := cc1[ch]; ok && v > 0 {
				st.Vibrato = &score.Vibrato{
					Rate:  5.0,
					Depth: clampF64(float64(v)/127.0*0.1, 0, 1),
				}
				if st.Instrument.Type == "synth" {
					if st.Instrument.Parameters == nil {
						st.Instrument.Parameters = make(map[string]float32)
					}
					st.Instrument.Parameters["lfo_pitch_depth"] = float32(clampF64(float64(v)/127.0*2.0, 0, 24))
				}
			}
			if v, ok := cc74[ch]; ok {
				if st.Instrument.Parameters == nil {
					st.Instrument.Parameters = make(map[string]float32)
				}
				st.Instrument.Parameters["cutoff"] = float32(clampF64(20.0+(float64(v)/127.0)*(20000.0-20.0), 20, 20000))
				st.Instrument.Parameters["brightness"] = float32(clampF64(float64(v)/127.0, 0, 1))
				st.Instrument.Parameters["pluck_brightness"] = float32(clampF64(200.0+(float64(v)/127.0)*(20000.0-200.0), 200, 20000))
			}
			if v, ok := cc71[ch]; ok {
				if st.Instrument.Parameters == nil {
					st.Instrument.Parameters = make(map[string]float32)
				}
				st.Instrument.Parameters["resonance"] = float32(clampF64(float64(v)/127.0, 0, 1))
			}
			for _, n := range byCh[ch] {
				start := tickTime(segs, n.StartTick)
				end := tickTime(segs, n.EndTick)

				st.Notes = append(st.Notes, score.NoteEvent{
					Time:     start,
					Duration: end - start,
					Pitch:    clampInt(n.Pitch, 0, 127),
					Velocity: float32(clampInt(n.Velocity, 1, 127)) / 127,
				})
			}
			sort.SliceStable(st.Notes, func(i, j int) bool { return st.Notes[i].Time < st.Notes[j].Time })
			st.Pedal = pedalMoves(tr, ch, segs)
			st.CC = ccEvents(tr, ch, segs)
			out.Tracks = append(out.Tracks, st)
		}
	}
	if len(out.Tracks) == 0 {
		return nil, errors.New("midi: file contains no notes")
	}
	return out, nil
}

func channelState(tr []Event) (program, cc7, cc10, cc1, cc11, cc71, cc74 map[int]int) {
	program = map[int]int{}
	cc7 = map[int]int{}
	cc10 = map[int]int{}
	cc1 = map[int]int{}
	cc11 = map[int]int{}
	cc71 = map[int]int{}
	cc74 = map[int]int{}
	for _, e := range tr {
		switch {
		case e.Type == EventProgramChange:
			if _, seen := program[e.Channel]; !seen {
				program[e.Channel] = e.Data1
			}
		case e.Type == EventControlChange:
			switch e.Data1 {
			case 1:
				if _, seen := cc1[e.Channel]; !seen {
					cc1[e.Channel] = e.Data2
				}
			case 7:
				cc7[e.Channel] = e.Data2
			case 10:
				cc10[e.Channel] = e.Data2
			case 11:
				if _, seen := cc11[e.Channel]; !seen {
					cc11[e.Channel] = e.Data2
				}
			case 71:
				if _, seen := cc71[e.Channel]; !seen {
					cc71[e.Channel] = e.Data2
				}
			case 74:
				if _, seen := cc74[e.Channel]; !seen {
					cc74[e.Channel] = e.Data2
				}
			}
		}
	}
	return
}

func ccEvents(tr []Event, ch int, segs []timeSeg) []score.CCEvent {
	var out []score.CCEvent
	for _, e := range tr {
		if e.Type != EventControlChange || e.Channel != ch {
			continue
		}
		if e.Data1 == 1 || e.Data1 == 11 || e.Data1 == 71 || e.Data1 == 74 {
			out = append(out, score.CCEvent{
				Time:       tickTime(segs, e.Tick),
				Controller: e.Data1,
				Value:      e.Data2,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func pedalMoves(tr []Event, ch int, segs []timeSeg) []score.PedalEvent {
	var out []score.PedalEvent
	for _, e := range tr {
		if e.Type != EventControlChange || e.Data1 != 64 || e.Channel != ch {
			continue
		}
		out = append(out, score.PedalEvent{
			Time: tickTime(segs, e.Tick),
			Down: e.Data2 >= 64,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func familyVoicing(program int, percussion bool) map[string]float32 {
	if percussion || program/8 == 14 {
		return map[string]float32{"brightness": 0.9, "breath_noise": 0.5,
			"vibrato_rate": 1, "vibrato_depth": 0}
	}
	switch program / 8 {
	case 5, 6:
		return map[string]float32{"brightness": 0.45, "breath_noise": 0.35,
			"vibrato_rate": 4.5, "vibrato_depth": 0.015}
	case 7:
		return map[string]float32{"brightness": 0.8, "breath_noise": 0.3,
			"vibrato_rate": 5, "vibrato_depth": 0.02}
	case 8, 9:
		return map[string]float32{"brightness": 0.7, "breath_noise": 0.35,
			"vibrato_rate": 5.5, "vibrato_depth": 0.02}
	case 4:
		return map[string]float32{"brightness": 0.3, "breath_noise": 0.3,
			"vibrato_rate": 3.5, "vibrato_depth": 0.008}
	case 0, 1, 2, 3:
		return map[string]float32{"brightness": 0.6, "breath_noise": 0.2,
			"vibrato_rate": 2, "vibrato_depth": 0.005}
	}
	return map[string]float32{"brightness": 0.55, "breath_noise": 0.25,
		"vibrato_rate": 4, "vibrato_depth": 0.01}
}

var gmEngineMapping = [128]string{
	"sampler", "sampler", "sampler", "sampler", "sampler", "sampler", "sampler", "sampler",
	"sampler", "sampler", "sampler", "sampler", "sampler", "sampler", "sampler", "sampler",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"ocarina", "ocarina", "ocarina", "ocarina", "ocarina", "ocarina", "ocarina", "ocarina",
	"ocarina", "ocarina", "ocarina", "ocarina", "ocarina", "ocarina", "ocarina", "ocarina",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
	"synth", "synth", "synth", "synth", "synth", "ocarina", "synth", "ocarina",
	"sampler", "sampler", "sampler", "sampler", "sampler", "sampler", "sampler", "sampler",
	"synth", "synth", "synth", "synth", "synth", "synth", "synth", "synth",
}

func GMEngineForProgram(prog int, percussion bool) string {
	if percussion {
		return "sampler"
	}
	prog = clampInt(prog, 0, 127)
	return gmEngineMapping[prog]
}

func synthVoicing(prog int) map[string]float32 {
	prog = clampInt(prog, 0, 127)
	switch {
	case prog >= 16 && prog <= 23:
		return map[string]float32{
			"synth_mode":      0,
			"waveform1":       2,
			"waveform2":       1,
			"cutoff":          8000,
			"resonance":       0.1,
			"attack":          0.005,
			"decay":           0.1,
			"sustain":         0.95,
			"release":         0.08,
			"unison_voices":   3,
			"unison_detune":   0.08,
			"unison_spread":   0.5,
			"lfo_waveform":    0,
			"lfo_rate":        5.5,
			"lfo_pitch_depth": 0.15,
		}
	case prog >= 24 && prog <= 31:
		damp := float32(0.35)
		if prog == 28 {
			damp = 0.7
		} else if prog == 25 {
			damp = 0.2
		}
		return map[string]float32{
			"synth_mode":          2,
			"pluck_damping":       damp,
			"pluck_decay":         0.99,
			"pluck_brightness":    12000,
			"pluck_pick_position": 0.25,
			"release":             0.3,
		}
	case prog >= 32 && prog <= 39:
		if prog == 38 || prog == 39 {
			return map[string]float32{
				"synth_mode":  1,
				"fm_ratio":    1.0,
				"fm_depth":    3.5,
				"fm_feedback": 0.2,
				"fm_attack":   0.001,
				"fm_decay":    0.15,
				"fm_sustain":  0.1,
				"fm_release":  0.1,
				"attack":      0.002,
				"decay":       0.25,
				"sustain":     0.6,
				"release":     0.15,
			}
		}
		return map[string]float32{
			"synth_mode":       0,
			"waveform1":        0,
			"waveform2":        1,
			"filter_type":      2,
			"cutoff":           800,
			"resonance":        0.4,
			"filter_env_depth": 3.0,
			"filter_attack":    0.005,
			"filter_decay":     0.2,
			"filter_sustain":   0.2,
			"filter_release":   0.2,
			"attack":           0.002,
			"decay":            0.2,
			"sustain":          0.7,
			"release":          0.15,
		}
	case prog >= 40 && prog <= 55:
		return map[string]float32{
			"synth_mode":    0,
			"waveform1":     0,
			"waveform2":     0,
			"unison_voices": 5,
			"unison_detune": 0.2,
			"unison_spread": 0.7,
			"pan_spread":    0.5,
			"filter_type":   2,
			"cutoff":        5000,
			"resonance":     0.15,
			"attack":        0.08,
			"decay":         0.3,
			"sustain":       0.85,
			"release":       0.4,
		}
	case prog >= 56 && prog <= 63:
		return map[string]float32{
			"synth_mode":       0,
			"waveform1":        0,
			"waveform2":        0,
			"detune":           0.05,
			"filter_type":      1,
			"cutoff":           2500,
			"resonance":        0.45,
			"filter_env_depth": 3.5,
			"filter_attack":    0.04,
			"filter_decay":     0.2,
			"filter_sustain":   0.4,
			"filter_release":   0.2,
			"attack":           0.03,
			"decay":            0.15,
			"sustain":          0.8,
			"release":          0.2,
		}
	case prog >= 80 && prog <= 87:
		return map[string]float32{
			"synth_mode":    0,
			"waveform1":     1,
			"waveform2":     0,
			"pulse_width":   0.4,
			"pwm":           0.35,
			"portamento":    0.05,
			"detune":        0.08,
			"unison_voices": 3,
			"unison_detune": 0.1,
			"unison_spread": 0.4,
			"filter_type":   1,
			"cutoff":        6000,
			"resonance":     0.3,
			"attack":        0.01,
			"decay":         0.1,
			"sustain":       0.8,
			"release":       0.15,
		}
	case prog >= 88 && prog <= 95:
		return map[string]float32{
			"synth_mode":    0,
			"waveform1":     0,
			"waveform2":     0,
			"unison_voices": 7,
			"unison_detune": 0.25,
			"unison_spread": 0.8,
			"pan_spread":    0.6,
			"filter_type":   2,
			"cutoff":        3500,
			"resonance":     0.2,
			"attack":        0.3,
			"decay":         0.5,
			"sustain":       0.9,
			"release":       0.8,
		}
	case prog >= 96 && prog <= 103:
		return map[string]float32{
			"synth_mode":      1,
			"fm_ratio":        3.5,
			"fm_depth":        4.0,
			"fm_feedback":     0.3,
			"fm_attack":       0.05,
			"fm_decay":        0.3,
			"fm_sustain":      0.5,
			"fm_release":      0.6,
			"lfo_rate":        2.0,
			"lfo_pitch_depth": 0.5,
			"attack":          0.1,
			"decay":           0.4,
			"sustain":         0.7,
			"release":         0.8,
		}
	case (prog >= 104 && prog <= 108) || prog == 110:
		pos := float32(0.25)
		damp := float32(0.3)
		bright := float32(11000)
		if prog == 104 {
			pos = 0.4
			damp = 0.2
		} else if prog == 105 {
			pos = 0.15
			damp = 0.6
		} else if prog == 108 {
			pos = 0.5
			damp = 0.1
			bright = 8000
		}
		return map[string]float32{
			"synth_mode":          2,
			"pluck_damping":       damp,
			"pluck_decay":         0.988,
			"pluck_brightness":    bright,
			"pluck_pick_position": pos,
			"release":             0.4,
		}
	case prog >= 120 && prog <= 127:
		return map[string]float32{
			"synth_mode":       1,
			"fm_ratio":         7.13,
			"fm_depth":         6.0,
			"fm_feedback":      0.8,
			"lfo_waveform":     4,
			"lfo_rate":         15.0,
			"lfo_filter_depth": 5.0,
			"lfo_pitch_depth":  3.0,
			"filter_type":      2,
			"cutoff":           4000,
			"resonance":        0.6,
			"attack":           0.01,
			"decay":            0.3,
			"sustain":          0.3,
			"release":          0.5,
		}
	default:
		return map[string]float32{
			"synth_mode": 0,
			"waveform1":  0,
			"waveform2":  1,
			"cutoff":     4000,
			"resonance":  0.2,
			"attack":     0.01,
			"decay":      0.1,
			"sustain":    0.8,
			"release":    0.2,
		}
	}
}

func mapGMInstrument(prog int, percussion bool) score.InstrumentDef {
	params := familyVoicing(prog, percussion)
	engineType := GMEngineForProgram(prog, percussion)
	if engineType == "sampler" {
		return score.InstrumentDef{
			Type:       "sampler",
			File:       "test_data/samples/piano.sfz",
			Parameters: params,
		}
	}
	if engineType == "synth" {
		sp := synthVoicing(prog)
		merged := make(map[string]float32, len(params)+len(sp))
		for k, v := range params {
			merged[k] = v
		}
		for k, v := range sp {
			merged[k] = v
		}
		return score.InstrumentDef{
			Type:       "synth",
			Parameters: merged,
		}
	}
	return score.InstrumentDef{
		Type:       engineType,
		Parameters: params,
	}
}

func FromScore(s *score.Score, ppq int) *File {
	if ppq <= 0 {
		ppq = DefaultTicksPerQuarter
	}
	bpm := s.Metadata.BPM
	if !(bpm > 0) {
		bpm = 120
	}
	secPerTick := 60 / (bpm * float64(ppq))
	toTick := func(sec float64) uint32 {
		t := roundF(sec / secPerTick)
		if t < 0 {
			t = 0
		}
		if t > float64(math.MaxUint32-1) {
			t = float64(math.MaxUint32 - 1)
		}
		return uint32(t)
	}

	f := &File{Header: Header{Format: 1, TicksPerQuarter: ppq}}

	endTick := uint32(0)
	for i := range s.Tracks {
		for _, n := range s.Tracks[i].Notes {
			if t := toTick(n.Time + n.Duration); t > endTick {
				endTick = t
			}
		}
	}
	conductor := []Event{
		{Tick: 0, Type: EventMeta, Data1: MetaTempo, Data: be24(MicrosFromBPM(bpm))},
	}
	conductor = append(conductor, Event{Tick: 0, Type: EventMeta, Data1: MetaTimeSignature,
		Data: timeSigPayload(s.Metadata.TimeSignature)})
	if title := strings.TrimSpace(s.Metadata.Title); title != "" {
		conductor = append(conductor, Event{Tick: 0, Type: EventMeta, Data1: MetaTrackName,
			Data: []byte(title)})
	}
	conductor = append(conductor, Event{Tick: endTick, Type: EventMeta, Data1: MetaEndOfTrack})
	f.Tracks = append(f.Tracks, conductor)

	for i := range s.Tracks {
		tr := &s.Tracks[i]
		ch := i % 15
		if ch >= 9 {
			ch++
		}
		events := []Event{}
		name := tr.Name
		if name == "" {
			name = tr.ID
		}
		if name != "" {
			events = append(events, Event{Tick: 0, Type: EventMeta, Data1: MetaTrackName, Data: []byte(name)})
		}
		prog := programOcarina
		switch strings.ToLower(tr.Instrument.Type) {
		case "sampler":
			prog = programPiano
		case "synth":
			prog = 80
		}
		events = append(events,
			Event{Tick: 0, Type: EventProgramChange, Channel: ch, Data1: prog},
			Event{Tick: 0, Type: EventControlChange, Channel: ch, Data1: 7,
				Data2: clampInt(int(roundF(float64(tr.Volume)/0.9*127)), 0, 127)},
			Event{Tick: 0, Type: EventControlChange, Channel: ch, Data1: 10,
				Data2: clampInt(int(roundF((float64(tr.Pan)+1)/2*127)), 0, 127)},
		)
		for _, cc := range tr.CC {
			events = append(events, Event{
				Tick:    toTick(cc.Time),
				Type:    EventControlChange,
				Channel: ch,
				Data1:   cc.Controller,
				Data2:   clampInt(cc.Value, 0, 127),
			})
		}
		for _, pd := range tr.Pedal {
			val := 0
			if pd.Down {
				val = 127
			}
			events = append(events, Event{
				Tick:    toTick(pd.Time),
				Type:    EventControlChange,
				Channel: ch,
				Data1:   64,
				Data2:   val,
			})
		}
		if tr.Expression != nil && !hasCCAtTick0(tr.CC, 11, toTick) {
			events = append(events, Event{
				Tick:    0,
				Type:    EventControlChange,
				Channel: ch,
				Data1:   11,
				Data2:   clampInt(int(roundF(*tr.Expression*127)), 0, 127),
			})
		}
		if tr.Vibrato != nil && tr.Vibrato.Depth > 0 && !hasCCAtTick0(tr.CC, 1, toTick) {
			events = append(events, Event{
				Tick:    0,
				Type:    EventControlChange,
				Channel: ch,
				Data1:   1,
				Data2:   clampInt(int(roundF(tr.Vibrato.Depth/0.1*127)), 0, 127),
			})
		}
		trackEnd := uint32(0)
		for _, n := range tr.Notes {
			on := toTick(n.Time)
			off := toTick(n.Time + n.Duration)
			if off <= on {
				off = on + 1
			}
			vel := clampInt(int(roundF(float64(n.Velocity)*127)), 1, 127)
			events = append(events,
				Event{Tick: on, Type: EventNoteOn, Channel: ch, Data1: n.Pitch, Data2: vel},
				Event{Tick: off, Type: EventNoteOff, Channel: ch, Data1: n.Pitch, Data2: 64},
			)
			if off > trackEnd {
				trackEnd = off
			}
		}
		for _, e := range events {
			if e.Tick > trackEnd {
				trackEnd = e.Tick
			}
		}
		sort.SliceStable(events, func(a, b int) bool {
			if events[a].Tick != events[b].Tick {
				return events[a].Tick < events[b].Tick
			}
			return eventRank(events[a]) < eventRank(events[b])
		})
		events = append(events, Event{Tick: trackEnd, Type: EventMeta, Data1: MetaEndOfTrack})
		f.Tracks = append(f.Tracks, events)
	}
	f.Header.Tracks = len(f.Tracks)
	return f
}

func hasCCAtTick0(ccs []score.CCEvent, num int, toTick func(float64) uint32) bool {
	for _, c := range ccs {
		if c.Controller == num && toTick(c.Time) == 0 {
			return true
		}
	}
	return false
}

func be24(v int) []byte {
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

func timeSigPayload(ts string) []byte {
	num, den := 4, 4
	if a, b, ok := strings.Cut(ts, "/"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(a)); err == nil && n > 0 {
			num = n
		}
		if d, err := strconv.Atoi(strings.TrimSpace(b)); err == nil && d > 0 {
			den = d
		}
	}
	pow := 2
	for p := 0; p <= 5; p++ {
		if 1<<p == den {
			pow = p
			break
		}
	}
	return []byte{byte(num), byte(pow), 24, 8}
}

func clampF64(v, lo, hi float64) float64 {
	switch {
	case v < lo || v != v:
		return lo
	case v > hi:
		return hi
	}
	return v
}
