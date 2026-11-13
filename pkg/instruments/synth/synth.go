package synth

import (
	"math"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
)

var _ instruments.Instrument = (*Synth)(nil)

var _ instruments.ParamsVoicer = (*Synth)(nil)

var _ instruments.EnvelopeVoicer = (*Synth)(nil)

var _ instruments.PedalVoicer = (*Synth)(nil)

const MaxVoices = 16

const (
	defaultWaveform  = WaveSaw
	defaultCutoff    = 4000.0
	defaultResonance = 0.2
	defaultAttack    = 0.005
	defaultDecay     = 0.1
	defaultSustain   = 0.8
	defaultRelease   = 0.2
	defaultDetune    = 0.0

	minCutoff = 20.0
	maxCutoff = 20000.0
	maxRes    = 0.8
)

type voice struct {
	active       bool
	pitch        int
	velocity     float32
	freq         float64
	phase1       float64
	phase2       float64
	filtY        float64
	filtPrev     float64
	svfS1        float64
	svfS2        float64
	svfS3        float64
	svfS4        float64
	voiceIndex   int
	panL         float64
	panR         float64
	unisonPhase1 [7]float64
	unisonPhase2 [7]float64
	filtYR       float64
	filtPrevR    float64
	svfS1R       float64
	svfS2R       float64
	svfS3R       float64
	svfS4R       float64
	lfo          voiceLFO
	hasNoteVib   bool
	noteVibRate  float64
	noteVibDepth float64
	noiseState1  uint64
	noiseState2  uint64
	pink1        [7]float64
	pink2        [7]float64
	env          *dsp.Envelope
	filtEnv      dsp.Envelope
	pedalHeld    bool
	fmEnv        dsp.Envelope
	fmPhaseC     float64
	fmPhaseM     float64
	fmModPrev    float64
	pluckBuf     [2400]float32
	pluckWrite   int
	pluckPrev    float64
	start        float64
	expr         float32
	startFreq    float64
	targetFreq   float64
	glideTime    float64
	glideElapsed float64
}

type Synth struct {
	sampleRate float64
	voices     [MaxVoices]voice
	envs       [MaxVoices]dsp.Envelope
	clock      float64

	wave1, wave2      Waveform
	cutoff, resonance float32
	attack, decay     float64
	sustain, release  float64
	detune            float32
	filterAlpha       float64
	filterRes         float64

	filterType        FilterType
	filterAttack      float64
	filterDecay       float64
	filterSustain     float64
	filterRelease     float64
	filterEnvDepth    float32
	pulseWidth1       float64
	pulseWidth2       float64
	panSpread         float32
	unisonVoices      int
	unisonDetune      float32
	unisonSpread      float32
	lfoWaveform       LFOWaveform
	lfoRate           float32
	lfoDelay          float32
	lfoPitchDepth     float32
	lfoFilterDepth    float32
	lfoAmpDepth       float32
	pedalDown         bool
	synthMode         int
	fmRatio           float64
	fmDepth           float64
	fmFeedback        float64
	fmAttack          float64
	fmDecay           float64
	fmSustain         float64
	fmRelease         float64
	pluckDamping      float64
	pluckDecay        float64
	pluckBrightness   float64
	pluckPickPosition float64
	portamento        float64
	lastFreq          float64
	velocityToFilter  float32
	velocityToAttack  float32
	wavetableID       int
	wavetablePos      float32
	wavetableEnvDepth float32
	wavetableLFODepth float32
	pitchBend         float64
	distortion        *dsp.Distortion
	useDistortion     bool
}

