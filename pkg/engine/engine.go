package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
	"barbiton/pkg/instruments/ocarina"
	"barbiton/pkg/instruments/sampler"
	"barbiton/pkg/instruments/synth"
	"barbiton/pkg/mixer"
	"barbiton/pkg/score"
)

const (
	DefaultSampleRate = 48000

	DefaultBlockSize = 1024

	tailSeconds = 2.5

	DefaultReverbWet = 0.2
)

type noteEvent struct {
	frame       int
	track       int
	pitch       int
	vel         float32
	on          bool
	params      instruments.NoteParams
	isPedal     bool
	pedalDown   bool
	isCC        bool
	ccNum       int
	ccVal       int
	isTurntable bool
	ttTrigger   score.TurntableTrigger
}

type Engine struct {
	score       *score.Score
	sampleRate  float64
	blockSize   int
	insts       []instruments.Instrument
	mix         *mixer.Mixer
	events      []noteEvent
	nextEvent   int
	frame       int
	totalFrames int
	ccParams    map[string]float32

	trackAuto     []score.AutomationMap
	masterAuto    score.AutomationMap
	hasAutomation bool
	autoParams    map[string]float32

	peakL, peakR float32
	trackPeak    []float32
}

func New(s *score.Score, sampleRate float64, blockSize int) (*Engine, error) {
	if err := score.Validate(s); err != nil {
		return nil, fmt.Errorf("invalid score: %w", err)
	}
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}
	if blockSize <= 1 {
		blockSize = DefaultBlockSize
	}

	hasHumanize := s.Metadata.HumanizeTimingMs > 0 || s.Metadata.HumanizeVelocity > 0
	if !hasHumanize {
		for i := range s.Tracks {
			if s.Tracks[i].HumanizeTimingMs > 0 || s.Tracks[i].HumanizeVelocity > 0 {
				hasHumanize = true
				break
			}
		}
	}
	if hasHumanize {
		s = s.Clone()
		s.ApplyHumanization(1)
	}

	e := &Engine{
		score:      s,
		sampleRate: sampleRate,
		blockSize:  blockSize,
		insts:      make([]instruments.Instrument, len(s.Tracks)),
		trackPeak:  make([]float32, len(s.Tracks)),
		ccParams:   make(map[string]float32, 4),
		trackAuto:  make([]score.AutomationMap, len(s.Tracks)),
		autoParams: make(map[string]float32, 16),
	}
	e.mix = mixer.New(int(sampleRate), len(s.Tracks), blockSize)
	e.mix.SetReverbWet(DefaultReverbWet)
	if s.Metadata.Turntable != nil {
		inertia := 30.0
		if s.Metadata.Turntable.InertiaMs > 0 {
			inertia = s.Metadata.Turntable.InertiaMs
		}
		e.mix.SetTurntableConfig(dsp.TurntableConfig{InertiaMs: inertia})
	}
	for i := range s.Tracks {
		tr := &s.Tracks[i]
		if len(tr.Automation) > 0 {
			e.trackAuto[i] = tr.Automation.Clone()
			e.hasAutomation = true
		}
		inst, err := newInstrument(tr.Instrument, sampleRate)
		if err != nil {
			return nil, fmt.Errorf("track %d (%s): %w", i, tr.ID, err)
		}
		if trackHasEQ(tr.EQ) {
			spec := EQSpecFor(tr.EQ)
			inst = &eqInstrument{
				inner: inst,
				eqL:   dsp.NewEQ(sampleRate, spec),
				eqR:   dsp.NewEQ(sampleRate, spec),
			}
		}
		e.insts[i] = inst
		idx := e.mix.AddTrack(inst, tr.Pan, tr.Volume)
		if idx < 0 {
			return nil, fmt.Errorf("track %d (%s): mixer is full", i, tr.ID)
		}
		e.mix.SetSend(idx, tr.ReverbSend)
		if tr.Delay != nil {

			e.mix.SetTrackDelay(idx, delayConfigFor(tr.Delay), s.Metadata.BPM)
		}
		if tr.Distortion != nil {
			mode := dsp.ParseDistortionMode(tr.Distortion.Mode)
			e.mix.SetTrackDistortion(idx, dsp.DistortionConfig{
				Mode:         mode,
				Drive:        tr.Distortion.Drive,
				Tone:         tr.Distortion.Tone,
				Mix:          tr.Distortion.Mix,
				OutputGainDB: tr.Distortion.OutputGainDB,
			})
		}
		if tr.OTT != nil {
			ts := tr.OTT.TimeScale
			if ts <= 0 {
				ts = 1.0
			}
			e.mix.SetTrackOTT(idx, dsp.OTTConfig{
				Depth:     tr.OTT.Depth,
				TimeScale: ts,
				InGainDB:  tr.OTT.InGainDB,
				OutGainDB: tr.OTT.OutGainDB,
				BandGains: tr.OTT.BandGains,
			})
		}
		if tr.Turntable != nil || len(tr.TurntableTriggers) > 0 {
			inertia := 30.0
			if tr.Turntable != nil && tr.Turntable.InertiaMs > 0 {
				inertia = tr.Turntable.InertiaMs
			}
			e.mix.SetTrackTurntable(idx, dsp.TurntableConfig{InertiaMs: inertia})
		}
	}
	e.assignRooms(s)

	for i := range s.Tracks {
		tr := &s.Tracks[i]
		if tr.SidechainSource != "" {
			srcIdx := -1
			for j := range s.Tracks {
				if s.Tracks[j].ID == tr.SidechainSource {
					srcIdx = j
					break
				}
			}
			if srcIdx >= 0 {
				cfg := dsp.DefaultSidechainConfig()
				if tr.SidechainDuckingDB != 0 {
					cfg.ThresholdDB = tr.SidechainDuckingDB
				}
				if tr.SidechainAttackMs > 0 {
					cfg.AttackMs = tr.SidechainAttackMs
				}
				if tr.SidechainReleaseMs > 0 {
					cfg.ReleaseMs = tr.SidechainReleaseMs
				}
				if tr.SidechainHoldMs > 0 {
					cfg.HoldMs = tr.SidechainHoldMs
				}
				if tr.SidechainRatio > 0 {
					cfg.Ratio = tr.SidechainRatio
				}
				if tr.SidechainMakeupDB != 0 {
					cfg.MakeupDB = tr.SidechainMakeupDB
				}
				e.mix.SetTrackSidechain(i, srcIdx, cfg)
			}
		}
	}

	if s.Metadata.ConvolutionIR != "" {
		convCfg := dsp.DefaultConvolutionConfig()
		if s.Metadata.ConvolutionDecayRatio > 0 {
			convCfg.DecayRatio = s.Metadata.ConvolutionDecayRatio
		}
		if s.Metadata.ConvolutionPreDelayMs > 0 {
			convCfg.PreDelayMs = s.Metadata.ConvolutionPreDelayMs
		}
		if s.Metadata.ConvolutionDryWet > 0 {
			convCfg.DryWet = s.Metadata.ConvolutionDryWet
		}
		if s.Metadata.ConvolutionLowCutHz > 0 {
			convCfg.LowCutHz = s.Metadata.ConvolutionLowCutHz
		}
		if s.Metadata.ConvolutionHighCutHz > 0 {
			convCfg.HighCutHz = s.Metadata.ConvolutionHighCutHz
		}
		e.mix.SetConvolutionConfig(convCfg)
		_ = e.mix.SetConvolutionIRFile(s.Metadata.ConvolutionIR)
	}

	if s.Metadata.MultibandEnabled {
		e.mix.SetMultibandEnabled(true)
	}

	if s.Metadata.OversamplingEnabled {
		e.mix.SetOversamplingEnabled(true)
	}

	if s.Metadata.OTT != nil {
		ts := s.Metadata.OTT.TimeScale
		if ts <= 0 {
			ts = 1.0
		}
		e.mix.SetOTTConfig(dsp.OTTConfig{
			Depth:     s.Metadata.OTT.Depth,
			TimeScale: ts,
			InGainDB:  s.Metadata.OTT.InGainDB,
			OutGainDB: s.Metadata.OTT.OutGainDB,
			BandGains: s.Metadata.OTT.BandGains,
		})
		e.mix.SetOTTEnabled(true)
	}

	for i := range s.Tracks {
		durations := breathDurations(&s.Tracks[i], s.Metadata.BreathMs)
		for j, t := range breathTimes(&s.Tracks[i], s.Metadata.BreathMs) {
			n := s.Tracks[i].Notes[j]
			on := int(math.Round(t * sampleRate))
			hold := durations[j]
			if strings.EqualFold(n.Articulation.Type, "staccato") {
				hold *= 0.5
			}
			off := int(math.Round((t + hold) * sampleRate))
			if off <= on {
				off = on + 1
			}
			e.events = append(e.events,
				noteEvent{frame: on, track: i, pitch: n.Pitch, vel: n.Velocity, on: true, params: paramsFor(n, &s.Tracks[i])},
				noteEvent{frame: off, track: i, pitch: n.Pitch, on: false})
		}
		for _, pd := range s.Tracks[i].Pedal {
			e.events = append(e.events,
				noteEvent{frame: int(math.Round(pd.Time * sampleRate)), track: i, isPedal: true, pedalDown: pd.Down})
		}
		for _, cc := range s.Tracks[i].CC {
			e.events = append(e.events,
				noteEvent{
					frame: int(math.Round(cc.Time * sampleRate)),
					track: i,
					isCC:  true,
					ccNum: cc.Controller,
					ccVal: cc.Value,
				})
		}
		for _, tt := range s.Tracks[i].TurntableTriggers {
			e.events = append(e.events,
				noteEvent{
					frame:       int(math.Round(tt.Time * sampleRate)),
					track:       i,
					isTurntable: true,
					ttTrigger:   tt,
				})
			if tt.Duration > 0 && isTimedTurntableType(tt.Type) {
				e.events = append(e.events,
					noteEvent{
						frame:       int(math.Round((tt.Time + tt.Duration) * sampleRate)),
						track:       i,
						isTurntable: true,
						ttTrigger:   score.TurntableTrigger{Type: "resume"},
					})
			}
		}
		if s.Tracks[i].Turntable != nil {
			for _, tt := range s.Tracks[i].Turntable.Triggers {
				e.events = append(e.events,
					noteEvent{
						frame:       int(math.Round(tt.Time * sampleRate)),
						track:       i,
						isTurntable: true,
						ttTrigger:   tt,
					})
				if tt.Duration > 0 && isTimedTurntableType(tt.Type) {
					e.events = append(e.events,
						noteEvent{
							frame:       int(math.Round((tt.Time + tt.Duration) * sampleRate)),
							track:       i,
							isTurntable: true,
							ttTrigger:   score.TurntableTrigger{Type: "resume"},
						})
				}
			}
		}
	}
	if s.Metadata.Turntable != nil {
		for _, tt := range s.Metadata.Turntable.Triggers {
			e.events = append(e.events,
				noteEvent{
					frame:       int(math.Round(tt.Time * sampleRate)),
					track:       -1,
					isTurntable: true,
					ttTrigger:   tt,
				})
			if tt.Duration > 0 && isTimedTurntableType(tt.Type) {
				e.events = append(e.events,
					noteEvent{
						frame:       int(math.Round((tt.Time + tt.Duration) * sampleRate)),
						track:       -1,
						isTurntable: true,
						ttTrigger:   score.TurntableTrigger{Type: "resume"},
					})
			}
		}
	}
	sort.SliceStable(e.events, func(a, b int) bool {
		if e.events[a].frame != e.events[b].frame {
			return e.events[a].frame < e.events[b].frame
		}
		if e.events[a].isTurntable != e.events[b].isTurntable {
			return e.events[a].isTurntable
		}
		if e.events[a].isCC != e.events[b].isCC {
			return e.events[a].isCC
		}
		if e.events[a].isPedal != e.events[b].isPedal {
			return e.events[a].isPedal
		}
		return !e.events[a].on && e.events[b].on
	})
	lastEventFrame := 0
	if len(e.events) > 0 {
		lastEventFrame = e.events[len(e.events)-1].frame
	}
	scoreDurationFrames := int(math.Round(s.Duration() * sampleRate))
	if lastEventFrame > scoreDurationFrames {
		scoreDurationFrames = lastEventFrame
	}
	e.totalFrames = scoreDurationFrames + int(tailSeconds*sampleRate)
	return e, nil
}

