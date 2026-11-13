package sampler

import (
	"math"
	"strings"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
)

var _ instruments.Instrument = (*Sampler)(nil)

var _ instruments.ParamsVoicer = (*Sampler)(nil)

const MaxVoices = 16

const defaultRandSeed uint64 = 0x243F6A8885A308D3

const (
	defaultAttack  = 0.005
	defaultDecay   = 1e-6
	defaultSustain = 1.0
	defaultRelease = 0.20

	mixScratchFrames = 2048
)

const loopCrossfadeSamples = 384

const stealFadeSamples = 240
const stealFadeStep = float32(1.0 / stealFadeSamples)

const autoFadeSamples = 480
const autoFadeStep = float32(1.0 / autoFadeSamples)

const maxPitchVoices = 8

const (
	vibDelay = 0.15
	vibRamp  = 0.10
	twoPi    = 2 * math.Pi
)

type Voice struct {
	Region       *Region
	Pitch        int
	Velocity     float32
	Phase        float64
	PlaybackRate float64
	Envelope     *dsp.ADSR
	Active       bool
	ReleaseTime  float64

	releasing  bool
	startTime  float64
	gain       float32
	panL, panR float32
	looping    bool
	loopStart  float64
	loopEnd    float64
	end        float64
	gate       float32
	pedalHeld  bool

	inCrossfade      bool
	crossfadePhase   float64
	crossfadeReadPos float64

	isStealing        bool
	stealFadeProgress float32
	fadeStep          float32
	stealFree         bool
	pendingPitch      int
	pendingVelocity   float32

	pendingRegion     *Region
	pendingRelease    bool
	pendingParams     instruments.NoteParams
	stealSampleRate   float64
	pendingAttack     float64
	pendingDecay      float64
	pendingSustain    float64
	pendingReleaseEnv float64
	pendingStartTime  float64

	crossfadeGain float32

	hasVibrato bool
	vibRate    float64
	vibDepthM  float64
	vibPhase   float64
	vibTimer   float64
	exprGain   float32

	filter    *dsp.CascadeFilter
	filterR   *dsp.CascadeFilter
	hasFilter bool
	eq        [3]*dsp.Biquad
	eqR       [3]*dsp.Biquad
	hasEQ     [3]bool

	ptrack       *[128]pitchTracker
	trackedPitch int
}

type pitchTracker struct {
	voices [maxPitchVoices]*Voice
	n      int
}

func (v *Voice) Process(buffer []float32, deltaTime float64) {
	v.ProcessStereo(buffer, nil, deltaTime)
}

func (v *Voice) applyFilters(bufL, bufR []float32) {
	if len(bufL) == 0 {
		return
	}
	if v.hasFilter && v.filter != nil {
		v.filter.ProcessInPlace(bufL)
		if bufR != nil && v.filterR != nil {
			v.filterR.ProcessInPlace(bufR)
		}
	}
	for b := 0; b < 3; b++ {
		if v.hasEQ[b] && v.eq[b] != nil {
			v.eq[b].ProcessInPlace(bufL)
			if bufR != nil && v.eqR[b] != nil {
				v.eqR[b].ProcessInPlace(bufR)
			}
		}
	}
}

