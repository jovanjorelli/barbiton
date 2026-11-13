package synth

import (
	"math"

	"barbiton/pkg/dsp"
)

func (s *Synth) processFM(buffer *dsp.Buffer, deltaTime float64) {
	left, right := buffer.Left, buffer.Right
	n := len(left)
	if len(right) < n {
		n = len(right)
	}

	ratio := s.fmRatio
	feedback := s.fmFeedback
	depth := s.fmDepth
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
		hasAnyLFO := hasPitchMod || hasAmpLFO

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

		incC := freq * deltaTime
		incM := freq * ratio * deltaTime

		for j := 0; j < n; j++ {
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

			curIncC := incC * pitchRatio
			curIncM := incM * pitchRatio

			ym := math.Sin(2*math.Pi*v.fmPhaseM + feedback*v.fmModPrev)
			v.fmModPrev = ym

			modIdx := depth * v.fmEnv.Next(deltaTime)

			yc := math.Sin(2*math.Pi*v.fmPhaseC + modIdx*ym)

			v.fmPhaseC += curIncC
			if v.fmPhaseC >= 1 || v.fmPhaseC < 0 {
				v.fmPhaseC -= math.Floor(v.fmPhaseC)
			}

			v.fmPhaseM += curIncM
			if v.fmPhaseM >= 1 || v.fmPhaseM < 0 {
				v.fmPhaseM -= math.Floor(v.fmPhaseM)
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

			if v.panL == 1.0 && v.panR == 1.0 {
				out := float32(yc * envGain)
				left[j] += out
				right[j] += out
			} else {
				left[j] += float32(yc * envGain * v.panL)
				right[j] += float32(yc * envGain * v.panR)
			}
		}

		if v.env.Stage() == dsp.StageIdle {
			v.active = false
		}
	}
}