func newInstrument(def score.InstrumentDef, sampleRate float64) (instruments.Instrument, error) {
	switch strings.ToLower(def.Type) {
	case "ocarina":
		o := ocarina.New(sampleRate)
		o.SetParameters(def.Parameters)
		return o, nil
	case "synth":
		sy := synth.New(sampleRate)
		sy.SetParameters(def.Parameters)
		return sy, nil
	case "sampler":
		if strings.TrimSpace(def.File) == "" {
			return nil, fmt.Errorf("sampler instrument needs a file path")
		}
		sm, err := sampler.LoadFile(def.File, sampleRate)
		if err != nil {
			return nil, err
		}
		sm.SetParameters(def.Parameters)
		return sm, nil
	}
	return nil, fmt.Errorf("unknown instrument type %q (want ocarina, synth, or sampler)", def.Type)
}

func trackHasEQ(eq score.TrackEQ) bool {
	return eq.HPF > 0 || eq.LowShelf.Freq > 0 || eq.MidPeak.Freq > 0 || eq.HighShelf.Freq > 0
}

func EQSpecFor(eq score.TrackEQ) dsp.EQSpec {
	band := func(b score.EQBand) dsp.EQBandSpec {
		return dsp.EQBandSpec{Freq: b.Freq, GainDB: b.Gain, Q: b.Q}
	}
	return dsp.EQSpec{
		HPF:       eq.HPF,
		LowShelf:  band(eq.LowShelf),
		MidPeak:   band(eq.MidPeak),
		HighShelf: band(eq.HighShelf),
	}
}

