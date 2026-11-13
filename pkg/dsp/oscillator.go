package dsp

import "math"

const twoPi = 2 * math.Pi

const DefaultNoiseSeed uint64 = 0x9E3779B97F4A7C15

type Sine struct {
	phase float64
	inc   float64
}

func NewSine() *Sine { return &Sine{} }

func (s *Sine) SetFrequency(freq, sampleRate float64) {
	if sampleRate > 0 {
		s.inc = freq / sampleRate
	}
}

func (s *Sine) Phase() float64 { return s.phase }

func (s *Sine) Next() float32 {
	v := SineFast(s.phase)
	s.phase += s.inc
	if s.phase >= 1 {
		s.phase -= math.Floor(s.phase)
	}
	return v
}

func (s *Sine) Fill(dst []float32) {
	for i := range dst {
		dst[i] = s.Next()
	}
}

type Saw struct {
	phase float64
	inc   float64
}

func NewSaw() *Saw { return &Saw{} }

func (o *Saw) SetFrequency(freq, sampleRate float64) {
	if sampleRate > 0 {
		o.inc = freq / sampleRate
	}
}

func (o *Saw) Phase() float64 { return o.phase }

func (o *Saw) Next() float32 {
	v := 2*(o.phase-math.Floor(o.phase)) - 1
	o.phase += o.inc
	if o.phase >= 1 {
		o.phase -= math.Floor(o.phase)
	}
	return float32(v)
}

func (o *Saw) Fill(dst []float32) {
	for i := range dst {
		dst[i] = o.Next()
	}
}

type xorshift64 uint64

func (r *xorshift64) next() uint64 {
	x := uint64(*r)
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*r = xorshift64(x)
	return x
}

type Noise struct {
	rng xorshift64
}

func NewNoise(seed uint64) *Noise {
	if seed == 0 {
		seed = DefaultNoiseSeed
	}
	return &Noise{rng: xorshift64(seed)}
}

func (n *Noise) Next() float32 {
	return float32(float64(n.rng.next()>>40)*(1.0/8388608.0) - 1.0)
}

func (n *Noise) Fill(dst []float32) {
	for i := range dst {
		dst[i] = n.Next()
	}
}