func (v *Voice) ProcessStereo(bufL, bufR []float32, deltaTime float64) {
	if !v.Active || v.Region == nil || v.Region.Sample == nil || len(bufL) == 0 || deltaTime <= 0 {
		return
	}
	samples := v.Region.Sample.Samples
	samplesR := v.Region.Sample.SamplesR
	isStereo := len(samplesR) > 0 && len(samplesR) >= len(samples)
	hasBufR := bufR != nil && len(bufR) >= len(bufL)
	end := v.end
	if end <= 0 || end > float64(len(samples)) {
		end = float64(len(samples))
	}
	dt := deltaTime / float64(len(bufL))
	const xfLen = float64(loopCrossfadeSamples)
	startIdx := 0
	i := 0
	for ; i < len(bufL); i++ {
		if v.isStealing && v.stealFadeProgress >= 1 {
			if i > startIdx {
				var rSlice []float32
				if hasBufR {
					rSlice = bufR[startIdx:i]
				}
				v.applyFilters(bufL[startIdx:i], rSlice)
				startIdx = i
			}
			v.finishSteal()
			if !v.Active || v.Region == nil || v.Region.Sample == nil {
				break
			}
			samples = v.Region.Sample.Samples
			samplesR = v.Region.Sample.SamplesR
			isStereo = len(samplesR) > 0 && len(samplesR) >= len(samples)
			end = v.end
			if end <= 0 || end > float64(len(samples)) {
				end = float64(len(samples))
			}
		}
		index := int(v.Phase)
		if index >= len(samples) {
			if v.isStealing && v.pendingRegion != nil {
				v.stealFadeProgress = 1
				continue
			}
			v.Active = false
			v.trackUnregister()
			break
		}
		var sample, sampleR float64
		loopLen := v.loopEnd - v.loopStart
		useXF := v.looping && loopLen >= xfLen && v.loopEnd <= float64(len(samples)) && v.loopStart >= 0
		if useXF {
			xfStart := v.loopEnd - xfLen
			if !v.inCrossfade && v.Phase >= xfStart && v.Phase < v.loopEnd {
				v.inCrossfade = true
				v.crossfadeReadPos = v.loopStart + (v.Phase - xfStart)
			}
			if v.inCrossfade {
				t := (v.Phase - xfStart) / xfLen
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				}
				v.crossfadePhase = t
				lo := int(v.loopStart)
				hi := int(v.loopEnd)
				out := interpSampleBounded(samples, v.Phase, lo, hi)
				in := interpSampleBounded(samples, v.crossfadeReadPos, lo, hi)
				outGain := math.Cos(t * math.Pi / 2)
				inGain := math.Sin(t * math.Pi / 2)
				sample = out*outGain + in*inGain
				if isStereo {
					outR := interpSampleBounded(samplesR, v.Phase, lo, hi)
					inR := interpSampleBounded(samplesR, v.crossfadeReadPos, lo, hi)
					sampleR = outR*outGain + inR*inGain
				} else {
					sampleR = sample
				}
			} else {
				sample = interpSample(samples, v.Phase)
				if isStereo {
					sampleR = interpSample(samplesR, v.Phase)
				} else {
					sampleR = sample
				}
			}
		} else {
			v.inCrossfade = false
			sample = interpSample(samples, v.Phase)
			if isStereo {
				sampleR = interpSample(samplesR, v.Phase)
			} else {
				sampleR = sample
			}
		}

		env := v.Envelope.Next(dt)
		scale := env * float64(v.Velocity) * float64(v.exprGain) * float64(v.crossfadeGain)
		out := sample * scale
		outR := sampleR * scale

		if v.isStealing {
			fadeGain := 1 - float64(v.stealFadeProgress)
			if fadeGain < 0 {
				fadeGain = 0
			}
			out *= fadeGain
			outR *= fadeGain
			step := v.fadeStep
			if step <= 0 {
				step = stealFadeStep
			}
			v.stealFadeProgress += step
			if v.stealFadeProgress >= 1 {
				v.stealFadeProgress = 1
			}
		}
		if thr := float64(v.gate); thr > 0 && out < thr && out > -thr {
			out = 0
		}
		if thr := float64(v.gate); thr > 0 && outR < thr && outR > -thr {
			outR = 0
		}
		if hasBufR {
			bufL[i] = float32(out)
			bufR[i] = float32(outR)
		} else {
			if isStereo {
				bufL[i] = float32(0.5 * (out + outR))
			} else {
				bufL[i] = float32(out)
			}
		}

		if v.Envelope.Stage() == dsp.StageIdle {
			if v.isStealing && v.pendingRegion != nil {
				v.stealFadeProgress = 1
				continue
			}
			v.Active = false
			v.trackUnregister()
			i++
			break
		}

		rateEff := v.PlaybackRate
		if v.hasVibrato {
			v.vibPhase += twoPi * v.vibRate * dt
			v.vibTimer += dt
			if v.Envelope.Stage() != dsp.StageAttack && v.vibTimer >= vibDelay {
				ramp := (v.vibTimer - vibDelay) / vibRamp
				if ramp > 1 {
					ramp = 1
				}
				rateEff *= 1 + v.vibDepthM*math.Sin(v.vibPhase)*ramp
			}
		}

		if v.inCrossfade {
			v.Phase += rateEff
			v.crossfadeReadPos += rateEff
			if v.Phase >= v.loopEnd {
				v.Phase = v.crossfadeReadPos
				v.inCrossfade = false
			}
		} else {
			v.Phase += rateEff
			switch {
			case v.looping && v.Phase >= v.loopEnd:
				v.Phase -= loopLen
			case v.Phase >= end:
				if v.isStealing && v.pendingRegion != nil {
					v.stealFadeProgress = 1
					continue
				}
				v.Active = false
				v.trackUnregister()
				i++
			}
		}
		if !v.Active {
			break
		}
	}
	if i > startIdx {
		var rSlice []float32
		if hasBufR {
			rSlice = bufR[startIdx:i]
		}
		v.applyFilters(bufL[startIdx:i], rSlice)
	}
	clear(bufL[i:])
	if hasBufR {
		clear(bufR[i:])
	}
}

