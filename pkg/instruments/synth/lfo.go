package synth

import "math"

type LFOWaveform int

const (
	LFOSine LFOWaveform = iota
	LFOTriangle
	LFOSaw
	LFOSquare
	LFOSampleAndHold
)

const LFOSSquare = LFOSquare

func lfoWaveParam(v float32) LFOWaveform {
	if math.IsNaN(float64(v)) {
		return LFOSine
	}
	switch int(math.Round(float64(v))) {
	case 1:
		return LFOTriangle
	case 2:
		return LFOSaw
	case 3:
		return LFOSquare
	case 4:
		return LFOSampleAndHold
	default:
		return LFOSine
	}
}

type voiceLFO struct {
	phase      float64
	delayTimer float64
	shVal      float64
	seed       uint64
}

func (lfo *voiceLFO) trigger(seed uint64) {
	lfo.phase = 0
	lfo.delayTimer = 0
	if seed == 0 {
		seed = 0x123456789abcdef0
	}
	lfo.seed = seed
	lfo.shVal = xorshift64Step(&lfo.seed)
}

func (lfo *voiceLFO) step(rate, delay float64, wave LFOWaveform, dt float64) float64 {
	if dt <= 0 || math.IsNaN(dt) {
		return 0
	}
	lfo.delayTimer += dt
	if delay > 0 && lfo.delayTimer < delay {
		return 0
	}
	r := rate
	if math.IsNaN(r) || r < 0.01 {
		r = 0.01
	} else if r > 50 {
		r = 50
	}
	lfo.phase += r * dt
	wrapped := false
	if lfo.phase >= 1.0 {
		lfo.phase -= math.Floor(lfo.phase)
		wrapped = true
	} else if lfo.phase < 0 {
		lfo.phase -= math.Floor(lfo.phase)
	}
	if wrapped && wave == LFOSampleAndHold {
		lfo.shVal = xorshift64Step(&lfo.seed)
	}
	var val float64
	switch wave {
	case LFOTriangle:
		val = 1.0 - 4.0*math.Abs(lfo.phase-0.5)
	case LFOSaw:
		val = 2.0*lfo.phase - 1.0
	case LFOSquare:
		if lfo.phase < 0.5 {
			val = 1.0
		} else {
			val = -1.0
		}
	case LFOSampleAndHold:
		val = lfo.shVal
	default:
		val = math.Sin(2.0 * math.Pi * lfo.phase)
	}
	return val
}
