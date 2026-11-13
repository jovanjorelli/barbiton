package score

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var KnownArticulations = []string{"staccato", "legato", "tenuto"}

const (
	MinMIDIPitch = 0

	MaxMIDIPitch = 127

	MinBPM = 0

	MaxBPM = 1000
)

func Validate(s *Score) error {
	if s == nil {
		return errors.New("score is nil")
	}
	var errs []error
	errs = append(errs, validateMetadata(s.Metadata)...)
	if len(s.Tracks) == 0 {
		errs = append(errs, errors.New("score must contain at least one track"))
	}
	seen := make(map[string]int, len(s.Tracks))
	for i := range s.Tracks {
		errs = append(errs, validateTrack(&s.Tracks[i], i, s.Metadata.Transpose, seen)...)
	}
	for i := range s.Tracks {
		scSrc := s.Tracks[i].SidechainSource
		if scSrc != "" {
			if _, exists := seen[scSrc]; !exists {
				errs = append(errs, fmt.Errorf("tracks[%d] %q: sidechain_source %q not found", i, s.Tracks[i].ID, scSrc))
			}
		}
	}
	return errors.Join(errs...)
}

func validateMetadata(m Metadata) []error {
	var errs []error
	if strings.TrimSpace(m.Title) == "" {
		errs = append(errs, errors.New("metadata.title is required"))
	}
	if !(m.BPM > MinBPM) || m.BPM > MaxBPM || math.IsNaN(m.BPM) {
		errs = append(errs, fmt.Errorf("metadata.bpm must be in (%g, %g], got %g", float64(MinBPM), float64(MaxBPM), m.BPM))
	}
	if err := validateTimeSignature(m.TimeSignature); err != nil {
		errs = append(errs, err)
	}
	if !(m.Swing >= 0) || m.Swing > 1 {
		errs = append(errs, fmt.Errorf("metadata.swing must be in [0, 1], got %g", m.Swing))
	}
	if !(m.BreathMs >= 0) || math.IsInf(m.BreathMs, 0) || math.IsNaN(m.BreathMs) {
		errs = append(errs, fmt.Errorf("metadata.breath must be a finite value >= 0 ms, got %g", m.BreathMs))
	}
	if m.HumanizeTimingMs < 0 || math.IsNaN(m.HumanizeTimingMs) || math.IsInf(m.HumanizeTimingMs, 0) {
		errs = append(errs, fmt.Errorf("metadata.humanize_timing_ms must be >= 0, got %g", m.HumanizeTimingMs))
	}
	if m.HumanizeVelocity < 0 || m.HumanizeVelocity > 1 || math.IsNaN(m.HumanizeVelocity) || math.IsInf(m.HumanizeVelocity, 0) {
		errs = append(errs, fmt.Errorf("metadata.humanize_velocity must be in [0, 1], got %g", m.HumanizeVelocity))
	}
	if m.OTT != nil {
		errs = append(errs, validateOTT(m.OTT, "metadata.ott")...)
	}
	if m.Turntable != nil {
		errs = append(errs, validateTurntable(m.Turntable, "metadata.turntable")...)
	}
	return errs
}

func validateTimeSignature(ts string) error {
	num, den, ok := strings.Cut(ts, "/")
	if ok {
		n, err1 := strconv.Atoi(strings.TrimSpace(num))
		d, err2 := strconv.Atoi(strings.TrimSpace(den))
		if err1 == nil && err2 == nil && n > 0 && d > 0 {
			return nil
		}
	}
	return fmt.Errorf("metadata.time_signature must be \"N/D\" with positive integers, got %q", ts)
}