func (v *Voice) trackRegister(pitch int) {
	if v.trackedPitch == pitch {
		if v.ptrack == nil || pitch < 0 || pitch > 127 {
			return
		}
		t := &v.ptrack[pitch]
		for k := 0; k < t.n; k++ {
			if t.voices[k] == v {
				return
			}
		}
		if t.n < maxPitchVoices {
			t.voices[t.n] = v
			t.n++
		}
		return
	}
	v.trackUnregister()
	if v.ptrack == nil || pitch < 0 || pitch > 127 {
		return
	}
	t := &v.ptrack[pitch]
	if t.n < maxPitchVoices {
		t.voices[t.n] = v
		t.n++
	}
	v.trackedPitch = pitch
}

func (v *Voice) trackUnregister() {
	if v.ptrack == nil || v.trackedPitch < 0 || v.trackedPitch > 127 {
		v.trackedPitch = -1
		return
	}
	t := &v.ptrack[v.trackedPitch]
	for k := 0; k < t.n; k++ {
		if t.voices[k] == v {
			t.n--
			t.voices[k] = t.voices[t.n]
			t.voices[t.n] = nil
			break
		}
	}
	v.trackedPitch = -1
}

func (v *Voice) finishSteal() {
	if v.stealFree {
		v.Active = false
		v.isStealing = false
		v.stealFree = false
		v.trackUnregister()
		return
	}
	rg := v.pendingRegion
	pitch := v.pendingPitch
	vel := v.pendingVelocity
	sr := v.stealSampleRate
	if sr <= 0 {
		sr = 48000
	}
	wantRelease := v.pendingRelease
	pendingParams := v.pendingParams
	initVoice(v, rg, pitch, vel, sr,
		v.pendingAttack, v.pendingDecay, v.pendingSustain, v.pendingReleaseEnv,
		v.pendingStartTime, pendingParams)
	if wantRelease && rg != nil && rg.LoopMode != LoopOneShot {
		v.Envelope.Release()
		v.releasing = true
	}
}

type Sampler struct {
	sampleRate float64
	sfz        *SFZFile
	voices     [MaxVoices]Voice
	envs       [MaxVoices]dsp.ADSR
	mix        []float32
	mixR       []float32
	clock      float64
	seq        [128]int
	pitchTrack [128]pitchTracker
	trig       []xfTrigger
	rand       uint64

	attack  float64
	decay   float64
	sustain float64
	release float64

	masterGain float32

	gateLin float32

	pedalDown bool

	roundRobin bool
	rrAlt      [128]int
}

func New(sampleRate float64) *Sampler {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	s := &Sampler{
		sampleRate: sampleRate,
		attack:     defaultAttack,
		decay:      defaultDecay,
		sustain:    defaultSustain,
		release:    defaultRelease,
		masterGain: 1,
		rand:       defaultRandSeed,
		mix:        make([]float32, mixScratchFrames),
		mixR:       make([]float32, mixScratchFrames),
	}
	for i := range s.voices {
		v := &s.voices[i]
		v.Envelope = &s.envs[i]
		v.Envelope.SetParameters(s.attack, s.decay, s.sustain, s.release)
		v.ptrack = &s.pitchTrack
		v.trackedPitch = -1
		v.fadeStep = stealFadeStep
		v.filter = dsp.NewCascadeFilter(1, dsp.Lowpass, 20000, sampleRate)
		v.filterR = dsp.NewCascadeFilter(1, dsp.Lowpass, 20000, sampleRate)
		for b := 0; b < 3; b++ {
			v.eq[b] = dsp.NewBiquadGain(dsp.Peaking, 1000, 0.7071, 0, sampleRate)
			v.eqR[b] = dsp.NewBiquadGain(dsp.Peaking, 1000, 0.7071, 0, sampleRate)
		}
	}
	return s
}

func LoadFile(path string, sampleRate float64) (*Sampler, error) {
	f, err := LoadSFZ(path)
	if err != nil {
		return nil, err
	}
	s := New(sampleRate)
	s.Load(f)
	return s, nil
}

func (s *Sampler) Load(f *SFZFile) {
	s.sfz = f
	for i := range s.voices {
		v := &s.voices[i]
		v.Active = false
		v.releasing = false
		v.inCrossfade = false
		v.crossfadePhase = 0
		v.crossfadeReadPos = 0
		v.isStealing = false
		v.stealFadeProgress = 0
		v.fadeStep = stealFadeStep
		v.stealFree = false
		v.pendingRegion = nil
		v.pendingRelease = false
		v.pedalHeld = false
		v.trackedPitch = -1
		v.crossfadeGain = 1
	}
	s.pedalDown = false
	for k := range s.pitchTrack {
		s.pitchTrack[k].n = 0
	}
	s.ResetSequences()
}

func (s *Sampler) ResetSequences() {
	for i := range s.seq {
		s.seq[i] = 0
	}
}