func New(sampleRate float64) *Synth {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	s := &Synth{sampleRate: sampleRate}
	s.wave1, s.wave2 = defaultWaveform, defaultWaveform
	s.cutoff, s.resonance = defaultCutoff, defaultResonance
	s.attack, s.decay = defaultAttack, defaultDecay
	s.sustain, s.release = defaultSustain, defaultRelease
	s.detune = defaultDetune
	s.filterType = Filter1Pole
	s.filterAttack = defaultAttack
	s.filterDecay = defaultDecay
	s.filterSustain = defaultSustain
	s.filterRelease = defaultRelease
	s.filterEnvDepth = 0.0
	s.pulseWidth1 = 0.5
	s.pulseWidth2 = 0.5
	s.panSpread = 0.0
	s.unisonVoices = 1
	s.unisonDetune = 0.0
	s.unisonSpread = 0.0
	s.lfoWaveform = LFOSine
	s.lfoRate = 5.0
	s.lfoDelay = 0.0
	s.lfoPitchDepth = 0.0
	s.lfoFilterDepth = 0.0
	s.lfoAmpDepth = 0.0
	s.synthMode = 0
	s.pedalDown = false
	s.fmRatio = 1.0
	s.fmDepth = 0.0
	s.fmFeedback = 0.0
	s.fmAttack = defaultAttack
	s.fmDecay = defaultDecay
	s.fmSustain = defaultSustain
	s.fmRelease = defaultRelease
	s.pluckDamping = 0.5
	s.pluckDecay = 0.98
	s.pluckBrightness = 10000.0
	s.pluckPickPosition = 0.2
	s.wavetableID = 0
	s.wavetablePos = 0.0
	s.wavetableEnvDepth = 0.0
	s.wavetableLFODepth = 0.0
	s.pitchBend = 0.0
	s.distortion = dsp.NewDistortion(sampleRate, dsp.DefaultDistortionConfig())
	s.distortion.SetMix(0.0)
	s.useDistortion = false
	s.retuneFilter()
	for i := range s.voices {
		s.voices[i].voiceIndex = i
		s.voices[i].panL = 1.0
		s.voices[i].panR = 1.0
		s.voices[i].env = &s.envs[i]
		s.voices[i].env.SetParameters(s.attack, s.decay, s.sustain, s.release)
		s.voices[i].filtEnv.SetParameters(s.filterAttack, s.filterDecay, s.filterSustain, s.filterRelease)
		s.voices[i].fmEnv.SetParameters(s.fmAttack, s.fmDecay, s.fmSustain, s.fmRelease)
	}
	return s
}

func (s *Synth) retuneFilter() {
	fc := float64(s.cutoff)
	if fc < minCutoff {
		fc = minCutoff
	}
	if fc > maxCutoff {
		fc = maxCutoff
	}
	s.filterAlpha = 1 - math.Exp(-2*math.Pi*fc/s.sampleRate)
	if s.filterAlpha > 1 {
		s.filterAlpha = 1
	}
	r := float64(s.resonance)
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	s.filterRes = r * maxRes
}