func validateTrack(t *Track, index int, globalTranspose int, seen map[string]int) []error {
	var errs []error
	label := fmt.Sprintf("tracks[%d] %q", index, t.ID)

	if strings.TrimSpace(t.ID) == "" {
		errs = append(errs, fmt.Errorf("tracks[%d]: id is required", index))
	} else if prev, dup := seen[t.ID]; dup {
		errs = append(errs, fmt.Errorf("%s: duplicate track id (first used by tracks[%d])", label, prev))
	} else {
		seen[t.ID] = index
	}

	if strings.TrimSpace(t.Instrument.Type) == "" {
		errs = append(errs, fmt.Errorf("%s: instrument.type is required", label))
	}
	if strings.EqualFold(t.Instrument.Type, "sampler") && strings.TrimSpace(t.Instrument.File) == "" {
		errs = append(errs, fmt.Errorf("%s: instrument.file is required for sampler instruments", label))
	}

	if t.Pan < -1 || t.Pan > 1 {
		errs = append(errs, fmt.Errorf("%s: pan must be in [-1, 1], got %g", label, t.Pan))
	}
	if t.Volume < 0 || t.Volume > 1 {
		errs = append(errs, fmt.Errorf("%s: volume must be in [0, 1], got %g", label, t.Volume))
	}
	if t.ReverbSend < 0 || t.ReverbSend > 1 {
		errs = append(errs, fmt.Errorf("%s: reverb_send must be in [0, 1], got %g", label, t.ReverbSend))
	}
	if !(t.Swing >= 0) || t.Swing > 1 {
		errs = append(errs, fmt.Errorf("%s: swing must be in [0, 1], got %g", label, t.Swing))
	}
	if !(t.BreathMs >= 0) || math.IsInf(t.BreathMs, 0) || math.IsNaN(t.BreathMs) {
		errs = append(errs, fmt.Errorf("%s: breath must be a finite value >= 0 ms, got %g", label, t.BreathMs))
	}
	if t.HumanizeTimingMs < 0 || math.IsNaN(t.HumanizeTimingMs) || math.IsInf(t.HumanizeTimingMs, 0) {
		errs = append(errs, fmt.Errorf("%s: humanize_timing_ms must be >= 0, got %g", label, t.HumanizeTimingMs))
	}
	if t.HumanizeVelocity < 0 || t.HumanizeVelocity > 1 || math.IsNaN(t.HumanizeVelocity) || math.IsInf(t.HumanizeVelocity, 0) {
		errs = append(errs, fmt.Errorf("%s: humanize_velocity must be in [0, 1], got %g", label, t.HumanizeVelocity))
	}
	if t.SidechainAttackMs < 0 || math.IsNaN(float64(t.SidechainAttackMs)) || math.IsInf(float64(t.SidechainAttackMs), 0) {
		errs = append(errs, fmt.Errorf("%s: sidechain_attack_ms must be >= 0, got %g", label, t.SidechainAttackMs))
	}
	if t.SidechainReleaseMs < 0 || math.IsNaN(float64(t.SidechainReleaseMs)) || math.IsInf(float64(t.SidechainReleaseMs), 0) {
		errs = append(errs, fmt.Errorf("%s: sidechain_release_ms must be >= 0, got %g", label, t.SidechainReleaseMs))
	}
	if t.SidechainHoldMs < 0 || math.IsNaN(float64(t.SidechainHoldMs)) || math.IsInf(float64(t.SidechainHoldMs), 0) {
		errs = append(errs, fmt.Errorf("%s: sidechain_hold_ms must be >= 0, got %g", label, t.SidechainHoldMs))
	}
	if t.SidechainRatio < 0 || math.IsNaN(float64(t.SidechainRatio)) || math.IsInf(float64(t.SidechainRatio), 0) {
		errs = append(errs, fmt.Errorf("%s: sidechain_ratio must be >= 0, got %g", label, t.SidechainRatio))
	}
	if t.SidechainSource != "" && t.SidechainSource == t.ID {
		errs = append(errs, fmt.Errorf("%s: sidechain_source cannot refer to itself", label))
	}
	errs = append(errs, validateRoom(t.Room, label)...)
	errs = append(errs, validateEQ(t.EQ, label)...)
	errs = append(errs, validateDelay(t.Delay, label)...)
	for j := range t.Pedal {
		p := t.Pedal[j]
		if math.IsNaN(p.Time) || math.IsInf(p.Time, 0) || p.Time < 0 {
			errs = append(errs, fmt.Errorf("%s: pedal[%d].time must be finite seconds >= 0, got %g", label, j, p.Time))
		}
	}
	for j := range t.CC {
		c := t.CC[j]
		if math.IsNaN(c.Time) || math.IsInf(c.Time, 0) || c.Time < 0 {
			errs = append(errs, fmt.Errorf("%s: cc[%d].time must be finite seconds >= 0, got %g", label, j, c.Time))
		}
		if c.Controller < 0 || c.Controller > 127 {
			errs = append(errs, fmt.Errorf("%s: cc[%d].controller must be in [0, 127], got %d", label, j, c.Controller))
		}
		if c.Value < 0 || c.Value > 127 {
			errs = append(errs, fmt.Errorf("%s: cc[%d].value must be in [0, 127], got %d", label, j, c.Value))
		}
	}
	for k, v := range t.Instrument.Parameters {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			errs = append(errs, fmt.Errorf("%s: instrument.parameters[%q] must be finite, got %g", label, k, v))
		}
	}
	if t.Automation != nil {
		errs = append(errs, ValidateAutomationMap(t.Automation)...)
	}
	if t.Distortion != nil {
		errs = append(errs, validateDistortion(t.Distortion, label)...)
	}
	if t.OTT != nil {
		errs = append(errs, validateOTT(t.OTT, label)...)
	}
	if t.Turntable != nil {
		errs = append(errs, validateTurntable(t.Turntable, label)...)
	}
	if len(t.TurntableTriggers) > 0 {
		errs = append(errs, validateTurntableTriggers(t.TurntableTriggers, label)...)
	}

	for j := range t.Notes {
		if err := checkTransposedPitch(&t.Notes[j], t, globalTranspose, index, j); err != nil {

			errs = append(errs, err)
			errs = append(errs, validateNote(&t.Notes[j], label, j, true)...)
			continue
		}
		errs = append(errs, validateNote(&t.Notes[j], label, j, false)...)
	}
	return errs
}

