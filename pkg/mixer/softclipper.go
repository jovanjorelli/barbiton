package mixer

import (
	"math"

	"barbiton/pkg/dsp"
)

type SoftClipper struct {
	threshold       float32
	clean           bool
	useOversampling bool
	oversampler     *dsp.OversampledSaturator
}

func NewSoftClipper(threshold float32) *SoftClipper {
	t := clamp32(threshold, 0.05, 1)
	return &SoftClipper{
		threshold:   t,
		oversampler: dsp.NewOversampledSaturator(t),
	}
}

func (c *SoftClipper) SetThreshold(t float32) {
	c.threshold = clamp32(t, 0.05, 1)
	if c.oversampler != nil {
		c.oversampler.SetThreshold(c.threshold)
	}
}

func (c *SoftClipper) Threshold() float32 { return c.threshold }

func (c *SoftClipper) SetClean(clean bool) {
	c.clean = clean
	if c.oversampler != nil {
		c.oversampler.SetClean(clean)
	}
}

func (c *SoftClipper) Clean() bool { return c.clean }

func (c *SoftClipper) SetOversampling(enabled bool) {
	c.useOversampling = enabled
	if enabled && c.oversampler == nil {
		c.oversampler = dsp.NewOversampledSaturator(c.threshold)
	}
}

func (c *SoftClipper) Oversampling() bool { return c.useOversampling }

func (c *SoftClipper) ProcessSample(x float32) float32 {
	if c.useOversampling && c.oversampler != nil {
		return c.oversampler.ProcessSample(x)
	}
	if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
		return 0
	}
	if c.clean {
		if x > 1 {
			return 1
		}
		if x < -1 {
			return -1
		}
		return x
	}
	t := c.threshold
	if x <= t && x >= -t {
		return x
	}
	denom := float64(1 - t)
	if denom < 1e-6 {
		if x > 0 {
			return 1
		}
		return -1
	}
	over := float64(abs32(x)-t) / denom
	y := float64(t) + denom*math.Tanh(over)
	if x < 0 {
		y = -y
	}
	return float32(y)
}

func (c *SoftClipper) ProcessSlice(b []float32) {
	if c.useOversampling && c.oversampler != nil {
		c.oversampler.ProcessInterleaved(b)
		return
	}
	for i, x := range b {
		b[i] = c.ProcessSample(x)
	}
}
