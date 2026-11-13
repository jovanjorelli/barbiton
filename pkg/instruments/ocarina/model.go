package ocarina

import (
	"math"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
)

var _ instruments.Instrument = (*Ocarina)(nil)

var _ instruments.ParamsVoicer = (*Ocarina)(nil)

const (
	speedOfSound = 343.0
	twoPi        = 2 * math.Pi

	defaultVolume     = 2.5e-4
	defaultNeckLength = 0.015

	harmonic2Gain = 0.25118864315095801
	harmonic3Gain = 0.12589254117941673

	formantLow  = 2000.0
	formantMid  = 4000.0
	formantHigh = 8000.0
	formantQ    = 2.0

	breathMix = 0.3

	attackTime   = 0.05
	releaseTime  = 0.10
	sustainLevel = 1.0
	decayTime    = 1e-6
)

const (
	defaultBreathNoise  float32 = 0.3
	defaultVibratoRate  float32 = 5.0
	defaultVibratoDepth float32 = 0.02
	defaultBrightness   float32 = 1.0
)

type Ocarina struct {
	volume     float64
	neckLength float64
	neckArea   float64
	freq       float64

	note     int
	velocity float32

	tonePhase    float64
	vibratoPhase float64
	noise        *dsp.Noise
	formants     [3]*dsp.Biquad
	env          *dsp.Envelope

	breathNoise  float32
	vibratoRate  float32
	vibratoDepth float32
	brightness   float32
	expr         float32
}

func New(sampleRate float64) *Ocarina {
	o := &Ocarina{
		volume:       defaultVolume,
		neckLength:   defaultNeckLength,
		note:         -1,
		noise:        dsp.NewNoise(0),
		env:          dsp.NewEnvelope(attackTime, decayTime, sustainLevel, releaseTime),
		breathNoise:  defaultBreathNoise,
		vibratoRate:  defaultVibratoRate,
		vibratoDepth: defaultVibratoDepth,
		brightness:   defaultBrightness,
		expr:         1,
	}
	o.formants[0] = dsp.NewBiquad(dsp.Bandpass, formantLow, formantQ, sampleRate)
	o.formants[1] = dsp.NewBiquad(dsp.Bandpass, formantMid, formantQ, sampleRate)
	o.formants[2] = dsp.NewBiquad(dsp.Bandpass, formantHigh, formantQ, sampleRate)
	return o
}

func HelmholtzFrequency(neckArea, volume, neckLength float64) float64 {
	if neckArea <= 0 || volume <= 0 || neckLength <= 0 {
		return 0
	}
	return speedOfSound / twoPi * math.Sqrt(neckArea/(volume*neckLength))
}

func (o *Ocarina) AreaForFrequency(freq float64) float64 {
	w := twoPi * freq / speedOfSound
	return o.volume * o.neckLength * w * w
}

func (o *Ocarina) NoteOn(pitch int, velocity float32) {
	o.NoteOnParams(pitch, velocity, instruments.NoteParams{Expression: 1, HasExpression: true})
}

func (o *Ocarina) NoteOnEx(pitch int, velocity float32, env instruments.EnvelopeOverride) {
	o.NoteOnParams(pitch, velocity, instruments.NoteParams{Env: env, Expression: 1, HasExpression: true})
}

func (o *Ocarina) NoteOnParams(pitch int, velocity float32, params instruments.NoteParams) {
	o.note = pitch
	o.velocity = dsp.ClampF32(velocity, 0, 1)
	o.neckArea = o.AreaForFrequency(midiToFreq(pitch))
	o.freq = HelmholtzFrequency(o.neckArea, o.volume, o.neckLength)
	env := params.Env
	attack, decay, release := attackTime, decayTime, releaseTime
	if env.HasAttack && env.Attack > 0 {
		attack = env.Attack
	}
	if env.HasDecay && env.Decay > 0 {
		decay = env.Decay
	}
	if env.HasRelease && env.Release > 0 {
		release = env.Release
	}
	o.env.SetParameters(attack, decay, sustainLevel, release)
	if params.HasVibrato {
		if r := params.VibratoRate; r > 0 {
			o.vibratoRate = dsp.ClampF32(float32(r), 1, 10)
		}
		if d := params.VibratoDepth; d > 0 {
			o.vibratoDepth = dsp.ClampF32(float32(math.Pow(2, d/12)-1), 0, 0.1)
		}
	}
	expr := float32(1)
	if params.HasExpression {
		expr = params.Expression
		if math.IsNaN(float64(expr)) {
			expr = 1
		} else if expr < 0 {
			expr = 0
		} else if expr > 1 {
			expr = 1
		}
	}
	o.expr = expr
	o.env.Trigger()
}

func (o *Ocarina) NoteOff(pitch int) {
	if o.note == pitch && o.env.Stage() != dsp.StageIdle {
		o.env.Release()
		o.note = -1
	}
}

func (o *Ocarina) SetParameters(params map[string]float32) {
	if v, ok := params["breath_noise"]; ok {
		o.breathNoise = dsp.ClampF32(v, 0, 1)
	}
	if v, ok := params["vibrato_rate"]; ok {
		o.vibratoRate = dsp.ClampF32(v, 1, 10)
	}
	if v, ok := params["vibrato_depth"]; ok {
		o.vibratoDepth = dsp.ClampF32(v, 0, 0.1)
	}
	if v, ok := params["brightness"]; ok {
		o.brightness = dsp.ClampF32(v, 0, 1)
	}
}

func (o *Ocarina) Frequency() float64 { return o.freq }

func (o *Ocarina) Note() int { return o.note }

func (o *Ocarina) BreathNoise() float32 { return o.breathNoise }

func (o *Ocarina) VibratoRate() float32 { return o.vibratoRate }

func (o *Ocarina) VibratoDepth() float32 { return o.vibratoDepth }

func (o *Ocarina) Brightness() float32 { return o.brightness }

func (o *Ocarina) Process(buffer *dsp.Buffer, deltaTime float64) {
	if buffer == nil || deltaTime <= 0 {
		return
	}
	left, right := buffer.Left, buffer.Right
	if o.env.Stage() == dsp.StageIdle {
		clear(left)
		clear(right)
		return
	}

	vel := float64(o.velocity)
	depth := float64(o.vibratoDepth)
	rate := float64(o.vibratoRate)
	breathGain := float64(o.breathNoise) * vel
	h2 := harmonic2Gain * float64(o.brightness)
	h3 := harmonic3Gain * float64(o.brightness)
	norm := 1 / (1 + h2 + h3)
	dt := deltaTime

	for i := range left {

		f := o.freq * (1 + depth*float64(dsp.SineFast(o.vibratoPhase)))
		o.vibratoPhase = wrap01(o.vibratoPhase + rate*dt)

		tp := o.tonePhase
		tone := (float64(dsp.SineFast(tp)) + h2*float64(dsp.SineFast(2*tp)) +
			h3*float64(dsp.SineFast(3*tp))) * norm
		o.tonePhase = wrap01(tp + f*dt)

		nz := o.noise.Next()
		breath := o.formants[0].ProcessSample(nz) +
			o.formants[1].ProcessSample(nz) +
			o.formants[2].ProcessSample(nz)

		out := float32((tone + float64(breath)*breathMix*breathGain) * o.env.Next(dt) * vel * float64(o.expr))
		left[i], right[i] = out, out
	}
}

func midiToFreq(pitch int) float64 {
	return 440 * math.Pow(2, float64(pitch-69)/12)
}

func wrap01(p float64) float64 {
	if p >= 1 {
		p -= math.Floor(p)
	}
	return p
}