func overrideFor(n score.NoteEvent) instruments.EnvelopeOverride {
	return instruments.EnvelopeOverride{
		Attack:     n.AttackSec,
		Decay:      n.DecaySec,
		Release:    n.ReleaseSec,
		HasAttack:  n.AttackSec > 0,
		HasDecay:   n.DecaySec > 0,
		HasRelease: n.ReleaseSec > 0,
	}
}

const (
	defaultVibRate  = 5.0
	defaultVibDepth = 0.03
)

func paramsFor(n score.NoteEvent, tr *score.Track) instruments.NoteParams {
	p := instruments.NoteParams{Env: overrideFor(n)}
	vib := tr.Vibrato
	if n.Vibrato != nil {
		vib = n.Vibrato
	}
	if vib != nil {
		rate, depth := vib.Rate, vib.Depth
		if !(rate > 0) {
			rate = defaultVibRate
		}
		if !(depth > 0) {
			depth = defaultVibDepth
		}
		p.HasVibrato = true
		p.VibratoRate = rate
		p.VibratoDepth = depth
	}
	expr := tr.Expression
	if n.Expression != nil {
		expr = n.Expression
	}
	p.Expression, p.HasExpression = 1, true
	if expr != nil {
		v := *expr
		if math.IsNaN(v) {
			v = 1
		} else if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		p.Expression = float32(v)
	}
	return p
}

