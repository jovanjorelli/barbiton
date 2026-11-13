package score

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Score struct {
	Metadata Metadata `json:"metadata"`
	Tracks   []Track  `json:"tracks"`
}

type Metadata struct {
	Title                 string          `json:"title"`
	BPM                   float64         `json:"bpm"`
	TimeSignature         string          `json:"time_signature"`
	Transpose             int             `json:"transpose,omitempty"`
	Swing                 float64         `json:"swing,omitempty"`
	BreathMs              float64         `json:"breath,omitempty"`
	HumanizeTimingMs      float64         `json:"humanize_timing_ms,omitempty"`
	HumanizeVelocity      float64         `json:"humanize_velocity,omitempty"`
	ConvolutionIR         string          `json:"convolution_ir,omitempty"`
	ConvolutionDecayRatio float32         `json:"convolution_decay,omitempty"`
	ConvolutionPreDelayMs float32         `json:"convolution_predelay_ms,omitempty"`
	ConvolutionDryWet     float32         `json:"convolution_dry_wet,omitempty"`
	ConvolutionLowCutHz   float32         `json:"convolution_low_cut,omitempty"`
	ConvolutionHighCutHz  float32         `json:"convolution_high_cut,omitempty"`
	MultibandEnabled      bool            `json:"multiband,omitempty"`
	OversamplingEnabled   bool            `json:"oversampling,omitempty"`
	OTT                   *TrackOTT       `json:"ott,omitempty"`
	Turntable             *TrackTurntable `json:"turntable,omitempty"`
}