func (s *Synth) SetParameters(params map[string]float32) {
	if v, ok := params["waveform1"]; ok {
		s.wave1 = waveParam(v)
	}
	if v, ok := params["waveform2"]; ok {
		s.wave2 = waveParam(v)
	}
	if v, ok := params["cutoff"]; ok {
		s.cutoff = dsp.ClampF32(v, minCutoff, maxCutoff)
	}
	if v, ok := params["resonance"]; ok {
		s.resonance = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["attack"]; ok {
		s.attack = float64(dsp.ClampF32(v, 0.0005, 2))
	}
	if v, ok := params["decay"]; ok {
		s.decay = float64(dsp.ClampF32(v, 0.001, 2))
	}
	if v, ok := params["sustain"]; ok {
		s.sustain = float64(dsp.ClampF32(v, 0, 1))
	}
	if v, ok := params["release"]; ok {
		s.release = float64(dsp.ClampF32(v, 0.005, 10))
	}
	if v, ok := params["detune"]; ok {
		s.detune = dsp.ClampF32(v, -12, 12)
	}
	if v, ok := params["filter_type"]; ok {
		s.filterType = filterParam(v)
	}
	if v, ok := params["filter_attack"]; ok {
		s.filterAttack = float64(dsp.ClampF32(v, 0.0005, 10))
	}
	if v, ok := params["filter_decay"]; ok {
		s.filterDecay = float64(dsp.ClampF32(v, 0.001, 10))
	}
	if v, ok := params["filter_sustain"]; ok {
		s.filterSustain = float64(dsp.ClampF32(v, 0, 1))
	}
	if v, ok := params["filter_release"]; ok {
		s.filterRelease = float64(dsp.ClampF32(v, 0.005, 10))
	}
	if v, ok := params["filter_env_depth"]; ok {
		s.filterEnvDepth = dsp.ClampF32(v, -10, 10)
	}
	if v, ok := params["pulse_width"]; ok {
		pw := float64(dsp.ClampF32(v, 0.05, 0.95))
		s.pulseWidth1 = pw
		s.pulseWidth2 = pw
	}
	if v, ok := params["pwm"]; ok {
		pw := float64(dsp.ClampF32(v, 0.05, 0.95))
		s.pulseWidth1 = pw
		s.pulseWidth2 = pw
	}
	if v, ok := params["pw"]; ok {
		pw := float64(dsp.ClampF32(v, 0.05, 0.95))
		s.pulseWidth1 = pw
		s.pulseWidth2 = pw
	}
	if v, ok := params["pulse_width1"]; ok {
		s.pulseWidth1 = float64(dsp.ClampF32(v, 0.05, 0.95))
	}
	if v, ok := params["pulse_width2"]; ok {
		s.pulseWidth2 = float64(dsp.ClampF32(v, 0.05, 0.95))
	}
	if v, ok := params["pan_spread"]; ok {
		s.panSpread = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["unison_voices"]; ok {
		s.unisonVoices = int(math.Round(float64(dsp.ClampF32(v, 1, 7))))
	}
	if v, ok := params["unison_detune"]; ok {
		s.unisonDetune = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["unison_spread"]; ok {
		s.unisonSpread = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["lfo_waveform"]; ok {
		s.lfoWaveform = lfoWaveParam(v)
	}
	if v, ok := params["lfo_rate"]; ok {
		s.lfoRate = dsp.ClampF32(v, 0.01, 50)
	}
	if v, ok := params["lfo_delay"]; ok {
		s.lfoDelay = dsp.ClampF32(v, 0, 2)
	}
	if v, ok := params["lfo_pitch_depth"]; ok {
		s.lfoPitchDepth = dsp.ClampF32(v, -24, 24)
	}
	if v, ok := params["lfo_filter_depth"]; ok {
		s.lfoFilterDepth = dsp.ClampF32(v, -10, 10)
	}
	if v, ok := params["lfo_amp_depth"]; ok {
		s.lfoAmpDepth = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["mode"]; ok {
		m := int(math.Round(float64(v)))
		if math.IsNaN(float64(v)) || m < 0 {
			m = 0
		} else if m > 3 {
			m = 3
		}
		s.synthMode = m
	}
	if v, ok := params["synth_mode"]; ok {
		m := int(math.Round(float64(v)))
		if math.IsNaN(float64(v)) || m < 0 {
			m = 0
		} else if m > 3 {
			m = 3
		}
		s.synthMode = m
	}
	if v, ok := params["fm_ratio"]; ok {
		s.fmRatio = float64(dsp.ClampF32(v, 0.1, 32.0))
	}
	if v, ok := params["fm_index"]; ok {
		s.fmDepth = float64(dsp.ClampF32(v, 0.0, 16.0))
	}
	if v, ok := params["fm_depth"]; ok {
		s.fmDepth = float64(dsp.ClampF32(v, 0.0, 16.0))
	}
	if v, ok := params["fm_feedback"]; ok {
		s.fmFeedback = float64(dsp.ClampF32(v, 0.0, 1.0))
	}
	if v, ok := params["fm_attack"]; ok {
		s.fmAttack = float64(dsp.ClampF32(v, 0.0005, 10))
	}
	if v, ok := params["fm_decay"]; ok {
		s.fmDecay = float64(dsp.ClampF32(v, 0.001, 10))
	}
	if v, ok := params["fm_sustain"]; ok {
		s.fmSustain = float64(dsp.ClampF32(v, 0.0, 1.0))
	}
	if v, ok := params["fm_release"]; ok {
		s.fmRelease = float64(dsp.ClampF32(v, 0.005, 10))
	}
	if v, ok := params["pluck_damp"]; ok {
		s.pluckDamping = float64(dsp.ClampF32(v, 0.0, 1.0))
	}
	if v, ok := params["pluck_damping"]; ok {
		s.pluckDamping = float64(dsp.ClampF32(v, 0.0, 1.0))
	}
	if v, ok := params["pluck_decay"]; ok {
		s.pluckDecay = float64(dsp.ClampF32(v, 0.8, 0.9999))
	}
	if v, ok := params["pluck_brightness"]; ok {
		s.pluckBrightness = float64(dsp.ClampF32(v, 200, 20000))
	}
	if v, ok := params["pluck_pick_pos"]; ok {
		s.pluckPickPosition = float64(dsp.ClampF32(v, 0.0, 1.0))
	}
	if v, ok := params["pluck_pick_position"]; ok {
		s.pluckPickPosition = float64(dsp.ClampF32(v, 0.0, 1.0))
	}
	if v, ok := params["portamento"]; ok {
		s.portamento = float64(dsp.ClampF32(v, 0.0, 5.0))
	}
	if v, ok := params["glide"]; ok {
		s.portamento = float64(dsp.ClampF32(v, 0.0, 5.0))
	}
	if v, ok := params["velocity_to_filter"]; ok {
		s.velocityToFilter = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["velocity_to_attack"]; ok {
		s.velocityToAttack = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["wavetable_pos"]; ok {
		s.wavetablePos = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["wavetable_env_depth"]; ok {
		s.wavetableEnvDepth = dsp.ClampF32(v, -1, 1)
	}
	if v, ok := params["wavetable_lfo_depth"]; ok {
		s.wavetableLFODepth = dsp.ClampF32(v, -1, 1)
	}
	if v, ok := params["wavetable"]; ok {
		s.wavetableID = int(math.Round(float64(dsp.ClampF32(v, 0, float32(NumReferenceTables-1)))))
	}
	if v, ok := params["table_name"]; ok {
		s.wavetableID = int(math.Round(float64(dsp.ClampF32(v, 0, float32(NumReferenceTables-1)))))
	}
	if v, ok := params["pitch_bend"]; ok {
		s.pitchBend = float64(v)
	}
	if v, ok := params["distortion_drive"]; ok {
		s.distortion.SetDrive(v)
		if s.distortion.Mix() == 0 {
			s.distortion.SetMix(1.0)
		}
		s.useDistortion = true
	}
	if v, ok := params["distortion_mode"]; ok {
		m := int(math.Round(float64(v)))
		s.distortion.SetMode(dsp.DistortionMode(m))
		s.useDistortion = true
	}
	if v, ok := params["distortion_mix"]; ok {
		s.distortion.SetMix(dsp.ClampF32(v, 0, 1))
		s.useDistortion = s.distortion.Mix() > 0
	}
	if v, ok := params["distortion_tone"]; ok {
		s.distortion.SetTone(dsp.ClampF32(v, 0, 20000))
	}
	s.retuneFilter()
	for i := range s.voices {
		s.voices[i].env.SetParameters(s.attack, s.decay, s.sustain, s.release)
		s.voices[i].filtEnv.SetParameters(s.filterAttack, s.filterDecay, s.filterSustain, s.filterRelease)
		s.voices[i].fmEnv.SetParameters(s.fmAttack, s.fmDecay, s.fmSustain, s.fmRelease)
	}
}

func (s *Synth) SetWavetableByName(name string) {
	s.wavetableID = WavetableIndex(name)
}

func (s *Synth) SetWavetableIndex(index int) {
	if index < 0 {
		index = 0
	} else if index >= NumReferenceTables {
		index = NumReferenceTables - 1
	}
	s.wavetableID = index
}

func (s *Synth) WavetableIndex() int {
	return s.wavetableID
}

func (s *Synth) SetWavetablePos(pos float32) {
	s.wavetablePos = dsp.ClampF32(pos, 0, 1)
}

func (s *Synth) WavetablePos() float32 {
	return s.wavetablePos
}

func (s *Synth) SetPitchBend(semitones float64) {
	s.pitchBend = semitones
}

func (s *Synth) PitchBend() float64 {
	return s.pitchBend
}

func (s *Synth) Distortion() *dsp.Distortion {
	return s.distortion
}

func midiFreq(pitch int) float64 {
	if pitch < 0 {
		pitch = 0
	}
	if pitch > 127 {
		pitch = 127
	}
	return 440 * math.Pow(2, float64(pitch-69)/12)
}

func (s *Synth) NoteOn(pitch int, velocity float32) {
	s.NoteOnParams(pitch, velocity, instruments.NoteParams{})
}

func (s *Synth) NoteOnEx(pitch int, velocity float32, env instruments.EnvelopeOverride) {
	s.NoteOnParams(pitch, velocity, instruments.NoteParams{Env: env})
}

func (s *Synth) NoteOnParams(pitch int, velocity float32, params instruments.NoteParams) {
	v := s.allocVoice()
	vel := dsp.ClampF32(velocity, 0, 1)
	freq := midiFreq(pitch)
	if s.portamento > 0 && s.lastFreq > 0 && s.lastFreq != freq {
		v.startFreq = s.lastFreq
		v.targetFreq = freq
		v.glideTime = s.portamento
		v.glideElapsed = 0
		v.freq = s.lastFreq
	} else {
		v.startFreq = freq
		v.targetFreq = freq
		v.glideTime = 0
		v.glideElapsed = 0
		v.freq = freq
	}
	s.lastFreq = freq
	v.pitch = pitch
	v.velocity = vel
	v.phase1, v.phase2 = 0, 0
	v.unisonPhase1 = [7]float64{}
	v.unisonPhase2 = [7]float64{}
	v.filtY, v.filtPrev = 0, 0
	v.filtYR, v.filtPrevR = 0, 0
	v.svfS1, v.svfS2, v.svfS3, v.svfS4 = 0, 0, 0, 0
	v.svfS1R, v.svfS2R, v.svfS3R, v.svfS4R = 0, 0, 0, 0
	v.pink1 = [7]float64{}
	v.pink2 = [7]float64{}
	v.panL, v.panR = computeVoicePan(s.panSpread, v.voiceIndex)
	v.pedalHeld = false
	v.fmPhaseC, v.fmPhaseM, v.fmModPrev = 0, 0, 0
	seed := dsp.DefaultNoiseSeed ^ (uint64(pitch)*1000003 + uint64(s.clock)*7919 + 1)
	if seed == 0 {
		seed = dsp.DefaultNoiseSeed
	}
	v.noiseState1 = seed
	v.noiseState2 = seed ^ 0x5555555555555555
	v.lfo.trigger(seed ^ 0x3333333333333333)
	v.hasNoteVib = params.HasVibrato && (params.VibratoDepth > 0 || params.VibratoRate > 0)
	v.noteVibRate = params.VibratoRate
	v.noteVibDepth = params.VibratoDepth
	if v.hasNoteVib && v.noteVibRate <= 0 {
		v.noteVibRate = float64(s.lfoRate)
	}
	v.expr = 1
	if params.HasExpression {
		e := float64(params.Expression)
		if math.IsNaN(e) {
			e = 1
		} else if e < 0 {
			e = 0
		} else if e > 1 {
			e = 1
		}
		v.expr = float32(e)
	}
	attack, decay, release := s.attack, s.decay, s.release
	if env := params.Env; env.HasAttack && env.Attack > 0 {
		attack = env.Attack
	}
	if s.velocityToAttack > 0 {
		slope := 1.0 - float64(s.velocityToAttack*vel)*0.85
		if slope < 0.05 {
			slope = 0.05
		}
		attack *= slope
	}
	if env := params.Env; env.HasDecay && env.Decay > 0 {
		decay = env.Decay
	}
	if env := params.Env; env.HasRelease && env.Release > 0 {
		release = env.Release
	}
	v.freq = freq
	if s.synthMode == 2 {
		s.initPluckVoice(v, release)
	} else {
		v.env.SetParameters(attack, decay, s.sustain, release)
		v.env.Trigger()
		v.filtEnv.SetParameters(s.filterAttack, s.filterDecay, s.filterSustain, s.filterRelease)
		v.filtEnv.Trigger()
		v.fmEnv.SetParameters(s.fmAttack, s.fmDecay, s.fmSustain, s.fmRelease)
		v.fmEnv.Trigger()
	}
	v.start = s.clock
	v.active = true
	s.clock++
}

func (s *Synth) NoteOff(pitch int) {
	for i := range s.voices {
		v := &s.voices[i]
		if v.active && v.pitch == pitch {
			if s.pedalDown {
				v.pedalHeld = true
				continue
			}
			v.env.Release()
			v.filtEnv.Release()
			v.fmEnv.Release()
		}
	}
}

func (s *Synth) SetPedal(down bool) {
	s.pedalDown = down
	if down {
		return
	}
	for i := range s.voices {
		v := &s.voices[i]
		if v.active && v.pedalHeld {
			v.pedalHeld = false
			v.env.Release()
			v.filtEnv.Release()
			v.fmEnv.Release()
		}
	}
}

func (s *Synth) PedalDown() bool {
	return s.pedalDown
}

func (s *Synth) ActiveVoices() int {
	n := 0
	for i := range s.voices {
		if s.voices[i].active {
			n++
		}
	}
	return n
}

func (s *Synth) Reset() {
	for i := range s.voices {
		s.voices[i].active = false
		s.voices[i].pitch = 0
		s.voices[i].velocity = 0
	}
}

func (s *Synth) allocVoice() *voice {
	for i := range s.voices {
		if !s.voices[i].active {
			return &s.voices[i]
		}
	}
	oldest := &s.voices[0]
	for i := range s.voices[1:] {
		if s.voices[i+1].start < oldest.start {
			oldest = &s.voices[i+1]
		}
	}
	return oldest
}

func (s *Synth) Process(buffer *dsp.Buffer, deltaTime float64) {
	if buffer == nil || deltaTime <= 0 {
		return
	}
	switch s.synthMode {
	case 1:
		s.processFM(buffer, deltaTime)
		if s.useDistortion && s.distortion != nil {
			s.distortion.ProcessStereo(buffer.Left, buffer.Right)
		}
		return
	case 2:
		s.processPluck(buffer, deltaTime)
		if s.useDistortion && s.distortion != nil {
			s.distortion.ProcessStereo(buffer.Left, buffer.Right)
		}
		return
	case 3:
		s.processWavetable(buffer, deltaTime)
		if s.useDistortion && s.distortion != nil {
			s.distortion.ProcessStereo(buffer.Left, buffer.Right)
		}
		return
	}
	left, right := buffer.Left, buffer.Right
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	detuneRatio := math.Pow(2, float64(s.detune)/12)

	hasFilterEnv := s.filterEnvDepth != 0
	hasFilterLFO := s.lfoFilterDepth != 0
	hasFilterMod := hasFilterEnv || hasFilterLFO || s.velocityToFilter > 0

	var staticSVF svfCoeffs
	var staticLadder ladderCoeffs
	if !hasFilterMod {
		switch s.filterType {
		case Filter12dBSVF, Filter24dBCascade:
			staticSVF = calcSVFCoeffs(float64(s.cutoff), float64(s.resonance), s.sampleRate)
		case Filter24dBLadder:
			staticLadder = calcLadderCoeffs(float64(s.cutoff), float64(s.resonance), s.sampleRate)
		}
	}

	uVoices := s.unisonVoices
	if uVoices < 1 {
		uVoices = 1
	} else if uVoices > 7 {
		uVoices = 7
	}
	uDetune := s.unisonDetune
	uSpread := s.unisonSpread
	uNorm := unisonPowerNorm(uVoices)

	var uDetuneRatios [7]float64
	var uPanL [7]float64
	var uPanR [7]float64
	if uVoices > 1 {
		for k := 0; k < uVoices; k++ {
			uDetuneRatios[k] = unisonDetuneRatio(k, uVoices, uDetune)
			uPanL[k], uPanR[k] = unisonSubPan(k, uVoices, uSpread)
		}
	}

	pbRatio := 1.0
	if s.pitchBend != 0 {
		pbRatio = math.Pow(2.0, s.pitchBend/12.0)
	}

	for i := range s.voices {
		v := &s.voices[i]
		if !v.active {
			continue
		}
		freq := v.freq * pbRatio
		vel := float64(v.velocity) * float64(v.expr)

		hasPitchLFO := s.lfoPitchDepth != 0
		hasPitchVib := v.hasNoteVib
		hasPitchMod := hasPitchLFO || hasPitchVib
		hasAmpLFO := s.lfoAmpDepth != 0
		hasAnyLFO := hasFilterLFO || hasPitchMod || hasAmpLFO

		effectivePitchDepth := float64(s.lfoPitchDepth)
		effectiveLFORate := float64(s.lfoRate)
		if v.hasNoteVib {
			if v.noteVibDepth > 0 {
				effectivePitchDepth += v.noteVibDepth
			}
			if v.noteVibRate > 0 {
				effectiveLFORate = v.noteVibRate
			}
		}

		inc1 := freq * deltaTime
		inc2 := freq * detuneRatio * deltaTime
		isStereoUnison := uVoices > 1 && uSpread > 0

		for j := 0; j < n; j++ {
			if v.glideTime > 0 && v.glideElapsed < v.glideTime {
				v.glideElapsed += deltaTime
				t := v.glideElapsed / v.glideTime
				if t >= 1.0 {
					v.freq = v.targetFreq
					v.glideTime = 0
				} else {
					v.freq = v.startFreq * math.Pow(v.targetFreq/v.startFreq, t)
				}
				inc1 = v.freq * deltaTime
				inc2 = v.freq * detuneRatio * deltaTime
			}

			var lfoVal float64
			if hasAnyLFO {
				lfoVal = v.lfo.step(effectiveLFORate, float64(s.lfoDelay), s.lfoWaveform, deltaTime)
			}

			pitchRatio := 1.0
			if hasPitchMod && effectivePitchDepth != 0 {
				pitchRatio = math.Pow(2, lfoVal*effectivePitchDepth/12.0)
				if math.IsNaN(pitchRatio) || pitchRatio <= 0 {
					pitchRatio = 1.0
				}
			}

			var xL, xR float64

			if uVoices <= 1 {
				curInc1 := inc1
				curInc2 := inc2
				if hasPitchMod && effectivePitchDepth != 0 {
					curInc1 *= pitchRatio
					curInc2 *= pitchRatio
				}

				var o1 float64
				switch s.wave1 {
				case WaveNoise:
					o1 = xorshift64Step(&v.noiseState1)
				case WavePinkNoise:
					o1 = kelletPinkStep(&v.pink1, xorshift64Step(&v.noiseState1))
				default:
					o1 = oscWaveBlepPWM(s.wave1, v.phase1, curInc1, s.pulseWidth1)
				}

				var o2 float64
				switch s.wave2 {
				case WaveNoise:
					o2 = xorshift64Step(&v.noiseState2)
				case WavePinkNoise:
					o2 = kelletPinkStep(&v.pink2, xorshift64Step(&v.noiseState2))
				default:
					o2 = oscWaveBlepPWM(s.wave2, v.phase2, curInc2, s.pulseWidth2)
				}

				x := 0.5 * (o1 + o2)
				xL = x
				xR = x

				v.phase1 += curInc1
				if v.phase1 >= 1 || v.phase1 < 0 {
					v.phase1 -= math.Floor(v.phase1)
				}
				v.phase2 += curInc2
				if v.phase2 >= 1 || v.phase2 < 0 {
					v.phase2 -= math.Floor(v.phase2)
				}
			} else {
				var sum1L, sum1R float64
				var sum2L, sum2R float64

				for k := 0; k < uVoices; k++ {
					curInc1 := inc1 * uDetuneRatios[k]
					curInc2 := inc2 * uDetuneRatios[k]
					if hasPitchMod && effectivePitchDepth != 0 {
						curInc1 *= pitchRatio
						curInc2 *= pitchRatio
					}

					var o1 float64
					switch s.wave1 {
					case WaveNoise:
						o1 = xorshift64Step(&v.noiseState1)
					case WavePinkNoise:
						o1 = kelletPinkStep(&v.pink1, xorshift64Step(&v.noiseState1))
					default:
						o1 = oscWaveBlepPWM(s.wave1, v.unisonPhase1[k], curInc1, s.pulseWidth1)
					}

					var o2 float64
					switch s.wave2 {
					case WaveNoise:
						o2 = xorshift64Step(&v.noiseState2)
					case WavePinkNoise:
						o2 = kelletPinkStep(&v.pink2, xorshift64Step(&v.noiseState2))
					default:
						o2 = oscWaveBlepPWM(s.wave2, v.unisonPhase2[k], curInc2, s.pulseWidth2)
					}

					v.unisonPhase1[k] += curInc1
					if v.unisonPhase1[k] >= 1 || v.unisonPhase1[k] < 0 {
						v.unisonPhase1[k] -= math.Floor(v.unisonPhase1[k])
					}
					v.unisonPhase2[k] += curInc2
					if v.unisonPhase2[k] >= 1 || v.unisonPhase2[k] < 0 {
						v.unisonPhase2[k] -= math.Floor(v.unisonPhase2[k])
					}

					if isStereoUnison {
						sum1L += o1 * uPanL[k]
						sum1R += o1 * uPanR[k]
						sum2L += o2 * uPanL[k]
						sum2R += o2 * uPanR[k]
					} else {
						sum1L += o1
						sum2L += o2
					}
				}

				if isStereoUnison {
					xL = 0.5 * (sum1L + sum2L) * uNorm
					xR = 0.5 * (sum1R + sum2R) * uNorm
				} else {
					xMono := 0.5 * (sum1L + sum2L) * uNorm
					xL = xMono
					xR = xMono
				}
			}

			var filtOutL, filtOutR float64

			if !hasFilterMod {
				switch s.filterType {
				case Filter12dBSVF:
					filtOutL = processSVF12(xL, staticSVF, &v.svfS1, &v.svfS2)
					if isStereoUnison {
						filtOutR = processSVF12(xR, staticSVF, &v.svfS1R, &v.svfS2R)
					}
				case Filter24dBCascade:
					filtOutL = processSVF24(xL, staticSVF, &v.svfS1, &v.svfS2, &v.svfS3, &v.svfS4)
					if isStereoUnison {
						filtOutR = processSVF24(xR, staticSVF, &v.svfS1R, &v.svfS2R, &v.svfS3R, &v.svfS4R)
					}
				case Filter24dBLadder:
					filtOutL = processLadder24(xL, staticLadder, &v.svfS1, &v.svfS2, &v.svfS3, &v.svfS4)
					if isStereoUnison {
						filtOutR = processLadder24(xR, staticLadder, &v.svfS1R, &v.svfS2R, &v.svfS3R, &v.svfS4R)
					}
				default:
					old := v.filtY
					v.filtY += s.filterAlpha*(xL-old) + s.filterRes*math.Tanh(old-v.filtPrev)
					v.filtPrev = old
					filtOutL = v.filtY
					if isStereoUnison {
						oldR := v.filtYR
						v.filtYR += s.filterAlpha*(xR-oldR) + s.filterRes*math.Tanh(oldR-v.filtPrevR)
						v.filtPrevR = oldR
						filtOutR = v.filtYR
					}
				}
			} else {
				envVal := 0.0
				if hasFilterEnv {
					envVal = v.filtEnv.Next(deltaTime)
				}
				octaves := float64(s.filterEnvDepth)*envVal + float64(s.lfoFilterDepth)*lfoVal
				if s.velocityToFilter > 0 {
					octaves += float64(s.velocityToFilter*v.velocity) * 4.0
				}
				modCutoff := float64(s.cutoff) * math.Pow(2, octaves)
				if math.IsNaN(modCutoff) || modCutoff < minCutoff {
					modCutoff = minCutoff
				} else if modCutoff > maxCutoff {
					modCutoff = maxCutoff
				}

				switch s.filterType {
				case Filter12dBSVF:
					dynSVF := calcSVFCoeffs(modCutoff, float64(s.resonance), s.sampleRate)
					filtOutL = processSVF12(xL, dynSVF, &v.svfS1, &v.svfS2)
					if isStereoUnison {
						filtOutR = processSVF12(xR, dynSVF, &v.svfS1R, &v.svfS2R)
					}
				case Filter24dBCascade:
					dynSVF := calcSVFCoeffs(modCutoff, float64(s.resonance), s.sampleRate)
					filtOutL = processSVF24(xL, dynSVF, &v.svfS1, &v.svfS2, &v.svfS3, &v.svfS4)
					if isStereoUnison {
						filtOutR = processSVF24(xR, dynSVF, &v.svfS1R, &v.svfS2R, &v.svfS3R, &v.svfS4R)
					}
				case Filter24dBLadder:
					dynLadder := calcLadderCoeffs(modCutoff, float64(s.resonance), s.sampleRate)
					filtOutL = processLadder24(xL, dynLadder, &v.svfS1, &v.svfS2, &v.svfS3, &v.svfS4)
					if isStereoUnison {
						filtOutR = processLadder24(xR, dynLadder, &v.svfS1R, &v.svfS2R, &v.svfS3R, &v.svfS4R)
					}
				default:
					alpha := 1 - math.Exp(-2*math.Pi*modCutoff/s.sampleRate)
					if math.IsNaN(alpha) || alpha > 1 {
						alpha = 1
					} else if alpha < 0 {
						alpha = 0
					}
					old := v.filtY
					v.filtY += alpha*(xL-old) + s.filterRes*math.Tanh(old-v.filtPrev)
					v.filtPrev = old
					filtOutL = v.filtY
					if isStereoUnison {
						oldR := v.filtYR
						v.filtYR += alpha*(xR-oldR) + s.filterRes*math.Tanh(oldR-v.filtPrevR)
						v.filtPrevR = oldR
						filtOutR = v.filtYR
					}
				}
			}

			if !isStereoUnison {
				filtOutR = filtOutL
			}

			envGain := v.env.Next(deltaTime) * vel
			if hasAmpLFO {
				ampMod := 1.0 - float64(s.lfoAmpDepth)*0.5*(1.0-lfoVal)
				if math.IsNaN(ampMod) || ampMod < 0 {
					ampMod = 0
				} else if ampMod > 1 {
					ampMod = 1
				}
				envGain *= ampMod
			}

			if v.panL == 1.0 && v.panR == 1.0 && !isStereoUnison {
				out := float32(filtOutL * envGain)
				left[j] += out
				right[j] += out
			} else {
				left[j] += float32(filtOutL * envGain * v.panL)
				right[j] += float32(filtOutR * envGain * v.panR)
			}
		}
		if v.env.Stage() == dsp.StageIdle {
			v.active = false
		}
	}
	if s.useDistortion && s.distortion != nil {
		s.distortion.ProcessStereo(left[:n], right[:n])
	}
}