func (s *Sampler) SetRandSeed(seed uint64) {
	if seed == 0 {
		seed = defaultRandSeed
	}
	s.rand = seed
}

func (s *Sampler) nextRand() float32 {
	s.rand = s.rand*6364136223846793005 + 1442695040888963407
	return float32(s.rand>>40) / 16777216
}

func randEligible(rg *Region, roll float32) bool {
	return roll >= rg.LoRand && roll <= rg.HiRand
}

func (s *Sampler) Regions() []Region {
	if s.sfz == nil {
		return nil
	}
	return s.sfz.Regions
}

func (s *Sampler) SetParameters(params map[string]float32) {
	if v, ok := params["master_volume"]; ok {
		s.masterGain = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["attack"]; ok {
		s.attack = float64(dsp.ClampF32(v, 0.0005, 2))
	}
	if v, ok := params["release"]; ok {
		s.release = float64(dsp.ClampF32(v, 0.005, 10))
	}
	if v, ok := params["round_robin"]; ok {
		s.roundRobin = v > 0.5
	}
}

func (s *Sampler) SetRoundRobin(enabled bool) {
	s.roundRobin = enabled
}

func (s *Sampler) RoundRobin() bool {
	return s.roundRobin
}

func (s *Sampler) NoteOn(pitch int, velocity float32) {
	s.NoteOnEx(pitch, velocity, instruments.EnvelopeOverride{})
}

func (s *Sampler) NoteOnEx(pitch int, velocity float32, env instruments.EnvelopeOverride) {
	s.NoteOnParams(pitch, velocity, instruments.NoteParams{Env: env, Expression: 1})
}

func (s *Sampler) NoteOnParams(pitch int, velocity float32, params instruments.NoteParams) {
	if s.sfz == nil {
		return
	}
	if pitch < 0 || pitch > 127 {
		return
	}
	vel := dsp.ClampF32(velocity, 0, 1)
	velInt := int(vel * 127)
	if velInt > 127 {
		velInt = 127
	}
	if cap(s.trig) < len(s.sfz.Regions) {
		s.trig = make([]xfTrigger, 0, len(s.sfz.Regions))
	}
	s.trig = s.trig[:0]
	roll := s.nextRand()
	step := s.seq[pitch]
	advanced := false
	for i := range s.sfz.Regions {
		rg := &s.sfz.Regions[i]
		if !randEligible(rg, roll) || !rg.Matches(pitch, velInt) {
			continue
		}
		if rg.SeqLength <= 1 || seqMatched(s.sfz.Regions, i, pitch, velInt, roll) < 2 {
			if s.roundRobin && rg.SeqLength <= 1 {
				mCount := 0
				for j := range s.sfz.Regions {
					rj := &s.sfz.Regions[j]
					if randEligible(rj, roll) && rj.Matches(pitch, velInt) && rj.SeqLength <= 1 && seqIdentityAlt(rg, rj) {
						mCount++
					}
				}
				if mCount > 1 {
					if seqServedBeforeAlt(s.sfz.Regions, i, pitch, velInt, roll) {
						continue
					}
					wantIdx := s.rrAlt[pitch] % mCount
					curIdx := 0
					for j := range s.sfz.Regions {
						rj := &s.sfz.Regions[j]
						if randEligible(rj, roll) && rj.Matches(pitch, velInt) && rj.SeqLength <= 1 && seqIdentityAlt(rg, rj) {
							if curIdx == wantIdx {
								s.trig = append(s.trig, xfTrigger{rg: rj, gain: xfadeGain(rj, pitch, velInt)})
							}
							curIdx++
						}
					}
					s.rrAlt[pitch]++
					continue
				}
			}
			s.trig = append(s.trig, xfTrigger{rg: rg, gain: xfadeGain(rg, pitch, velInt)})
			continue
		}
		if seqServedBefore(s.sfz.Regions, i, pitch, velInt, roll) {
			continue
		}

		want := step%rg.SeqLength + 1
		for j := range s.sfz.Regions {
			rj := &s.sfz.Regions[j]
			if rj.Sample != nil && rj.SeqPosition == want && randEligible(rj, roll) && seqIdentity(rg, rj) {
				s.trig = append(s.trig, xfTrigger{rg: rj, gain: xfadeGain(rj, pitch, velInt)})
			}
		}
		advanced = true
	}
	if advanced {
		s.seq[pitch] = step + 1
	}
	s.fireTriggers(pitch, vel, params)
}

type xfTrigger struct {
	rg   *Region
	gain float32
}

func xfadeGain(rg *Region, pitch, vel int) float32 {
	fadeIn := func(v, lo, hi int) float32 {
		if lo == hi {
			return 1
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		t := float32(v-lo) / float32(hi-lo)
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		return float32(math.Sin(float64(t) * math.Pi / 2))
	}
	fadeOut := func(v, lo, hi int) float32 {
		if lo == hi {
			return 1
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		t := float32(v-lo) / float32(hi-lo)
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		return float32(math.Cos(float64(t) * math.Pi / 2))
	}
	return fadeIn(vel, rg.XfinLoVel, rg.XfinHiVel) *
		fadeOut(vel, rg.XfoutLoVel, rg.XfoutHiVel) *
		fadeIn(pitch, rg.XfinLoKey, rg.XfinHiKey) *
		fadeOut(pitch, rg.XfoutLoKey, rg.XfoutHiKey)
}

func withXFade(params instruments.NoteParams, gain float32) instruments.NoteParams {
	params.XFadeGain = gain
	params.HasXFade = true
	return params
}

func (s *Sampler) fireTriggers(pitch int, vel float32, params instruments.NoteParams) {
	ts := s.trig
	if len(ts) == 0 {
		return
	}
	if len(ts) == 1 {
		s.trigger(ts[0].rg, pitch, vel, withXFade(params, ts[0].gain))
		return
	}
	for k := range ts {
		if ts[k].rg.OffBy != 0 {
			s.cutOffBy(ts[k].rg.OffBy)
		}
	}
	for k := range ts {
		dup := false
		for j := 0; j < k; j++ {
			if ts[j].rg == ts[k].rg {
				dup = true
				break
			}
		}
		if !dup {
			s.autoFadePitch(ts[k].rg, pitch)
		}
	}

	for a := 1; a < len(ts); a++ {
		for b := a; b > 0 && ts[b].gain > ts[b-1].gain; b-- {
			ts[b], ts[b-1] = ts[b-1], ts[b]
		}
	}
	var free [MaxVoices]int
	nf := 0
	for i := range s.voices {
		if !s.voices[i].Active {
			free[nf] = i
			nf++
		}
	}
	for k := range ts {
		p := withXFade(params, ts[k].gain)
		switch {
		case k < nf:
			s.startVoice(&s.voices[free[k]], ts[k].rg, pitch, vel, p)
		case k == 0:

			s.trigger(ts[k].rg, pitch, vel, p)
		default:

		}
	}
}

func (s *Sampler) trigger(rg *Region, pitch int, vel float32, params instruments.NoteParams) {

	if rg.OffBy != 0 {
		s.cutOffBy(rg.OffBy)
	}

	s.autoFadePitch(rg, pitch)
	v := s.allocVoice()
	if v.Active {
		if !v.isStealing {
			v.isStealing = true
			v.stealFadeProgress = 0
			v.fadeStep = stealFadeStep
			v.releasing = false
			v.startTime = s.clock
		}
		v.stealFree = false
		v.pendingRegion = rg
		v.pendingPitch = pitch
		v.pendingVelocity = vel
		v.pendingRelease = false
		v.pendingParams = params
		v.Pitch = pitch
		v.stealSampleRate = s.sampleRate
		v.pendingAttack = s.attack
		v.pendingDecay = s.decay
		v.pendingSustain = s.sustain
		v.pendingReleaseEnv = s.release
		v.pendingStartTime = s.clock
		return
	}
	s.startVoice(v, rg, pitch, vel, params)
}

func (s *Sampler) fadeVoiceOut(v *Voice) {
	v.isStealing = true
	v.stealFadeProgress = 0
	v.fadeStep = autoFadeStep
	v.stealFree = true
	v.releasing = false
	v.startTime = s.clock
}

func (s *Sampler) pitchCandidates(pitch int) (cands [MaxVoices]*Voice, n int) {
	seen := func(vi *Voice) bool {
		for k := 0; k < n; k++ {
			if cands[k] == vi {
				return true
			}
		}
		return false
	}
	add := func(v *Voice) {
		if n < MaxVoices && !seen(v) {
			cands[n] = v
			n++
		}
	}
	if pitch >= 0 && pitch <= 127 {
		t := &s.pitchTrack[pitch]
		for k := 0; k < t.n; k++ {
			if v := t.voices[k]; v != nil && v.Active && !v.isStealing && v.Pitch == pitch {
				add(v)
			}
		}
	}
	for i := range s.voices {
		if v := &s.voices[i]; v.Active && !v.isStealing && v.Pitch == pitch {
			add(v)
		}
	}
	for a := 1; a < n; a++ {
		for b := a; b > 0 && cands[b].startTime < cands[b-1].startTime; b-- {
			cands[b], cands[b-1] = cands[b-1], cands[b]
		}
	}
	return cands, n
}

func (s *Sampler) autoFadePitch(rg *Region, pitch int) {
	cands, n := s.pitchCandidates(pitch)
	if rg.NotePolyphony > 0 {
		for k := 0; k < n-rg.NotePolyphony+1; k++ {
			s.fadeVoiceOut(cands[k])
		}
		return
	}
	for k := 0; k < n; k++ {
		if cands[k].Region == rg {
			s.fadeVoiceOut(cands[k])
		}
	}
}

func seqIdentity(a, b *Region) bool {
	return a.SeqLength == b.SeqLength &&
		a.LoKey == b.LoKey && a.HiKey == b.HiKey
}

func seqMatched(regions []Region, i, pitch, vel int, roll float32) int {
	n := 0
	for j := range regions {
		if randEligible(&regions[j], roll) && regions[j].Matches(pitch, vel) && seqIdentity(&regions[i], &regions[j]) {
			n++
		}
	}
	return n
}

func seqServedBefore(regions []Region, i, pitch, vel int, roll float32) bool {
	for j := 0; j < i; j++ {
		if randEligible(&regions[j], roll) && regions[j].Matches(pitch, vel) && seqIdentity(&regions[i], &regions[j]) {
			return true
		}
	}
	return false
}

func seqIdentityAlt(a, b *Region) bool {
	return a.LoKey == b.LoKey && a.HiKey == b.HiKey && a.LoVel == b.LoVel && a.HiVel == b.HiVel
}

func seqServedBeforeAlt(regions []Region, i, pitch, vel int, roll float32) bool {
	for j := 0; j < i; j++ {
		if randEligible(&regions[j], roll) && regions[j].Matches(pitch, vel) && regions[j].SeqLength <= 1 && seqIdentityAlt(&regions[i], &regions[j]) {
			return true
		}
	}
	return false
}

func (s *Sampler) cutOffBy(group int) {
	for i := range s.voices {
		v := &s.voices[i]
		if v.Active && !v.releasing && !v.isStealing && v.Region != nil && v.Region.OffBy == group {
			s.fadeVoiceOut(v)
			v.Envelope.Release()
			v.releasing = true
			v.ReleaseTime = s.clock
		}
	}
}

func (s *Sampler) NoteOff(pitch int) {
	for i := range s.voices {
		v := &s.voices[i]
		if !v.Active {
			continue
		}
		if v.isStealing {
			if pitch == v.Pitch {
				v.pendingRelease = true
			}
			continue
		}
		if !v.releasing && v.Pitch == pitch && v.Region.LoopMode != LoopOneShot {
			if s.pedalDown {

				v.pedalHeld = true
				continue
			}
			v.Envelope.Release()
			v.releasing = true
			v.ReleaseTime = s.clock
		}
	}
}

func (s *Sampler) SetPedal(down bool) {
	s.pedalDown = down
	if down {
		return
	}
	for i := range s.voices {
		v := &s.voices[i]
		if v.Active && v.pedalHeld {
			v.pedalHeld = false
			if !v.releasing && v.Region.LoopMode != LoopOneShot {
				v.Envelope.Release()
				v.releasing = true
				v.ReleaseTime = s.clock
			}
		}
	}
}

func (s *Sampler) PedalDown() bool { return s.pedalDown }

func (s *Sampler) Process(buffer *dsp.Buffer, deltaTime float64) {
	if buffer == nil || deltaTime <= 0 {
		return
	}
	n := buffer.Len()
	if n == 0 {
		return
	}
	if len(s.mix) < n {
		s.mix = make([]float32, n)
	}
	left, right := buffer.Left, buffer.Right
	clear(left)
	clear(right)
	blockTime := deltaTime * float64(n)
	if len(s.mixR) < n {
		s.mixR = make([]float32, n)
	}
	for i := range s.voices {
		v := &s.voices[i]
		if !v.Active {
			continue
		}
		g := v.gain * s.masterGain
		gl, gr := g*v.panL, g*v.panR
		v.ProcessStereo(s.mix[:n], s.mixR[:n], blockTime)
		for j := 0; j < n; j++ {
			left[j] += s.mix[j] * gl
			right[j] += s.mixR[j] * gr
		}
	}
	s.clock += blockTime
}

func (s *Sampler) ActiveVoices() int {
	count := 0
	for i := range s.voices {
		if s.voices[i].Active {
			count++
		}
	}
	return count
}

func (s *Sampler) allocVoice() *Voice {
	for i := range s.voices {
		if !s.voices[i].Active {
			return &s.voices[i]
		}
	}
	var oldest *Voice
	for i := range s.voices {
		v := &s.voices[i]
		if v.stealFree && (oldest == nil || v.startTime < oldest.startTime) {
			oldest = v
		}
	}
	if oldest != nil {
		return oldest
	}
	for i := range s.voices {
		v := &s.voices[i]
		if v.releasing && !v.isStealing && (oldest == nil || v.ReleaseTime < oldest.ReleaseTime) {
			oldest = v
		}
	}
	if oldest != nil {
		return oldest
	}
	for i := range s.voices {
		v := &s.voices[i]
		if !v.isStealing && (oldest == nil || v.startTime < oldest.startTime) {
			oldest = v
		}
	}
	if oldest != nil {
		return oldest
	}
	for i := range s.voices {
		v := &s.voices[i]
		if oldest == nil || v.startTime < oldest.startTime {
			oldest = v
		}
	}
	return oldest
}

func (s *Sampler) startVoice(v *Voice, rg *Region, pitch int, velocity float32, params instruments.NoteParams) {
	initVoice(v, rg, pitch, velocity, s.sampleRate,
		s.attack, s.decay, s.sustain, s.release, s.clock, params)
	v.gate = s.gateLin
}

func (s *Sampler) SetNoiseGateDB(db float64) {
	if !(db < 0) {
		s.gateLin = 0
		return
	}
	if db < -120 {
		db = -120
	}
	s.gateLin = float32(math.Pow(10, db/20))
}

func (s *Sampler) NoiseGateThreshold() float32 { return s.gateLin }

func initVoice(v *Voice, rg *Region, pitch int, velocity float32, sampleRate, attack, decay, sustain, release, startTime float64, params instruments.NoteParams) {
	sd := rg.Sample
	v.Region = rg
	v.Pitch = pitch
	v.Velocity = velocity
	v.PlaybackRate = rg.PlaybackRateFor(pitch, sampleRate)

	end := float64(sd.Frames())
	if rg.End > 0 && float64(rg.End) < end {
		end = float64(rg.End)
	}
	v.end = end
	v.Phase = 0
	if rg.Offset > 0 && float64(rg.Offset) < end {
		v.Phase = float64(rg.Offset)
	}

	v.Active = true
	v.releasing = false
	v.pedalHeld = false
	v.startTime = startTime
	v.ReleaseTime = 0
	v.gain = rg.LinearGain()

	pan := clampF(float64(rg.Pan), -1, 1)
	v.panL = float32(math.Sqrt((1 - pan) / 2))
	v.panR = float32(math.Sqrt((1 + pan) / 2))

	v.looping = false
	v.loopStart, v.loopEnd = 0, end
	if rg.LoopMode == LoopContinuous && sd.LoopEnd > sd.LoopStart+1 && sd.LoopEnd <= sd.Frames() {
		v.looping = true
		v.loopStart = float64(sd.LoopStart)
		v.loopEnd = float64(sd.LoopEnd)
		if v.loopEnd > end {
			v.loopEnd = end
			if v.loopStart >= v.loopEnd {
				v.looping = false
			}
		}
	}

	v.crossfadeGain = 1
	if params.HasXFade {
		v.crossfadeGain = params.XFadeGain
	}

	v.inCrossfade = false
	v.crossfadePhase = 0
	v.crossfadeReadPos = 0
	v.isStealing = false
	v.stealFadeProgress = 0
	v.fadeStep = stealFadeStep
	v.stealFree = false
	v.pendingPitch = 0
	v.pendingVelocity = 0
	v.pendingRegion = nil
	v.pendingRelease = false
	v.pendingParams = instruments.NoteParams{}
	v.stealSampleRate = sampleRate
	v.pendingAttack = attack
	v.pendingDecay = decay
	v.pendingSustain = sustain
	v.pendingReleaseEnv = release
	v.pendingStartTime = startTime

	env := params.Env
	v.hasVibrato = params.HasVibrato && params.VibratoRate > 0 && params.VibratoDepth > 0
	v.vibRate = params.VibratoRate
	v.vibDepthM = 0
	if v.hasVibrato {
		v.vibDepthM = math.Pow(2, params.VibratoDepth/12) - 1
	}
	v.vibPhase = 0
	v.vibTimer = 0

	expr := 1.0
	if params.HasExpression {
		expr = float64(params.Expression)
		if math.IsNaN(expr) {
			expr = 1
		} else if expr < 0 {
			expr = 0
		} else if expr > 1 {
			expr = 1
		}
	}
	v.exprGain = float32(expr)

	if rg.HasAmpEG {
		eg := rg.AmpEG
		attack = float64(eg.Attack)
		decay = float64(eg.Decay)
		sustain = clampF(float64(eg.Sustain)/100, 0, 1)
		release = float64(eg.Release)
	}

	if env.HasAttack && env.Attack > 0 {
		attack = env.Attack
	}
	if env.HasDecay && env.Decay > 0 {
		decay = env.Decay
	}
	if env.HasRelease && env.Release > 0 {
		release = env.Release
	}
	if v.Envelope == nil {
		v.Envelope = &dsp.ADSR{}
	}
	v.Envelope.SetParameters(attack, decay, sustain, release)
	v.Envelope.Trigger()
	v.trackRegister(pitch)

	if v.filter == nil {
		v.filter = dsp.NewCascadeFilter(1, dsp.Lowpass, 20000, sampleRate)
		v.filterR = dsp.NewCascadeFilter(1, dsp.Lowpass, 20000, sampleRate)
		for b := 0; b < 3; b++ {
			v.eq[b] = dsp.NewBiquadGain(dsp.Peaking, 1000, 0.7071, 0, sampleRate)
			v.eqR[b] = dsp.NewBiquadGain(dsp.Peaking, 1000, 0.7071, 0, sampleRate)
		}
	}
	v.hasFilter = false
	if rg != nil && rg.Cutoff > 0 {
		cutoff := float64(rg.Cutoff)
		if rg.FilKeytrack != 0 {
			center := 60
			if rg.FilKeycenter > 0 {
				center = rg.FilKeycenter
			}
			cutoff *= math.Pow(2, float64(rg.FilKeytrack)*float64(pitch-center)/1200)
		}
		if rg.FilVeltrack != 0 {
			cutoff *= math.Pow(2, float64(rg.FilVeltrack)*float64(velocity-1)/1200)
		}
		if cutoff < 20 {
			cutoff = 20
		}
		nyquist := sampleRate / 2
		if cutoff >= nyquist {
			cutoff = nyquist - 1
		}
		mode := dsp.Lowpass
		stages := 1
		filType := strings.ToLower(strings.TrimSpace(rg.FilType))
		switch filType {
		case "hpf_1p":
			mode = dsp.Highpass1P
			stages = 1
		case "hpf_4p":
			mode = dsp.Highpass
			stages = 2
		case "hpf_6p":
			mode = dsp.Highpass
			stages = 3
		case "hpf_2p", "hpf":
			mode = dsp.Highpass
			stages = 1
		case "lpf_1p":
			mode = dsp.Lowpass1P
			stages = 1
		case "lpf_4p":
			mode = dsp.Lowpass
			stages = 2
		case "lpf_6p":
			mode = dsp.Lowpass
			stages = 3
		case "lpf_2p", "lpf":
			mode = dsp.Lowpass
			stages = 1
		case "bpf_2p", "bpf_1p", "bpf":
			mode = dsp.Bandpass
			stages = 1
		case "brf_2p", "brf_1p", "brf", "notch_2p", "notch_1p", "notch":
			mode = dsp.Notch
			stages = 1
		default:
			if strings.Contains(filType, "1p") {
				stages = 1
				if strings.HasPrefix(filType, "h") {
					mode = dsp.Highpass1P
				} else {
					mode = dsp.Lowpass1P
				}
			} else {
				if strings.Contains(filType, "4p") {
					stages = 2
				} else if strings.Contains(filType, "6p") {
					stages = 3
				} else {
					stages = 1
				}
				if strings.HasPrefix(filType, "h") {
					mode = dsp.Highpass
				} else if strings.HasPrefix(filType, "brf") || strings.HasPrefix(filType, "n") {
					mode = dsp.Notch
				} else if strings.HasPrefix(filType, "b") {
					mode = dsp.Bandpass
				} else {
					mode = dsp.Lowpass
				}
			}
		}
		v.filter.SetButterworth(stages, mode, cutoff, float64(rg.Resonance), sampleRate)
		v.filter.Reset()
		v.filterR.SetButterworth(stages, mode, cutoff, float64(rg.Resonance), sampleRate)
		v.filterR.Reset()
		v.hasFilter = true
	}
	for b := 0; b < 3; b++ {
		v.hasEQ[b] = false
		if rg != nil && rg.EQ[b].Freq > 0 {
			freq := float64(rg.EQ[b].Freq)
			gain := float64(rg.EQ[b].Gain)
			if rg.EQ[b].Veltrack != 0 {
				gain += float64(rg.EQ[b].Veltrack) * float64(velocity-1)
			}
			if gain == 0 {
				continue
			}
			bw := float64(rg.EQ[b].BW)
			if bw <= 0 {
				bw = 1.0
			}
			q := 1.0 / bw
			v.eq[b].SetSampleRate(sampleRate)
			v.eq[b].SetMode(dsp.Peaking)
			v.eq[b].SetFrequency(freq)
			v.eqR[b].SetSampleRate(sampleRate)
			v.eqR[b].SetMode(dsp.Peaking)
			v.eqR[b].SetFrequency(freq)
			v.eq[b].SetGainDB(gain)
			v.eq[b].SetQ(q)
			v.eq[b].Reset()
			v.eqR[b].SetGainDB(gain)
			v.eqR[b].SetQ(q)
			v.eqR[b].Reset()
			v.hasEQ[b] = true
		}
	}
}
