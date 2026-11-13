package instruments

import "barbiton/pkg/dsp"

type EnvelopeOverride struct {
	Attack, Decay, Release          float64
	HasAttack, HasDecay, HasRelease bool
}

type NoteParams struct {
	Env EnvelopeOverride

	VibratoRate, VibratoDepth float64
	HasVibrato                bool

	Expression    float32
	HasExpression bool

	XFadeGain float32
	HasXFade  bool
}

type ParamsVoicer interface {
	Instrument
	NoteOnParams(pitch int, velocity float32, params NoteParams)
}

type EnvelopeVoicer interface {
	Instrument
	NoteOnEx(pitch int, velocity float32, env EnvelopeOverride)
}

type PedalVoicer interface {
	Instrument
	SetPedal(down bool)
}

type Instrument interface {
	Process(buffer *dsp.Buffer, deltaTime float64)

	NoteOn(pitch int, velocity float32)

	NoteOff(pitch int)

	SetParameters(params map[string]float32)
}
