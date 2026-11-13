package dsp

import (
	"math"
)

type OversampledSaturator struct {
	threshold float32
	drive     float32
	clean     bool

	ringL   [32]float32
	ringR   [32]float32
	ringPos int

	decRingEvenL [32]float32
	decRingEvenR [32]float32
	decRingOddL  [32]float32
	decRingOddR  [32]float32
	decPos       int
}

var halfBandCoeffs = [7]float32{
	0.6115276,
	-0.1549276,
	0.0608594,
	-0.0232759,
	0.0070792,
	-0.0014119,
	0.0001491,
}

func NewOversampledSaturator(threshold float32) *OversampledSaturator {
	if threshold < 0.05 {
		threshold = 0.05
	} else if threshold > 1.0 {
		threshold = 1.0
	}
	return &OversampledSaturator{
		threshold: threshold,
		drive:     1.0,
	}
}

func (s *OversampledSaturator) SetThreshold(t float32) {
	if t < 0.05 {
		t = 0.05
	} else if t > 1.0 {
		t = 1.0
	}
	s.threshold = t
}

func (s *OversampledSaturator) Threshold() float32 {
	return s.threshold
}

func (s *OversampledSaturator) SetDrive(d float32) {
	if d < 0.1 {
		d = 0.1
	} else if d > 20.0 {
		d = 20.0
	}
	s.drive = d
}

func (s *OversampledSaturator) Drive() float32 {
	return s.drive
}

func (s *OversampledSaturator) SetClean(clean bool) {
	s.clean = clean
}

func (s *OversampledSaturator) Clean() bool {
	return s.clean
}

func (s *OversampledSaturator) Reset() {
	clear(s.ringL[:])
	clear(s.ringR[:])
	clear(s.decRingEvenL[:])
	clear(s.decRingEvenR[:])
	clear(s.decRingOddL[:])
	clear(s.decRingOddR[:])
	s.ringPos = 0
	s.decPos = 0
}

func (s *OversampledSaturator) saturateSample(x float32) float32 {
	if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
		return 0
	}
	if s.clean {
		if x > 1.0 {
			return 1.0
		}
		if x < -1.0 {
			return -1.0
		}
		return x
	}

	u := x * s.drive
	t := s.threshold
	if u <= t && u >= -t {
		return u
	}

	denom := 1.0 - t
	if denom < 1e-6 {
		if u > 0 {
			return 1.0
		}
		return -1.0
	}

	a := u
	sign := float32(1.0)
	if a < 0 {
		a = -a
		sign = -1.0
	}

	over := float64((a - t) / denom)
	y := float32(float64(t) + float64(denom)*math.Tanh(over))
	return sign * y
}

func (s *OversampledSaturator) ProcessSample(x float32) float32 {
	outL, _ := s.ProcessFrame(x, 0)
	return outL
}

