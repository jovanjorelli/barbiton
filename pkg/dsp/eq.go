package dsp

import "math"

type EQBandSpec struct {
	Freq   float32
	GainDB float32
	Q      float32
}

type EQSpec struct {
	HPF       float32
	LowShelf  EQBandSpec
	MidPeak   EQBandSpec
	HighShelf EQBandSpec
}

const (
	eqDefaultShelfQ = 0.7071067811865476
	eqDefaultPeakQ  = 1.0
)

type EQ struct {
	sampleRate                      float64
	spec                            EQSpec
	hpf                             Biquad
	low                             Biquad
	mid                             Biquad
	high                            Biquad
	useHPF, useLow, useMid, useHigh bool
}

func NewEQ(sampleRate float64, spec EQSpec) *EQ {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	e := &EQ{
		sampleRate: sampleRate,
		hpf:        Biquad{mode: Highpass, sampleRate: sampleRate},
		low:        Biquad{mode: LowShelf, sampleRate: sampleRate},
		mid:        Biquad{mode: Peaking, sampleRate: sampleRate},
		high:       Biquad{mode: HighShelf, sampleRate: sampleRate},
	}
	e.SetSpec(spec)
	return e
}

func (e *EQ) SetSpec(spec EQSpec) {
	e.spec = spec

	e.useHPF = spec.HPF > 0
	if e.useHPF {
		e.updateBand(&e.hpf, Highpass, float64(spec.HPF), eqDefaultShelfQ, 0)
	}
	e.useLow = spec.LowShelf.Freq > 0
	if e.useLow {
		e.updateBand(&e.low, LowShelf, float64(spec.LowShelf.Freq), bandQ(spec.LowShelf.Q, eqDefaultShelfQ), float64(spec.LowShelf.GainDB))
	}
	e.useMid = spec.MidPeak.Freq > 0
	if e.useMid {
		e.updateBand(&e.mid, Peaking, float64(spec.MidPeak.Freq), bandQ(spec.MidPeak.Q, eqDefaultPeakQ), float64(spec.MidPeak.GainDB))
	}
	e.useHigh = spec.HighShelf.Freq > 0
	if e.useHigh {
		e.updateBand(&e.high, HighShelf, float64(spec.HighShelf.Freq), bandQ(spec.HighShelf.Q, eqDefaultShelfQ), float64(spec.HighShelf.GainDB))
	}
}

func (e *EQ) updateBand(b *Biquad, mode FilterMode, freq, q, gainDB float64) {
	b.mode = mode
	b.sampleRate = e.sampleRate
	if math.IsNaN(q) || math.IsInf(q, 0) || q <= 0 {
		q = defaultQ
	}
	b.q = q
	if math.IsNaN(gainDB) || math.IsInf(gainDB, 0) {
		gainDB = 0
	}
	b.gainDB = gainDB
	if math.IsNaN(freq) || math.IsInf(freq, 0) || freq <= 0 {
		freq = 1e-3
	}
	nyquist := e.sampleRate / 2
	if freq >= nyquist {
		freq = math.Nextafter(nyquist, 0)
	}
	b.freq = freq
	b.computeCoeffs()
}

func (e *EQ) Spec() EQSpec { return e.spec }

func (e *EQ) Bands() int {
	n := 0
	for _, on := range []bool{e.useHPF, e.useLow, e.useMid, e.useHigh} {
		if on {
			n++
		}
	}
	return n
}

func (e *EQ) ProcessSample(x float32) float32 {
	if e.useHPF {
		x = e.hpf.ProcessSample(x)
	}
	if e.useLow {
		x = e.low.ProcessSample(x)
	}
	if e.useMid {
		x = e.mid.ProcessSample(x)
	}
	if e.useHigh {
		x = e.high.ProcessSample(x)
	}
	return x
}

func (e *EQ) ProcessInPlace(b []float32) {
	if e.useHPF {
		e.hpf.ProcessInPlace(b)
	}
	if e.useLow {
		e.low.ProcessInPlace(b)
	}
	if e.useMid {
		e.mid.ProcessInPlace(b)
	}
	if e.useHigh {
		e.high.ProcessInPlace(b)
	}
}

func (e *EQ) Process(dst, src []float32) {
	if e.useHPF {
		e.hpf.Process(dst, src)
	}

	if !e.useHPF {
		copyN(dst, src)
	}
	if e.useLow {
		e.low.ProcessInPlace(dst)
	}
	if e.useMid {
		e.mid.ProcessInPlace(dst)
	}
	if e.useHigh {
		e.high.ProcessInPlace(dst)
	}
}

func (e *EQ) Reset() {
	e.hpf.Reset()
	e.low.Reset()
	e.mid.Reset()
	e.high.Reset()
}

func (e *EQ) ResponseAt(freq float64) float64 {
	r := 1.0
	if e.useHPF {
		r *= e.hpf.ResponseAt(freq)
	}
	if e.useLow {
		r *= e.low.ResponseAt(freq)
	}
	if e.useMid {
		r *= e.mid.ResponseAt(freq)
	}
	if e.useHigh {
		r *= e.high.ResponseAt(freq)
	}
	return r
}

func (f *Biquad) ResponseAt(freq float64) float64 {
	w := twoPi * freq / f.sampleRate
	c1, s1 := math.Cos(w), math.Sin(w)
	c2, s2 := math.Cos(2*w), math.Sin(2*w)
	numR := f.b0 + f.b1*c1 + f.b2*c2
	numI := -(f.b1*s1 + f.b2*s2)
	denR := 1 + f.a1*c1 + f.a2*c2
	denI := -(f.a1*s1 + f.a2*s2)
	return math.Hypot(numR, numI) / math.Hypot(denR, denI)
}

func bandQ(q float32, fallback float64) float64 {
	if q <= 0 {
		return fallback
	}
	return float64(q)
}

func copyN(dst, src []float32) {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	copy(dst[:n], src[:n])
}