func delayConfigFor(td *score.TrackDelay) dsp.DelayConfig {
	mode := dsp.DelayModeStereo
	switch strings.ToLower(strings.TrimSpace(td.Mode)) {
	case "pingpong", "ping-pong", "ping_pong":
		mode = dsp.DelayModePingPong
	}
	sub := dsp.SubdivisionQuarter
	if name := strings.TrimSpace(td.Subdivision); name != "" {
		if parsed, ok := dsp.ParseSubdivision(name); ok {
			sub = parsed
		}
	}
	return dsp.DelayConfig{
		Mode:         mode,
		Subdivision:  sub,
		DelaySeconds: td.Seconds,
		Feedback:     td.Feedback,
		DampingHz:    td.DampingHz,
		Wet:          td.Wet,
	}
}

type eqInstrument struct {
	inner instruments.Instrument
	eqL   *dsp.EQ
	eqR   *dsp.EQ
}

func (e *eqInstrument) Process(buffer *dsp.Buffer, deltaTime float64) {
	e.inner.Process(buffer, deltaTime)
	e.eqL.ProcessInPlace(buffer.Left)
	e.eqR.ProcessInPlace(buffer.Right)
}

func (e *eqInstrument) NoteOn(pitch int, velocity float32) { e.inner.NoteOn(pitch, velocity) }

func (e *eqInstrument) NoteOff(pitch int) { e.inner.NoteOff(pitch) }