func (s *OversampledSaturator) ProcessFrame(inL, inR float32) (float32, float32) {
	if inL > -1e-25 && inL < 1e-25 {
		inL = 0
	}
	if inR > -1e-25 && inR < 1e-25 {
		inR = 0
	}

	m := 31
	p := s.ringPos
	s.ringL[p] = inL
	s.ringR[p] = inR

	evenL := s.ringL[(p-7)&m]
	evenR := s.ringR[(p-7)&m]

	var oddL, oddR float32
	oddL += halfBandCoeffs[0] * (s.ringL[(p-6)&m] + s.ringL[(p-7)&m])
	oddR += halfBandCoeffs[0] * (s.ringR[(p-6)&m] + s.ringR[(p-7)&m])

	oddL += halfBandCoeffs[1] * (s.ringL[(p-5)&m] + s.ringL[(p-8)&m])
	oddR += halfBandCoeffs[1] * (s.ringR[(p-5)&m] + s.ringR[(p-8)&m])

	oddL += halfBandCoeffs[2] * (s.ringL[(p-4)&m] + s.ringL[(p-9)&m])
	oddR += halfBandCoeffs[2] * (s.ringR[(p-4)&m] + s.ringR[(p-9)&m])

	oddL += halfBandCoeffs[3] * (s.ringL[(p-3)&m] + s.ringL[(p-10)&m])
	oddR += halfBandCoeffs[3] * (s.ringR[(p-3)&m] + s.ringR[(p-10)&m])

	oddL += halfBandCoeffs[4] * (s.ringL[(p-2)&m] + s.ringL[(p-11)&m])
	oddR += halfBandCoeffs[4] * (s.ringR[(p-2)&m] + s.ringR[(p-11)&m])

	oddL += halfBandCoeffs[5] * (s.ringL[(p-1)&m] + s.ringL[(p-12)&m])
	oddR += halfBandCoeffs[5] * (s.ringR[(p-1)&m] + s.ringR[(p-12)&m])

	oddL += halfBandCoeffs[6] * (s.ringL[p&m] + s.ringL[(p-13)&m])
	oddR += halfBandCoeffs[6] * (s.ringR[p&m] + s.ringR[(p-13)&m])

	s.ringPos = (p + 1) & m

	satEvenL := s.saturateSample(evenL)
	satEvenR := s.saturateSample(evenR)
	satOddL := s.saturateSample(oddL)
	satOddR := s.saturateSample(oddR)

	dp := s.decPos
	s.decRingEvenL[dp] = satEvenL
	s.decRingEvenR[dp] = satEvenR
	s.decRingOddL[dp] = satOddL
	s.decRingOddR[dp] = satOddR

	evenValL := s.decRingEvenL[(dp-7)&m]
	evenValR := s.decRingEvenR[(dp-7)&m]

	var decOddL, decOddR float32
	decOddL += halfBandCoeffs[0] * (s.decRingOddL[(dp-7)&m] + s.decRingOddL[(dp-8)&m])
	decOddR += halfBandCoeffs[0] * (s.decRingOddR[(dp-7)&m] + s.decRingOddR[(dp-8)&m])

	decOddL += halfBandCoeffs[1] * (s.decRingOddL[(dp-6)&m] + s.decRingOddL[(dp-9)&m])
	decOddR += halfBandCoeffs[1] * (s.decRingOddR[(dp-6)&m] + s.decRingOddR[(dp-9)&m])

	decOddL += halfBandCoeffs[2] * (s.decRingOddL[(dp-5)&m] + s.decRingOddL[(dp-10)&m])
	decOddR += halfBandCoeffs[2] * (s.decRingOddR[(dp-5)&m] + s.decRingOddR[(dp-10)&m])

	decOddL += halfBandCoeffs[3] * (s.decRingOddL[(dp-4)&m] + s.decRingOddL[(dp-11)&m])
	decOddR += halfBandCoeffs[3] * (s.decRingOddR[(dp-4)&m] + s.decRingOddR[(dp-11)&m])

	decOddL += halfBandCoeffs[4] * (s.decRingOddL[(dp-3)&m] + s.decRingOddL[(dp-12)&m])
	decOddR += halfBandCoeffs[4] * (s.decRingOddR[(dp-3)&m] + s.decRingOddR[(dp-12)&m])

	decOddL += halfBandCoeffs[5] * (s.decRingOddL[(dp-2)&m] + s.decRingOddL[(dp-13)&m])
	decOddR += halfBandCoeffs[5] * (s.decRingOddR[(dp-2)&m] + s.decRingOddR[(dp-13)&m])

	decOddL += halfBandCoeffs[6] * (s.decRingOddL[(dp-1)&m] + s.decRingOddL[(dp-14)&m])
	decOddR += halfBandCoeffs[6] * (s.decRingOddR[(dp-1)&m] + s.decRingOddR[(dp-14)&m])

	s.decPos = (dp + 1) & m

	finalL := 0.5 * (evenValL + decOddL)
	finalR := 0.5 * (evenValR + decOddR)

	if finalL > 1.0 {
		finalL = 1.0
	} else if finalL < -1.0 {
		finalL = -1.0
	}
	if finalR > 1.0 {
		finalR = 1.0
	} else if finalR < -1.0 {
		finalR = -1.0
	}

	if math.IsNaN(float64(finalL)) || math.IsInf(float64(finalL), 0) {
		finalL = 0
	}
	if math.IsNaN(float64(finalR)) || math.IsInf(float64(finalR), 0) {
		finalR = 0
	}

	return finalL, finalR
}

func (s *OversampledSaturator) ProcessSlice(b []float32) {
	n := len(b)
	if n == 0 {
		return
	}
	_ = b[n-1]
	for i := 0; i < n; i++ {
		b[i] = s.ProcessSample(b[i])
	}
}

func (s *OversampledSaturator) ProcessInterleaved(b []float32) {
	frames := len(b) / 2
	if frames == 0 {
		return
	}
	_ = b[2*frames-1]
	for i := 0; i < frames; i++ {
		oL, oR := s.ProcessFrame(b[2*i], b[2*i+1])
		b[2*i] = oL
		b[2*i+1] = oR
	}
}

func (s *OversampledSaturator) ProcessStereo(left, right []float32) {
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	if n == 0 {
		return
	}
	_ = left[n-1]
	_ = right[n-1]
	for i := 0; i < n; i++ {
		oL, oR := s.ProcessFrame(left[i], right[i])
		left[i] = oL
		right[i] = oR
	}
}
