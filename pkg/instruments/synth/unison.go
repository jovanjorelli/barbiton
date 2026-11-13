package synth

import "math"

func computeVoicePan(spread float32, voiceIndex int) (float64, float64) {
	if math.IsNaN(float64(spread)) || spread <= 0 {
		return 1.0, 1.0
	}
	s := spread
	if s > 1 {
		s = 1
	}
	idx := voiceIndex % 8
	if idx < 0 {
		idx += 8
	}
	p := float64(s) * (2.0*float64(idx)/7.0 - 1.0)
	theta := (math.Pi / 4.0) * (p + 1.0)
	return math.Sqrt(2.0) * math.Cos(theta), math.Sqrt(2.0) * math.Sin(theta)
}

func unisonDetuneRatio(subIndex, totalVoices int, detune float32) float64 {
	if totalVoices <= 1 || math.IsNaN(float64(detune)) || detune == 0 {
		return 1.0
	}
	d := detune
	if d < 0 {
		d = 0
	} else if d > 1 {
		d = 1
	}
	idx := subIndex
	if idx < 0 {
		idx = 0
	} else if idx >= totalVoices {
		idx = totalVoices - 1
	}
	delta := float64(2*idx)/float64(totalVoices-1) - 1.0
	return math.Pow(2.0, delta*float64(d)/12.0)
}

func unisonSubPan(subIndex, totalVoices int, spread float32) (float64, float64) {
	if totalVoices <= 1 || math.IsNaN(float64(spread)) || spread <= 0 {
		return 1.0, 1.0
	}
	s := spread
	if s > 1 {
		s = 1
	}
	idx := subIndex
	if idx < 0 {
		idx = 0
	} else if idx >= totalVoices {
		idx = totalVoices - 1
	}
	delta := float64(2*idx)/float64(totalVoices-1) - 1.0
	p := delta * float64(s)
	theta := (math.Pi / 4.0) * (p + 1.0)
	return math.Sqrt(2.0) * math.Cos(theta), math.Sqrt(2.0) * math.Sin(theta)
}

func unisonPowerNorm(totalVoices int) float64 {
	if totalVoices <= 1 {
		return 1.0
	}
	n := totalVoices
	if n > 7 {
		n = 7
	}
	return 1.0 / math.Sqrt(float64(n))
}
