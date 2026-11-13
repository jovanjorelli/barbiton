package synth

import (
	"math"

	"barbiton/pkg/dsp"
)

const pluckBufSize = 2400

func (s *Synth) initPluckVoice(v *voice, release float64) {
	damping := s.pluckDamping
	S := 1.0 - 0.5*damping
	freq := v.freq
	if freq < 20.0 {
		freq = 20.0
	} else if freq > 20000.0 {
		freq = 20000.0
	}

	D := (s.sampleRate / freq) - (1.0 - S)
	if D < 2.0 {
		D = 2.0
	} else if D > float64(pluckBufSize-2) {
		D = float64(pluckBufSize - 2)
	}

	L := int(math.Ceil(D))
	if L < 2 {
		L = 2
	} else if L > pluckBufSize-2 {
		L = pluckBufSize - 2
	}

	for i := 0; i < pluckBufSize; i++ {
		v.pluckBuf[i] = 0
	}

	seed := dsp.DefaultNoiseSeed ^ (uint64(v.pitch)*1000003 + uint64(s.clock)*7919 + 7)
	if seed == 0 {
		seed = dsp.DefaultNoiseSeed
	}

	var sum float64
	for i := 0; i < L; i++ {
		sample := float32(xorshift64Step(&seed))
		v.pluckBuf[i] = sample
		sum += float64(sample)
	}

	mean := float32(sum / float64(L))
	for i := 0; i < L; i++ {
		v.pluckBuf[i] -= mean
	}

	brightness := s.pluckBrightness
	if brightness < 200 {
		brightness = 200
	} else if brightness > 20000 {
		brightness = 20000
	}
	alpha := 1.0 - math.Exp(-2.0*math.Pi*brightness/s.sampleRate)
	if alpha > 1.0 {
		alpha = 1.0
	} else if alpha < 0.001 {
		alpha = 0.001
	}

	var yLP float64
	for i := 0; i < L; i++ {
		yLP += alpha * (float64(v.pluckBuf[i]) - yLP)
		v.pluckBuf[i] = float32(yLP)
	}

	pos := s.pluckPickPosition
	if pos > 0 {
		M := int(math.Round(pos * float64(L)))
		if M < 1 {
			M = 1
		} else if M >= L {
			M = L - 1
		}
		var temp [pluckBufSize]float32
		copy(temp[:L], v.pluckBuf[:L])
		for i := 0; i < L; i++ {
			prevIdx := i - M
			if prevIdx < 0 {
				prevIdx += L
			}
			v.pluckBuf[i] = temp[i] - temp[prevIdx]
		}
	}

	maxPeak := float32(0)
	for i := 0; i < L; i++ {
		val := v.pluckBuf[i]
		if val < 0 {
			val = -val
		}
		if val > maxPeak {
			maxPeak = val
		}
	}

	vel := v.velocity
	if maxPeak > 1e-6 {
		scale := vel / maxPeak
		for i := 0; i < L; i++ {
			v.pluckBuf[i] *= scale
		}
	}

	v.pluckWrite = L
	v.pluckPrev = float64(v.pluckBuf[L-1])

	v.env.SetParameters(0.0001, 10.0, 1.0, release)
	v.env.Trigger()
}

func (s *Synth) processPluck(buffer *dsp.Buffer, deltaTime float64) {
	left, right := buffer.Left, buffer.Right
	n := len(left)
	if len(right) < n {
		n = len(right)
	}

	damping := s.pluckDamping
	S := 1.0 - 0.5*damping
	oneMinusS := 1.0 - S
	decay := s.pluckDecay

	for i := range s.voices {
		v := &s.voices[i]
		if !v.active {
			continue
		}

		freq := v.freq
		if freq < 20.0 {
			freq = 20.0
		} else if freq > 20000.0 {
			freq = 20000.0
		}

		D := (s.sampleRate / freq) - oneMinusS
		if D < 2.0 {
			D = 2.0
		} else if D > float64(pluckBufSize-2) {
			D = float64(pluckBufSize - 2)
		}

		w := v.pluckWrite
		prev := v.pluckPrev
		expr := float64(v.expr)

		for j := 0; j < n; j++ {
			r := float64(w) - D
			if r < 0 {
				r += float64(pluckBufSize)
			} else if r >= float64(pluckBufSize) {
				r -= float64(pluckBufSize)
			}

			i0 := int(r)
			frac := r - float64(i0)
			i1 := i0 + 1
			if i1 >= pluckBufSize {
				i1 = 0
			}

			yRead := (1.0-frac)*float64(v.pluckBuf[i0]) + frac*float64(v.pluckBuf[i1])
			yFilt := decay * (S*yRead + oneMinusS*prev)
			if math.Abs(yFilt) < 1e-15 {
				yFilt = 0
			}
			prev = yRead

			v.pluckBuf[w] = float32(yFilt)
			w++
			if w >= pluckBufSize {
				w = 0
			}

			envGain := v.env.Next(deltaTime) * expr
			out := float32(yFilt * envGain)

			if v.panL == 1.0 && v.panR == 1.0 {
				left[j] += out
				right[j] += out
			} else {
				left[j] += out * float32(v.panL)
				right[j] += out * float32(v.panR)
			}
		}

		v.pluckWrite = w
		v.pluckPrev = prev

		if v.env.Stage() == dsp.StageIdle {
			v.active = false
		}
	}
}
