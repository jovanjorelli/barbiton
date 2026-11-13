package sampler

import "math"

func cubicPoint(p0, p1, p2, p3, x float64) float64 {
	c1 := 0.5 * (p2 - p0)
	c2 := math.FMA(-2.5, p1, math.FMA(2, p2, math.FMA(-0.5, p3, p0)))
	c3 := math.FMA(1.5, p1-p2, 0.5*(p3-p0))
	return math.FMA(math.FMA(math.FMA(c3, x, c2), x, c1), x, p1)
}

func readSample(src []float32, i int) float64 {
	if len(src) == 0 {
		return 0
	}
	if i < 0 {
		i = 0
	} else if i >= len(src) {
		i = len(src) - 1
	}
	return float64(src[i])
}

func readSampleBounded(src []float32, i, lo, hi int) float64 {
	if len(src) == 0 {
		return 0
	}
	if hi <= lo {
		return readSample(src, i)
	}
	if i < lo {
		i = lo
	} else if i >= hi {
		i = hi - 1
	}
	if i < 0 {
		i = 0
	} else if i >= len(src) {
		i = len(src) - 1
	}
	return float64(src[i])
}

func interpSample(src []float32, phase float64) float64 {
	index := int(phase)
	frac := phase - float64(index)
	return cubicPoint(
		readSample(src, index-1),
		readSample(src, index),
		readSample(src, index+1),
		readSample(src, index+2),
		frac)
}

func interpSampleBounded(src []float32, phase float64, lo, hi int) float64 {
	index := int(phase)
	frac := phase - float64(index)
	return cubicPoint(
		readSampleBounded(src, index-1, lo, hi),
		readSampleBounded(src, index, lo, hi),
		readSampleBounded(src, index+1, lo, hi),
		readSampleBounded(src, index+2, lo, hi),
		frac)
}

func interpSampleLinear(src []float32, phase float64) float64 {
	index := int(phase)
	frac := phase - float64(index)
	s1 := readSample(src, index)
	s2 := readSample(src, index+1)
	return s1 + (s2-s1)*frac
}

func ResampleTo(dst []float32, src []float32, rate, startPhase float64) int {
	return resampleTo(dst, src, rate, startPhase, false)
}

func ResampleLinearTo(dst []float32, src []float32, rate, startPhase float64) int {
	return resampleTo(dst, src, rate, startPhase, true)
}

func resampleTo(dst []float32, src []float32, rate, startPhase float64, linear bool) int {
	if len(src) == 0 || !(rate > 0) || math.IsNaN(startPhase) {
		return 0
	}
	phase := startPhase
	if phase < 0 {
		phase = 0
	}
	end := float64(len(src))
	written := 0
	for written < len(dst) && phase < end {
		if linear {
			dst[written] = float32(interpSampleLinear(src, phase))
		} else {
			dst[written] = float32(interpSample(src, phase))
		}
		written++
		phase += rate
	}
	return written
}
