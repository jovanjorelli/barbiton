package synth

import (
	"math"

	"barbiton/pkg/dsp"
)

func (s *Synth) processWavetable(buffer *dsp.Buffer, deltaTime float64) {
	left, right := buffer.Left, buffer.Right
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	if n == 0 {
		return
	}
	_ = left[n-1]
	_ = right[n-1]

	wt := ReferenceWavetable(s.wavetableID)
	pbRatio := 1.0
	if s.pitchBend != 0 {
		pbRatio = math.Pow(2.0, s.pitchBend/12.0)
	}

	detuneRatio := math.Pow(2, float64(s.detune)/12)
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
				inc1 = v.freq * pbRatio * deltaTime
				inc2 = v.freq * pbRatio * detuneRatio * deltaTime
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

			filtEnvVal := 0.0
			if hasFilterEnv {
				filtEnvVal = v.filtEnv.Next(deltaTime)
			}
			wtPos := float64(s.wavetablePos) + float64(s.wavetableEnvDepth)*filtEnvVal + float64(s.wavetableLFODepth)*lfoVal

			var xL, xR float64

			if uVoices <= 1 {
				curInc1 := inc1 * pitchRatio
				curInc2 := inc2 * pitchRatio
				s1 := SampleWavetableHermite(wt, wtPos, v.phase1)
				s2 := SampleWavetableHermite(wt, wtPos, v.phase2)
				sMixed := 0.5 * (s1 + s2)
				xL = sMixed
				xR = sMixed
				v.phase1 += curInc1
				if v.phase1 >= 1 || v.phase1 < 0 {
					v.phase1 -= math.Floor(v.phase1)
				}
				v.phase2 += curInc2
				if v.phase2 >= 1 || v.phase2 < 0 {
					v.phase2 -= math.Floor(v.phase2)
				}
			} else {
				curIncBase1 := inc1 * pitchRatio
				curIncBase2 := inc2 * pitchRatio
				for k := 0; k < uVoices; k++ {
					kInc1 := curIncBase1 * uDetuneRatios[k]
					kInc2 := curIncBase2 * uDetuneRatios[k]
					s1 := SampleWavetableHermite(wt, wtPos, v.unisonPhase1[k])
					s2 := SampleWavetableHermite(wt, wtPos, v.unisonPhase2[k])
					sMixed := 0.5 * (s1 + s2)
					if isStereoUnison {
						xL += sMixed * uPanL[k]
						xR += sMixed * uPanR[k]
					} else {
						xL += sMixed
						xR += sMixed
					}
					v.unisonPhase1[k] += kInc1
					if v.unisonPhase1[k] >= 1 || v.unisonPhase1[k] < 0 {
						v.unisonPhase1[k] -= math.Floor(v.unisonPhase1[k])
					}
					v.unisonPhase2[k] += kInc2
					if v.unisonPhase2[k] >= 1 || v.unisonPhase2[k] < 0 {
						v.unisonPhase2[k] -= math.Floor(v.unisonPhase2[k])
					}
				}
				xL *= uNorm
				xR *= uNorm
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
				octaves := float64(s.filterEnvDepth)*filtEnvVal + float64(s.lfoFilterDepth)*lfoVal
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

			if isStereoUnison {
				left[j] += float32(filtOutL * envGain * v.panL)
				right[j] += float32(filtOutR * envGain * v.panR)
			} else if v.panL == 1.0 && v.panR == 1.0 {
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
}