func checkTransposedPitch(n *NoteEvent, t *Track, globalTranspose, trackIndex, noteIndex int) error {
	shift := globalTranspose + t.Transpose
	if shift == 0 {
		return nil
	}
	var original, resulting int
	if n.IsTransposed {
		original = n.OriginalPitch
		resulting = n.Pitch
	} else if (n.Pitch < MinMIDIPitch || n.Pitch > MaxMIDIPitch) && (n.Pitch-shift >= MinMIDIPitch && n.Pitch-shift <= MaxMIDIPitch) {
		original = n.Pitch - shift
		resulting = n.Pitch
	} else {
		original = n.Pitch
		resulting = n.Pitch + shift
	}
	if original < MinMIDIPitch || original > MaxMIDIPitch {
		return nil
	}
	if resulting >= MinMIDIPitch && resulting <= MaxMIDIPitch {
		return nil
	}
	name := strings.TrimSpace(t.Name)
	if name == "" {
		name = strings.TrimSpace(t.ID)
	}
	if name == "" {
		name = fmt.Sprintf("tracks[%d]", trackIndex)
	}
	if resulting > MaxMIDIPitch {
		return fmt.Errorf("track %s, note index %d, original pitch %d, global transpose %+d, track transpose %+d, resulting pitch %d exceeds maximum %d",
			name, noteIndex, original, globalTranspose, t.Transpose, resulting, MaxMIDIPitch)
	}
	return fmt.Errorf("track %s, note index %d, original pitch %d, global transpose %+d, track transpose %+d, resulting pitch %d below minimum %d",
		name, noteIndex, original, globalTranspose, t.Transpose, resulting, MinMIDIPitch)
}