func (m *Metadata) UnmarshalJSON(data []byte) error {
	type rawMeta Metadata
	var r struct {
		rawMeta
		TurntableTriggers []TurntableTrigger `json:"turntable_triggers,omitempty"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*m = Metadata(r.rawMeta)
	if len(r.TurntableTriggers) > 0 {
		if m.Turntable == nil {
			m.Turntable = &TrackTurntable{Enabled: true, InertiaMs: 30.0}
		}
		m.Turntable.Triggers = append(m.Turntable.Triggers, r.TurntableTriggers...)
	}
	return nil
}

type Track struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Instrument         InstrumentDef      `json:"instrument"`
	Pan                float32            `json:"pan"`
	Volume             float32            `json:"volume"`
	Transpose          int                `json:"transpose,omitempty"`
	Swing              float64            `json:"swing,omitempty"`
	Vibrato            *Vibrato           `json:"vibrato,omitempty"`
	Expression         *float64           `json:"expression,omitempty"`
	BreathMs           float64            `json:"breath,omitempty"`
	Room               *Room              `json:"room,omitempty"`
	ReverbSend         float32            `json:"reverb_send,omitempty"`
	EQ                 TrackEQ            `json:"eq,omitempty"`
	Delay              *TrackDelay        `json:"delay,omitempty"`
	Pedal              []PedalEvent       `json:"pedal,omitempty"`
	CC                 []CCEvent          `json:"cc,omitempty"`
	SidechainSource    string             `json:"sidechain_source,omitempty"`
	SidechainDuckingDB float32            `json:"sidechain_ducking_db,omitempty"`
	SidechainAttackMs  float32            `json:"sidechain_attack_ms,omitempty"`
	SidechainReleaseMs float32            `json:"sidechain_release_ms,omitempty"`
	SidechainHoldMs    float32            `json:"sidechain_hold_ms,omitempty"`
	SidechainRatio     float32            `json:"sidechain_ratio,omitempty"`
	SidechainMakeupDB  float32            `json:"sidechain_makeup_db,omitempty"`
	HumanizeTimingMs   float64            `json:"humanize_timing_ms,omitempty"`
	HumanizeVelocity   float64            `json:"humanize_velocity,omitempty"`
	Notes              []NoteEvent        `json:"notes"`
	Automation         AutomationMap      `json:"automation,omitempty"`
	Distortion         *TrackDistortion   `json:"distortion,omitempty"`
	OTT                *TrackOTT          `json:"ott,omitempty"`
	Turntable          *TrackTurntable    `json:"turntable,omitempty"`
	TurntableTriggers  []TurntableTrigger `json:"turntable_triggers,omitempty"`
}

type TrackDistortion struct {
	Mode         string  `json:"mode,omitempty"`
	Drive        float32 `json:"drive,omitempty"`
	Tone         float32 `json:"tone,omitempty"`
	Mix          float32 `json:"mix,omitempty"`
	OutputGainDB float32 `json:"output_gain_db,omitempty"`
}

func (td *TrackDistortion) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed == "true" {
		*td = TrackDistortion{Mode: "tube", Drive: 1.0, Mix: 1.0, Tone: 1.0}
		return nil
	}
	if trimmed == "false" {
		*td = TrackDistortion{Mix: 0}
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var mode string
		if err := json.Unmarshal(data, &mode); err != nil {
			return err
		}
		*td = TrackDistortion{Mode: mode, Drive: 1.0, Mix: 1.0, Tone: 1.0}
		return nil
	}
	type rawDist TrackDistortion
	var d rawDist
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	*td = TrackDistortion(d)
	if td.Mode == "" {
		td.Mode = "tube"
	}
	if td.Mix == 0 && td.Drive == 0 {
		td.Mix = 1.0
		td.Drive = 1.0
	}
	if td.Tone == 0 {
		td.Tone = 10000.0
	}
	return nil
}

type TrackOTT struct {
	Depth     float32    `json:"depth,omitempty"`
	TimeScale float32    `json:"time_scale,omitempty"`
	InGainDB  float32    `json:"in_gain_db,omitempty"`
	OutGainDB float32    `json:"out_gain_db,omitempty"`
	BandGains [3]float32 `json:"band_gains,omitempty"`
}

func (to *TrackOTT) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed == "true" {
		*to = TrackOTT{Depth: 1.0, TimeScale: 1.0}
		return nil
	}
	if trimmed == "false" {
		*to = TrackOTT{Depth: 0}
		return nil
	}
	type rawOTT TrackOTT
	var r rawOTT
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*to = TrackOTT(r)
	if to.TimeScale <= 0 {
		to.TimeScale = 1.0
	}
	return nil
}

type TrackTurntable struct {
	Enabled   bool               `json:"enabled,omitempty"`
	InertiaMs float64            `json:"inertia_ms,omitempty"`
	Triggers  []TurntableTrigger `json:"triggers,omitempty"`
}

func (tt *TrackTurntable) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed == "true" {
		*tt = TrackTurntable{Enabled: true, InertiaMs: 30.0}
		return nil
	}
	if trimmed == "false" {
		*tt = TrackTurntable{Enabled: false}
		return nil
	}
	type rawTT TrackTurntable
	var r rawTT
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*tt = TrackTurntable(r)
	if tt.InertiaMs <= 0 {
		tt.InertiaMs = 30.0
	}
	tt.Enabled = true
	return nil
}

type TurntableTrigger struct {
	Time        float64 `json:"time"`
	Type        string  `json:"type"`
	Duration    float64 `json:"duration,omitempty"`
	BrakeTimeMs float64 `json:"brake_time_ms,omitempty"`
	Curve       string  `json:"curve,omitempty"`
	Speed       float32 `json:"speed,omitempty"`
	Subdivision string  `json:"subdivision,omitempty"`
	DecayMs     float64 `json:"decay_ms,omitempty"`
}

func (tr *TurntableTrigger) UnmarshalJSON(data []byte) error {
	type rawTrigger TurntableTrigger
	var r struct {
		rawTrigger
		Action     string  `json:"action,omitempty"`
		At         float64 `json:"at,omitempty"`
		DurationMs float64 `json:"duration_ms,omitempty"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*tr = TurntableTrigger(r.rawTrigger)
	if tr.Type == "" && r.Action != "" {
		tr.Type = r.Action
	}
	if tr.Time == 0 && r.At != 0 {
		tr.Time = r.At
	}
	if tr.Duration == 0 && r.DurationMs > 0 {
		tr.Duration = r.DurationMs / 1000.0
	}
	return nil
}

type CCEvent struct {
	Time       float64 `json:"time"`
	Controller int     `json:"controller"`
	Value      int     `json:"value"`
}

func (c *CCEvent) UnmarshalJSON(data []byte) error {
	type rawCC CCEvent
	var r struct {
		rawCC
		CC *int `json:"cc,omitempty"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*c = CCEvent(r.rawCC)
	if r.CC != nil && c.Controller == 0 {
		c.Controller = *r.CC
	}
	return nil
}

type PedalEvent struct {
	Time float64 `json:"time"`
	Down bool    `json:"down"`
}

type TrackDelay struct {
	Mode        string  `json:"mode,omitempty"`
	Subdivision string  `json:"subdivision,omitempty"`
	Seconds     float64 `json:"seconds,omitempty"`
	Feedback    float32 `json:"feedback,omitempty"`
	DampingHz   float32 `json:"damping_hz,omitempty"`
	Wet         float32 `json:"wet,omitempty"`
}

type EQBand struct {
	Freq float32 `json:"freq"`
	Gain float32 `json:"gain"`
	Q    float32 `json:"q"`
}

type TrackEQ struct {
	HPF       float32 `json:"hpf,omitempty"`
	LowShelf  EQBand  `json:"eq_low,omitempty"`
	MidPeak   EQBand  `json:"eq_mid,omitempty"`
	HighShelf EQBand  `json:"eq_high,omitempty"`
}

type InstrumentDef struct {
	Type       string             `json:"type"`
	File       string             `json:"file,omitempty"`
	Parameters map[string]float32 `json:"parameters,omitempty"`
}

func (i *InstrumentDef) UnmarshalJSON(data []byte) error {
	type rawDef struct {
		Type       string                 `json:"type"`
		File       string                 `json:"file,omitempty"`
		Parameters map[string]interface{} `json:"parameters,omitempty"`
	}
	var r rawDef
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	i.Type = r.Type
	i.File = r.File
	if r.Parameters != nil {
		i.Parameters = make(map[string]float32, len(r.Parameters))
		for k, v := range r.Parameters {
			switch val := v.(type) {
			case float64:
				i.Parameters[k] = float32(val)
			case bool:
				if val {
					i.Parameters[k] = 1.0
				} else {
					i.Parameters[k] = 0.0
				}
			case string:
				s := strings.ToLower(strings.TrimSpace(val))
				switch s {
				case "saw_to_square":
					i.Parameters[k] = 0.0
				case "metallic_growl":
					i.Parameters[k] = 1.0
				case "vocal_formant_talk":
					i.Parameters[k] = 2.0
				case "aggressive_modern":
					i.Parameters[k] = 3.0
				case "diode":
					i.Parameters[k] = 0.0
				case "tube":
					i.Parameters[k] = 1.0
				case "hardclip", "hard_clip", "hard":
					i.Parameters[k] = 2.0
				case "foldback", "fold", "wavefold":
					i.Parameters[k] = 3.0
				default:
					if f, err := strconv.ParseFloat(val, 32); err == nil {
						i.Parameters[k] = float32(f)
					}
				}
			}
		}
	}
	return nil
}

type NoteEvent struct {
	Time           float64      `json:"time"`
	Duration       float64      `json:"duration"`
	Pitch          int          `json:"pitch"`
	OriginalPitch  int          `json:"original_pitch,omitempty"`
	IsTransposed   bool         `json:"is_transposed,omitempty"`
	Velocity       float32      `json:"velocity"`
	TimingOffsetMs float64      `json:"timing_offset_ms,omitempty"`
	Accent         float64      `json:"accent,omitempty"`
	AttackSec      float64      `json:"attack,omitempty"`
	DecaySec       float64      `json:"decay,omitempty"`
	ReleaseSec     float64      `json:"release,omitempty"`
	PhraseID       int          `json:"phrase_id,omitempty"`
	Vibrato        *Vibrato     `json:"vibrato,omitempty"`
	Expression     *float64     `json:"expression,omitempty"`
	Articulation   Articulation `json:"articulation,omitempty"`
}

type Articulation struct {
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params,omitempty"`
}

func (n NoteEvent) End() float64 { return n.Time + n.Duration }

func (n NoteEvent) PitchName() string {
	names := [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	return fmt.Sprintf("%s%d", names[((n.Pitch%12)+12)%12], n.Pitch/12-1)
}

func (s *Score) ApplyTransposition() {
	if s == nil {
		return
	}
	for i := range s.Tracks {
		if s.Tracks[i].Notes == nil {
			continue
		}
		shift := s.Metadata.Transpose + s.Tracks[i].Transpose
		notesCopy := make([]NoteEvent, len(s.Tracks[i].Notes))
		for j := range s.Tracks[i].Notes {
			n := s.Tracks[i].Notes[j]
			if !n.IsTransposed {
				n.OriginalPitch = n.Pitch
				n.Pitch = n.Pitch + shift
				n.IsTransposed = true
			} else {
				n.Pitch = n.OriginalPitch + shift
			}
			notesCopy[j] = n
		}
		s.Tracks[i].Notes = notesCopy
	}
}

func (s *Score) Clone() *Score {
	if s == nil {
		return nil
	}
	cp := *s
	if s.Tracks != nil {
		cp.Tracks = make([]Track, len(s.Tracks))
		for i := range s.Tracks {
			cp.Tracks[i] = s.Tracks[i]
			if s.Tracks[i].Notes != nil {
				cp.Tracks[i].Notes = make([]NoteEvent, len(s.Tracks[i].Notes))
				copy(cp.Tracks[i].Notes, s.Tracks[i].Notes)
			}
			if s.Tracks[i].Pedal != nil {
				cp.Tracks[i].Pedal = make([]PedalEvent, len(s.Tracks[i].Pedal))
				copy(cp.Tracks[i].Pedal, s.Tracks[i].Pedal)
			}
			if s.Tracks[i].CC != nil {
				cp.Tracks[i].CC = make([]CCEvent, len(s.Tracks[i].CC))
				copy(cp.Tracks[i].CC, s.Tracks[i].CC)
			}
			if s.Tracks[i].Vibrato != nil {
				v := *s.Tracks[i].Vibrato
				cp.Tracks[i].Vibrato = &v
			}
			if s.Tracks[i].Expression != nil {
				e := *s.Tracks[i].Expression
				cp.Tracks[i].Expression = &e
			}
			if s.Tracks[i].Room != nil {
				r := *s.Tracks[i].Room
				cp.Tracks[i].Room = &r
			}
			if s.Tracks[i].Delay != nil {
				d := *s.Tracks[i].Delay
				cp.Tracks[i].Delay = &d
			}
			if s.Tracks[i].Instrument.Parameters != nil {
				params := make(map[string]float32, len(s.Tracks[i].Instrument.Parameters))
				for k, v := range s.Tracks[i].Instrument.Parameters {
					params[k] = v
				}
				cp.Tracks[i].Instrument.Parameters = params
			}
			if s.Tracks[i].Automation != nil {
				cp.Tracks[i].Automation = s.Tracks[i].Automation.Clone()
			}
			if s.Tracks[i].Distortion != nil {
				dist := *s.Tracks[i].Distortion
				cp.Tracks[i].Distortion = &dist
			}
			if s.Tracks[i].OTT != nil {
				ott := *s.Tracks[i].OTT
				cp.Tracks[i].OTT = &ott
			}
			if s.Tracks[i].Turntable != nil {
				tt := *s.Tracks[i].Turntable
				if s.Tracks[i].Turntable.Triggers != nil {
					tt.Triggers = make([]TurntableTrigger, len(s.Tracks[i].Turntable.Triggers))
					copy(tt.Triggers, s.Tracks[i].Turntable.Triggers)
				}
				cp.Tracks[i].Turntable = &tt
			}
			if s.Tracks[i].TurntableTriggers != nil {
				cp.Tracks[i].TurntableTriggers = make([]TurntableTrigger, len(s.Tracks[i].TurntableTriggers))
				copy(cp.Tracks[i].TurntableTriggers, s.Tracks[i].TurntableTriggers)
			}
		}
	}
	if s.Metadata.OTT != nil {
		mott := *s.Metadata.OTT
		cp.Metadata.OTT = &mott
	}
	if s.Metadata.Turntable != nil {
		mtt := *s.Metadata.Turntable
		if s.Metadata.Turntable.Triggers != nil {
			mtt.Triggers = make([]TurntableTrigger, len(s.Metadata.Turntable.Triggers))
			copy(mtt.Triggers, s.Metadata.Turntable.Triggers)
		}
		cp.Metadata.Turntable = &mtt
	}
	return &cp
}

func (s *Score) TransposedCopy() *Score {
	if s == nil {
		return nil
	}
	cp := s.Clone()
	cp.ApplyTransposition()
	return cp
}

func (s *Score) ApplyTimingOffsets() {
	end := 0.0
	for i := range s.Tracks {
		for _, n := range s.Tracks[i].Notes {
			if e := n.End(); e > end {
				end = e
			}
		}
	}
	for i := range s.Tracks {
		for j := range s.Tracks[i].Notes {
			off := s.Tracks[i].Notes[j].TimingOffsetMs
			if off == 0 {
				continue
			}
			t := s.Tracks[i].Notes[j].Time + off/1000
			if !(t >= 0) {
				t = 0
			} else if t > end {
				t = end
			}
			s.Tracks[i].Notes[j].Time = t
		}
	}
}

const swingTolerance = 0.1

func beatDuration(bpm float64, timeSignature string) float64 {
	if !(bpm > 0) || math.IsInf(bpm, 0) || math.IsNaN(bpm) {
		return 0
	}
	den := 4
	if _, d, ok := strings.Cut(timeSignature, "/"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(d)); err == nil && n > 0 {
			den = n
		} else {
			return 0
		}
	}
	return 60 / bpm * 4 / float64(den)
}

func (s *Score) ApplySwing() {
	beat := beatDuration(s.Metadata.BPM, s.Metadata.TimeSignature)
	if !(beat > 0) || math.IsInf(beat, 0) {
		return
	}
	for i := range s.Tracks {
		swing := s.Tracks[i].Swing
		if swing == 0 {
			swing = s.Metadata.Swing
		}
		if swing == 0 {
			continue
		}
		delay := swing * beat / 3
		for j := range s.Tracks[i].Notes {
			frac := math.Mod(s.Tracks[i].Notes[j].Time/beat, 1)
			if math.Abs(frac-0.5) <= swingTolerance {
				s.Tracks[i].Notes[j].Time += delay
			}
		}
	}
}

func (s *Score) ApplyAccents() {
	for i := range s.Tracks {
		for j := range s.Tracks[i].Notes {
			a := s.Tracks[i].Notes[j].Accent
			if a == 0 || a == 1 {
				continue
			}
			v := float64(s.Tracks[i].Notes[j].Velocity) * a
			if !(v >= 0) {
				v = 0
			} else if v > 1 {
				v = 1
			}
			s.Tracks[i].Notes[j].Velocity = float32(v)
		}
	}
}

func (s *Score) ApplyHumanization(seed uint64) {
	if s == nil {
		return
	}
	if seed == 0 {
		seed = 0x9E3779B97F4A7C15
	}
	for i := range s.Tracks {
		tr := &s.Tracks[i]
		timingMs := tr.HumanizeTimingMs
		if timingMs <= 0 {
			timingMs = s.Metadata.HumanizeTimingMs
		}
		velJitter := tr.HumanizeVelocity
		if velJitter <= 0 {
			velJitter = s.Metadata.HumanizeVelocity
		}
		if timingMs <= 0 && velJitter <= 0 {
			continue
		}
		rngState := seed + uint64(i)*0x9E3779B97F4A7C15
		nextUniform := func() float64 {
			rngState = rngState*6364136223846793005 + 1442695040888963407
			return float64(rngState>>11) / float64(uint64(1)<<53)
		}
		nextGauss := func() float64 {
			u1 := nextUniform()
			for u1 <= 1e-12 {
				u1 = nextUniform()
			}
			u2 := nextUniform()
			return math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
		}
		for ni := range tr.Notes {
			n := &tr.Notes[ni]
			if timingMs > 0 {
				dt := nextGauss() * (timingMs * 0.001)
				n.Time += dt
				if n.Time < 0 {
					n.Time = 0
				}
			}
			if velJitter > 0 {
				v := float64(n.Velocity) * (1.0 + nextGauss()*velJitter)
				if v < 0 {
					v = 0
				} else if v > 1 {
					v = 1
				}
				n.Velocity = float32(v)
			}
		}
	}
}

type Vibrato struct {
	Rate  float64 `json:"rate,omitempty"`
	Depth float64 `json:"depth,omitempty"`
}

type RoomConfig struct {
	Size    float64 `json:"size,omitempty"`
	Damping float64 `json:"damping,omitempty"`
	Width   float64 `json:"width,omitempty"`
}

type Room struct {
	IsPreset bool
	Preset   string
	Config   RoomConfig
}

var roomPresets = []string{"cathedral", "hall", "chapel", "room", "plate", "none"}

func IsRoomPreset(name string) bool {
	for _, p := range roomPresets {
		if p == name {
			return true
		}
	}
	return false
}

func (r *Room) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return fmt.Errorf("room: %w", err)
		}
		r.IsPreset = true
		r.Preset = strings.ToLower(strings.TrimSpace(name))
		return nil
	}
	var cfg RoomConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("room: %w", err)
	}
	r.IsPreset = false
	r.Config = cfg
	return nil
}

func (s *Score) TotalNotes() int {
	total := 0
	for i := range s.Tracks {
		total += len(s.Tracks[i].Notes)
	}
	return total
}

func (s *Score) Duration() float64 {
	end := 0.0
	for i := range s.Tracks {
		for _, n := range s.Tracks[i].Notes {
			if e := n.End(); e > end {
				end = e
			}
		}
	}
	return end
}