func (e *eqInstrument) NoteOnParams(pitch int, velocity float32, params instruments.NoteParams) {
	if ex, ok := e.inner.(instruments.ParamsVoicer); ok {
		ex.NoteOnParams(pitch, velocity, params)
		return
	}
	e.inner.NoteOn(pitch, velocity)
}

func (e *eqInstrument) NoteOnEx(pitch int, velocity float32, env instruments.EnvelopeOverride) {
	if ex, ok := e.inner.(instruments.EnvelopeVoicer); ok {
		ex.NoteOnEx(pitch, velocity, env)
		return
	}
	e.inner.NoteOn(pitch, velocity)
}

func (e *eqInstrument) SetParameters(params map[string]float32) {
	e.inner.SetParameters(params)
}

func (e *eqInstrument) SetPedal(down bool) {
	if p, ok := e.inner.(instruments.PedalVoicer); ok {
		p.SetPedal(down)
	}
}

func setPedal(inst instruments.Instrument, down bool) {
	if p, ok := inst.(instruments.PedalVoicer); ok {
		p.SetPedal(down)
	}
}

func (e *Engine) Score() *score.Score { return e.score }

func (e *Engine) SampleRate() float64 { return e.sampleRate }

func (e *Engine) BlockSize() int { return e.blockSize }

func (e *Engine) Mixer() *mixer.Mixer { return e.mix }

func (e *Engine) Err() error { return e.mix.Err() }

func (e *Engine) SetNoiseGateDB(db float64) {
	for _, inst := range e.insts {
		if s, ok := inst.(*sampler.Sampler); ok {
			s.SetNoiseGateDB(db)
		}
		if eq, ok := inst.(*eqInstrument); ok {
			if s, ok := eq.inner.(*sampler.Sampler); ok {
				s.SetNoiseGateDB(db)
			}
		}
	}
}

func (e *Engine) Duration() float64 { return float64(e.totalFrames) / e.sampleRate }

func (e *Engine) Position() float64 { return float64(e.frame) / e.sampleRate }

func (e *Engine) PositionFrames() int { return e.frame }

func (e *Engine) TotalFrames() int { return e.totalFrames }

func (e *Engine) TrackCount() int { return len(e.insts) }

func (e *Engine) Peak() (float32, float32) { return e.peakL, e.peakR }

func (e *Engine) TrackPeak(i int) float32 {
	if i < 0 || i >= len(e.trackPeak) {
		return 0
	}
	return e.trackPeak[i]
}

func (e *Engine) ProcessBlock() {
	e.ProcessFrames(e.blockSize, nil)
}

func (e *Engine) ProcessFrames(n int, sink func(master []float32)) {
	e.peakL, e.peakR = 0, 0
	for i := range e.trackPeak {
		e.trackPeak[i] = 0
	}
	rendered := 0
	for rendered < n {

		for e.nextEvent < len(e.events) && e.events[e.nextEvent].frame <= e.frame {
			ev := e.events[e.nextEvent]
			if ev.isTurntable {
				e.applyTurntableTrigger(ev.track, ev.ttTrigger)
			} else if ev.isCC {
				e.ApplyCC(ev.track, ev.ccNum, ev.ccVal)
			} else if ev.isPedal {
				setPedal(e.insts[ev.track], ev.pedalDown)
			} else if ev.on {
				if ex, ok := e.insts[ev.track].(instruments.ParamsVoicer); ok {
					ex.NoteOnParams(ev.pitch, ev.vel, ev.params)
				} else {
					e.insts[ev.track].NoteOn(ev.pitch, ev.vel)
				}
			} else {
				e.insts[ev.track].NoteOff(ev.pitch)
			}
			e.nextEvent++
		}
		chunk := n - rendered
		if chunk > e.blockSize {
			chunk = e.blockSize
		}
		if e.hasAutomation && chunk > 64 {
			chunk = 64
		}
		if e.nextEvent < len(e.events) {
			if gap := e.events[e.nextEvent].frame - e.frame; gap < chunk {
				chunk = gap
			}
		}
		if chunk < 1 {
			chunk = 1
		}
		if e.hasAutomation {
			e.applyAutomation()
		}
		e.mix.Process(chunk)
		e.updateMeters()
		if sink != nil {
			sink(e.mix.Master())
		}
		e.frame += chunk
		rendered += chunk
	}
}

