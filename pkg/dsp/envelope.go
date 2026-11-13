package dsp

import "math"

type Stage uint8

const (
	StageIdle Stage = iota

	StageAttack

	StageDecay

	StageSustain

	StageRelease
)

type ADSR = Envelope

const expConvergence = 9.0

const silenceEpsilon = 1e-4

type Envelope struct {
	attack      float64
	decay       float64
	sustain     float64
	release     float64
	decayRate   float64
	releaseRate float64
	stage       Stage
	value       float64
	decayMult   float64
	releaseMult float64
	decayDt     float64
	releaseDt   float64
}

func NewEnvelope(attack, decay, sustain, release float64) *Envelope {
	e := &Envelope{}
	e.SetParameters(attack, decay, sustain, release)
	return e
}

func (e *Envelope) SetParameters(attack, decay, sustain, release float64) {
	e.attack = clampMin(attack, 1e-6)
	e.decay = clampMin(decay, 1e-6)
	e.release = clampMin(release, 1e-6)
	e.sustain = clamp01(sustain)
	e.decayRate = expConvergence / e.decay
	e.releaseRate = expConvergence / e.release
	if e.decayDt > 0 {
		e.decayMult = math.Exp(-e.decayRate * e.decayDt)
	} else if e.decayMult > 0 {
		e.decayMult = 1
	}
	if e.releaseDt > 0 {
		e.releaseMult = math.Exp(-e.releaseRate * e.releaseDt)
	} else if e.releaseMult > 0 {
		e.releaseMult = 1
	}
}

func (e *Envelope) Trigger() {
	e.value = 0
	e.stage = StageAttack
}

func (e *Envelope) Release() {
	switch e.stage {
	case StageAttack, StageDecay, StageSustain:
		e.stage = StageRelease
		if e.decayDt > 0 {
			e.releaseDt = e.decayDt
			e.releaseMult = math.Exp(-e.releaseRate * e.releaseDt)
		}
	}
}

func (e *Envelope) Stage() Stage { return e.stage }

func (e *Envelope) AttackTime() float64 { return e.attack }

func (e *Envelope) DecayTime() float64 { return e.decay }

func (e *Envelope) SustainLevel() float64 { return e.sustain }

func (e *Envelope) ReleaseTime() float64 { return e.release }

func (e *Envelope) Value() float64 { return e.value }

func (e *Envelope) Next(dt float64) float64 {
	switch e.stage {
	case StageAttack:

		e.value += dt / e.attack
		if e.value >= 1 {
			e.value = 1
			e.stage = StageDecay
			if dt != e.decayDt || e.decayMult == 0 {
				e.decayDt = dt
				e.decayMult = math.Exp(-e.decayRate * dt)
			}
		}
	case StageDecay:

		if dt != e.decayDt || e.decayMult == 0 {
			e.decayDt = dt
			e.decayMult = math.Exp(-e.decayRate * dt)
		}
		e.value = e.sustain + (e.value-e.sustain)*e.decayMult
		if math.Abs(e.value-e.sustain) < silenceEpsilon {
			e.value = e.sustain
			e.stage = StageSustain
		}
	case StageSustain:
		e.value = e.sustain
	case StageRelease:

		if dt != e.releaseDt || e.releaseMult == 0 {
			e.releaseDt = dt
			e.releaseMult = math.Exp(-e.releaseRate * dt)
		}
		e.value *= e.releaseMult
		if e.value < silenceEpsilon {
			e.value = 0
			e.stage = StageIdle
		}
	case StageIdle:
		e.value = 0
	}
	return clamp01(e.value)
}

func (e *Envelope) Fill(dst []float32, dt float64) {
	for i := range dst {
		dst[i] = float32(e.Next(dt))
	}
}

func (e *Envelope) Apply(dst []float32, dt float64) {
	for i := range dst {
		dst[i] *= float32(e.Next(dt))
	}
}

func clampMin(v, lo float64) float64 {
	if v < lo {
		return lo
	}
	return v
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}

func ClampF32(v, lo, hi float32) float32 {
	switch {
	case v < lo || v != v:
		return lo
	case v > hi:
		return hi
	}
	return v
}