func validateNote(n *NoteEvent, trackLabel string, index int, skipPitch bool) []error {
	var errs []error
	at := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Errorf("%s: notes[%d]: %s", trackLabel, index, fmt.Sprintf(format, args...)))
	}

	if !(n.Time >= 0) || math.IsInf(n.Time, 0) {
		at("time must be a finite value >= 0, got %g", n.Time)
	}
	if !(n.Duration > 0) || math.IsInf(n.Duration, 0) {
		at("duration must be a finite value > 0, got %g", n.Duration)
	}
	pitchToCheck := n.Pitch
	if n.IsTransposed {
		pitchToCheck = n.OriginalPitch
	}
	if pitchToCheck < MinMIDIPitch || pitchToCheck > MaxMIDIPitch {
		if !skipPitch {
			at("pitch must be a MIDI note in [%d, %d], got %d", MinMIDIPitch, MaxMIDIPitch, pitchToCheck)
		}
	}
	if !(n.Velocity >= 0) || n.Velocity > 1 {
		at("velocity must be in [0, 1], got %g", n.Velocity)
	}
	if n.Articulation.Type != "" && !isKnownArticulation(n.Articulation.Type) {
		at("unknown articulation type %q (known: %s)", n.Articulation.Type, strings.Join(KnownArticulations, ", "))
	}
	return errs
}

func validateEQ(eq TrackEQ, label string) []error {
	var errs []error
	if eq.HPF < 0 || eq.HPF >= 100000 {
		errs = append(errs, fmt.Errorf("%s: eq.hpf must be 0 (off) or in (0, 100000) Hz, got %g", label, eq.HPF))
	}
	band := func(name string, b EQBand) {
		if b.Freq == 0 && b.Gain == 0 && b.Q == 0 {
			return
		}
		if b.Freq <= 0 || b.Freq >= 100000 {
			errs = append(errs, fmt.Errorf("%s: %s.freq must be in (0, 100000) Hz, got %g", label, name, b.Freq))
		}
		if b.Gain < -36 || b.Gain > 36 {
			errs = append(errs, fmt.Errorf("%s: %s.gain must be in [-36, 36] dB, got %g", label, name, b.Gain))
		}
		if b.Q < 0 || b.Q > 50 {
			errs = append(errs, fmt.Errorf("%s: %s.q must be in [0, 50], got %g", label, name, b.Q))
		}
	}
	band("eq_low", eq.LowShelf)
	band("eq_mid", eq.MidPeak)
	band("eq_high", eq.HighShelf)
	return errs
}

func validateDelay(d *TrackDelay, label string) []error {
	if d == nil {
		return nil
	}
	var errs []error
	switch strings.ToLower(strings.TrimSpace(d.Mode)) {
	case "", "stereo", "pingpong", "ping-pong", "ping_pong":
	default:
		errs = append(errs, fmt.Errorf("%s: delay.mode must be \"stereo\" or \"pingpong\", got %q", label, d.Mode))
	}
	if strings.TrimSpace(d.Subdivision) != "" {
		switch strings.ToLower(strings.TrimSpace(d.Subdivision)) {
		case "1/1", "1/2", "1/4", "1/8", "1/8d", "1/16", "1/16t",
			"whole", "half", "quarter", "eighth", "sixteenth":
		default:
			errs = append(errs, fmt.Errorf("%s: delay.subdivision must be one of 1/1 1/2 1/4 1/8 1/8d 1/16 1/16t, got %q", label, d.Subdivision))
		}
	}
	if !(d.Seconds >= 0) || math.IsInf(d.Seconds, 0) || d.Seconds > 4 {
		errs = append(errs, fmt.Errorf("%s: delay.seconds must be in [0, 4], got %g", label, d.Seconds))
	}
	if !(d.Feedback >= 0) || d.Feedback > 0.98 {
		errs = append(errs, fmt.Errorf("%s: delay.feedback must be in [0, 0.98], got %g", label, d.Feedback))
	}
	if d.DampingHz < 0 || d.DampingHz >= 100000 {
		errs = append(errs, fmt.Errorf("%s: delay.damping_hz must be in [0, 100000), got %g", label, d.DampingHz))
	}
	if !(d.Wet >= 0) || d.Wet > 1 {
		errs = append(errs, fmt.Errorf("%s: delay.wet must be in [0, 1], got %g", label, d.Wet))
	}
	return errs
}