func (e *Engine) updateMeters() {
	master := e.mix.Master()
	for i := 0; i+1 < len(master); i += 2 {
		if a := abs32(master[i]); a > e.peakL {
			e.peakL = a
		}
		if a := abs32(master[i+1]); a > e.peakR {
			e.peakR = a
		}
	}
	for i := range e.trackPeak {
		if p := e.mix.TrackPeak(i); p > e.trackPeak[i] {
			e.trackPeak[i] = p
		}
	}
}

func (e *Engine) applyAutomation() {
	t := float64(e.frame) / e.sampleRate
	for i, am := range e.trackAuto {
		if am == nil || len(am) == 0 {
			continue
		}
		clear(e.autoParams)
		hasInstParams := false
		for param, track := range am {
			val := float32(track.Interpolate(t))
			switch param {
			case "volume":
				e.mix.SetVolume(i, val)
			case "pan":
				e.mix.SetPan(i, val)
			case "reverb_send":
				e.mix.SetSend(i, val)
			case "distortion_drive":
				if dist := e.mix.TrackDistortion(i); dist != nil {
					dist.SetDrive(val)
				}
				e.autoParams[param] = val
				hasInstParams = true
			case "distortion_mix":
				if dist := e.mix.TrackDistortion(i); dist != nil {
					dist.SetMix(val)
				}
				e.autoParams[param] = val
				hasInstParams = true
			case "ott_depth":
				if ott := e.mix.TrackOTT(i); ott != nil {
					ott.SetDepth(val)
				}
			case "turntable_speed", "scratch_speed":
				e.mix.SetTrackScratch(i, val)
			case "turntable_brake", "tape_stop":
				if val > 0 {
					e.mix.SetTrackBrake(i, float64(val*1000.0), "linear")
				}
			case "beat_roll", "stutter":
				if val > 0 {
					sub := dsp.SubdivisionQuarter
					if val >= 32 {
						sub = dsp.SubdivisionThirtySecond
					} else if val >= 16 {
						sub = dsp.SubdivisionSixteenth
					} else if val >= 8 {
						sub = dsp.SubdivisionEighth
					}
					bpm := 120.0
					if e.score != nil && e.score.Metadata.BPM > 0 {
						bpm = e.score.Metadata.BPM
					}
					e.mix.SetTrackBeatRoll(i, true, sub, bpm)
				} else {
					e.mix.SetTrackBeatRoll(i, false, dsp.SubdivisionQuarter, 120.0)
				}
			case "backspin":
				if val != 0 {
					sp := val
					if sp > 0 {
						sp = -sp
					}
					e.mix.SetTrackBackspin(i, sp, 500.0)
				}
			default:
				e.autoParams[param] = val
				hasInstParams = true
			}
		}
		if hasInstParams && i < len(e.insts) && e.insts[i] != nil {
			e.insts[i].SetParameters(e.autoParams)
		}
	}

	if len(e.masterAuto) > 0 {
		for param, track := range e.masterAuto {
			val := float32(track.Interpolate(t))
			switch param {
			case "turntable_speed", "scratch_speed":
				e.mix.SetMasterScratch(val)
			case "turntable_brake", "tape_stop":
				if val > 0 {
					e.mix.SetMasterBrake(float64(val*1000.0), "linear")
				}
			case "beat_roll", "stutter":
				if val > 0 {
					sub := dsp.SubdivisionQuarter
					if val >= 32 {
						sub = dsp.SubdivisionThirtySecond
					} else if val >= 16 {
						sub = dsp.SubdivisionSixteenth
					} else if val >= 8 {
						sub = dsp.SubdivisionEighth
					}
					bpm := 120.0
					if e.score != nil && e.score.Metadata.BPM > 0 {
						bpm = e.score.Metadata.BPM
					}
					e.mix.SetMasterBeatRoll(true, sub, bpm)
				} else {
					e.mix.SetMasterBeatRoll(false, dsp.SubdivisionQuarter, 120.0)
				}
			case "backspin":
				if val != 0 {
					sp := val
					if sp > 0 {
						sp = -sp
					}
					e.mix.SetMasterBackspin(sp, 500.0)
				}
			}
		}
	}
}