func validateRoom(r *Room, label string) []error {
	if r == nil {
		return nil
	}
	if r.IsPreset {
		if !IsRoomPreset(r.Preset) {
			return []error{fmt.Errorf("%s: room must be \"cathedral\", \"hall\", \"chapel\", \"room\", \"plate\", or \"none\", got %q", label, r.Preset)}
		}
		return nil
	}
	var errs []error
	check := func(name string, v float64) {
		if !(v >= 0) || v > 1 {
			errs = append(errs, fmt.Errorf("%s: room.%s must be in [0, 1], got %g", label, name, v))
		}
	}
	check("size", r.Config.Size)
	check("damping", r.Config.Damping)
	check("width", r.Config.Width)
	return errs
}

func Warnings(s *Score) []string {
	if s == nil {
		return nil
	}
	var out []string
	for i := range s.Tracks {
		t := &s.Tracks[i]
		label := fmt.Sprintf("tracks[%d] %q", i, t.ID)
		if t.Delay != nil && t.Delay.Wet == 0 {
			out = append(out, fmt.Sprintf("%s: delay.wet is 0, so the delay will be inaudible", label))
		}
	}
	return out
}

func validateDistortion(d *TrackDistortion, label string) []error {
	if d == nil {
		return nil
	}
	var errs []error
	m := strings.ToLower(strings.TrimSpace(d.Mode))
	if m != "" && m != "tube" && m != "diode" && m != "hardclip" && m != "hard_clip" && m != "foldback" && m != "fold" {
		errs = append(errs, fmt.Errorf("%s: distortion.mode unknown %q", label, d.Mode))
	}
	if d.Drive < 0 || math.IsNaN(float64(d.Drive)) || math.IsInf(float64(d.Drive), 0) {
		errs = append(errs, fmt.Errorf("%s: distortion.drive must be >= 0, got %g", label, d.Drive))
	}
	if d.Mix < 0 || d.Mix > 1 || math.IsNaN(float64(d.Mix)) || math.IsInf(float64(d.Mix), 0) {
		errs = append(errs, fmt.Errorf("%s: distortion.mix must be in [0, 1], got %g", label, d.Mix))
	}
	if d.Tone < 0 || math.IsNaN(float64(d.Tone)) || math.IsInf(float64(d.Tone), 0) {
		errs = append(errs, fmt.Errorf("%s: distortion.tone must be >= 0, got %g", label, d.Tone))
	}
	if math.IsNaN(float64(d.OutputGainDB)) || math.IsInf(float64(d.OutputGainDB), 0) {
		errs = append(errs, fmt.Errorf("%s: distortion.output_gain_db must be finite, got %g", label, d.OutputGainDB))
	}
	return errs
}

func validateOTT(o *TrackOTT, label string) []error {
	if o == nil {
		return nil
	}
	var errs []error
	if o.Depth < 0 || o.Depth > 1 || math.IsNaN(float64(o.Depth)) || math.IsInf(float64(o.Depth), 0) {
		errs = append(errs, fmt.Errorf("%s: ott.depth must be in [0, 1], got %g", label, o.Depth))
	}
	if o.TimeScale < 0 || math.IsNaN(float64(o.TimeScale)) || math.IsInf(float64(o.TimeScale), 0) {
		errs = append(errs, fmt.Errorf("%s: ott.time_scale must be >= 0, got %g", label, o.TimeScale))
	}
	if math.IsNaN(float64(o.InGainDB)) || math.IsInf(float64(o.InGainDB), 0) {
		errs = append(errs, fmt.Errorf("%s: ott.in_gain_db must be finite, got %g", label, o.InGainDB))
	}
	if math.IsNaN(float64(o.OutGainDB)) || math.IsInf(float64(o.OutGainDB), 0) {
		errs = append(errs, fmt.Errorf("%s: ott.out_gain_db must be finite, got %g", label, o.OutGainDB))
	}
	for i, g := range o.BandGains {
		if math.IsNaN(float64(g)) || math.IsInf(float64(g), 0) {
			errs = append(errs, fmt.Errorf("%s: ott.band_gains[%d] must be finite, got %g", label, i, g))
		}
	}
	return errs
}

func isKnownArticulation(t string) bool {
	for _, known := range KnownArticulations {
		if t == known {
			return true
		}
	}
	return false
}

func validateTurntable(tt *TrackTurntable, label string) []error {
	if tt == nil {
		return nil
	}
	var errs []error
	if tt.InertiaMs < 0 || math.IsNaN(tt.InertiaMs) || math.IsInf(tt.InertiaMs, 0) {
		errs = append(errs, fmt.Errorf("%s: turntable.inertia_ms must be >= 0, got %g", label, tt.InertiaMs))
	}
	if len(tt.Triggers) > 0 {
		errs = append(errs, validateTurntableTriggers(tt.Triggers, label)...)
	}
	return errs
}

func isValidTurntableSubdivision(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1/1", "whole", "1/2", "half", "1/4", "quarter", "1/8", "eighth", "1/8d", "1/16", "sixteenth", "1/16t", "1/32", "thirtysecond", "thirty-second":
		return true
	}
	return false
}

func validateTurntableTriggers(triggers []TurntableTrigger, label string) []error {
	var errs []error
	for i, tr := range triggers {
		if tr.Time < 0 || math.IsNaN(tr.Time) || math.IsInf(tr.Time, 0) {
			errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].time must be >= 0, got %g", label, i, tr.Time))
		}
		typ := strings.ToLower(strings.TrimSpace(tr.Type))
		switch typ {
		case "brake", "tape_stop", "scratch", "beat_roll", "stutter", "backspin", "release", "resume", "start", "bypass":
		default:
			errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].type unknown %q", label, i, tr.Type))
		}
		if tr.Speed < -3.0 || tr.Speed > 3.0 || math.IsNaN(float64(tr.Speed)) || math.IsInf(float64(tr.Speed), 0) {
			errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].speed must be in [-3, 3], got %g", label, i, tr.Speed))
		}
		if tr.BrakeTimeMs < 0 || math.IsNaN(tr.BrakeTimeMs) || math.IsInf(tr.BrakeTimeMs, 0) {
			errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].brake_time_ms must be >= 0, got %g", label, i, tr.BrakeTimeMs))
		}
		if tr.DecayMs < 0 || math.IsNaN(tr.DecayMs) || math.IsInf(tr.DecayMs, 0) {
			errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].decay_ms must be >= 0, got %g", label, i, tr.DecayMs))
		}
		if tr.Duration < 0 || math.IsNaN(tr.Duration) || math.IsInf(tr.Duration, 0) {
			errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].duration must be >= 0, got %g", label, i, tr.Duration))
		}
		if tr.Curve != "" {
			c := strings.ToLower(strings.TrimSpace(tr.Curve))
			if c != "linear" && c != "exponential" && c != "exp" {
				errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].curve unknown %q, must be linear or exponential", label, i, tr.Curve))
			}
		}
		if tr.Subdivision != "" && !isValidTurntableSubdivision(tr.Subdivision) {
			errs = append(errs, fmt.Errorf("%s: turntable_triggers[%d].subdivision unknown %q", label, i, tr.Subdivision))
		}
	}
	return errs
}