func isTimedTurntableType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "beat_roll", "stutter", "scratch":
		return true
	}
	return false
}

func (e *Engine) applyTurntableTrigger(track int, tr score.TurntableTrigger) {
	typ := strings.ToLower(strings.TrimSpace(tr.Type))
	switch typ {
	case "brake", "tape_stop":
		bTime := tr.BrakeTimeMs
		if bTime <= 0 && tr.Duration > 0 {
			bTime = tr.Duration * 1000.0
		}
		if bTime <= 0 {
			bTime = 200.0
		}
		curve := tr.Curve
		if curve == "" {
			curve = "linear"
		}
		if track < 0 {
			e.mix.SetMasterBrake(bTime, curve)
		} else {
			e.mix.SetTrackBrake(track, bTime, curve)
		}
	case "scratch":
		if track < 0 {
			e.mix.SetMasterScratch(tr.Speed)
		} else {
			e.mix.SetTrackScratch(track, tr.Speed)
		}
	case "beat_roll", "stutter":
		sub, ok := dsp.ParseTurntableSubdivision(tr.Subdivision)
		if !ok {
			sub = dsp.SubdivisionQuarter
		}
		bpm := 120.0
		if e.score != nil && e.score.Metadata.BPM > 0 {
			bpm = e.score.Metadata.BPM
		}
		if track < 0 {
			e.mix.SetMasterBeatRoll(true, sub, bpm)
		} else {
			e.mix.SetTrackBeatRoll(track, true, sub, bpm)
		}
	case "backspin":
		sp := tr.Speed
		if sp == 0 {
			sp = -3.0
		}
		decay := tr.DecayMs
		if decay <= 0 && tr.Duration > 0 {
			decay = tr.Duration * 1000.0
		}
		if decay <= 0 {
			decay = 500.0
		}
		if track < 0 {
			e.mix.SetMasterBackspin(sp, decay)
		} else {
			e.mix.SetTrackBackspin(track, sp, decay)
		}
	case "release", "resume", "start", "bypass":
		sTime := tr.Duration * 1000.0
		curve := tr.Curve
		if track < 0 {
			if typ == "bypass" || typ == "resume" || sTime <= 0 {
				e.mix.SetMasterTurntableBypass()
			} else if tt := e.mix.Turntable(); tt != nil {
				tt.ReleaseBrake(sTime, curve)
			}
		} else {
			if typ == "bypass" || typ == "resume" || sTime <= 0 {
				e.mix.SetTrackTurntableBypass(track)
			} else if tt := e.mix.TrackTurntable(track); tt != nil {
				tt.ReleaseBrake(sTime, curve)
			}
		}
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (e *Engine) ApplyCC(track int, cc int, val int) {
	if track < 0 || track >= len(e.insts) {
		return
	}
	if val < 0 {
		val = 0
	} else if val > 127 {
		val = 127
	}
	inst := e.insts[track]
	switch cc {
	case 64:
		setPedal(inst, val >= 64)
	case 1:
		clear(e.ccParams)
		e.ccParams["lfo_pitch_depth"] = float32(val) / 127.0 * 2.0
		e.ccParams["vibrato_depth"] = float32(val) / 127.0 * 0.1
		inst.SetParameters(e.ccParams)
	case 74:
		clear(e.ccParams)
		e.ccParams["cutoff"] = float32(20.0 + (float64(val)/127.0)*(20000.0-20.0))
		e.ccParams["brightness"] = float32(val) / 127.0
		e.ccParams["pluck_brightness"] = float32(200.0 + (float64(val)/127.0)*(20000.0-200.0))
		inst.SetParameters(e.ccParams)
	case 71:
		clear(e.ccParams)
		e.ccParams["resonance"] = float32(val) / 127.0
		inst.SetParameters(e.ccParams)
	case 11:
		expr := float32(val) / 127.0
		if e.score != nil && track < len(e.score.Tracks) {
			e.mix.SetVolume(track, e.score.Tracks[track].Volume*expr)
		}
		clear(e.ccParams)
		e.ccParams["expression"] = expr
		inst.SetParameters(e.ccParams)
	}
}

func (e *Engine) SetCC(track int, cc int, val int) {
	e.ApplyCC(track, cc, val)
}
