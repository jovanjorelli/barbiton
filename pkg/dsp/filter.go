package dsp

import "math"

type FilterMode uint8

const (
	Lowpass FilterMode = iota

	Highpass

	Bandpass

	Peaking

	LowShelf

	HighShelf
	Notch
	Lowpass1P
	Highpass1P
)

const defaultQ = 0.7071067811865476

type Biquad struct {
	mode       FilterMode
	freq       float64
	q          float64
	gainDB     float64
	sampleRate float64

	b0, b1, b2, a1, a2 float64

	x1, x2, y1, y2 float64
}

func NewBiquad(mode FilterMode, freq, q, sampleRate float64) *Biquad {
	return NewBiquadGain(mode, freq, q, 0, sampleRate)
}

func NewBiquadGain(mode FilterMode, freq, q, gainDB, sampleRate float64) *Biquad {
	if math.IsNaN(q) || math.IsInf(q, 0) || q <= 0 {
		q = defaultQ
	}
	if math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) || sampleRate <= 0 {
		sampleRate = 48000
	}
	if math.IsNaN(gainDB) || math.IsInf(gainDB, 0) {
		gainDB = 0
	}
	f := &Biquad{mode: mode, q: q, gainDB: gainDB, sampleRate: sampleRate}
	f.SetFrequency(freq)
	return f
}

func (f *Biquad) SetGainDB(db float64) {
	if math.IsNaN(db) || math.IsInf(db, 0) {
		db = 0
	}
	f.gainDB = db
	f.computeCoeffs()
}

func (f *Biquad) GainDB() float64 { return f.gainDB }

func (f *Biquad) SetFrequency(freq float64) {
	if math.IsNaN(freq) || math.IsInf(freq, 0) || freq <= 0 {
		freq = 1e-3
	}
	nyquist := f.sampleRate / 2
	if freq >= nyquist {
		freq = math.Nextafter(nyquist, 0)
	}
	f.freq = freq
	f.computeCoeffs()
}

func (f *Biquad) SetQ(q float64) {
	if math.IsNaN(q) || math.IsInf(q, 0) || q <= 0 {
		q = defaultQ
	}
	f.q = q
	f.computeCoeffs()
}

func (f *Biquad) Frequency() float64 { return f.freq }

func (f *Biquad) Q() float64 { return f.q }

func (f *Biquad) Reset() {
	f.x1, f.x2, f.y1, f.y2 = 0, 0, 0, 0
}

func (f *Biquad) SetMode(mode FilterMode) {
	f.mode = mode
	f.computeCoeffs()
}

func (f *Biquad) SetSampleRate(sr float64) {
	if math.IsNaN(sr) || math.IsInf(sr, 0) || sr <= 0 {
		sr = 48000
	}
	f.sampleRate = sr
	f.computeCoeffs()
}

func (f *Biquad) computeCoeffs() {
	w0 := twoPi * f.freq / f.sampleRate
	sin, cos := math.Sincos(w0)
	alpha := sin / (2 * f.q)
	var b0, b1, b2, a0, a1, a2 float64
	switch f.mode {
	case Highpass:
		b0 = (1 + cos) / 2
		b1 = -(1 + cos)
		b2 = (1 + cos) / 2
		a0, a1, a2 = 1+alpha, -2*cos, 1-alpha
	case Bandpass:
		b0, b1, b2 = alpha, 0, -alpha
		a0, a1, a2 = 1+alpha, -2*cos, 1-alpha
	case Peaking:
		A := math.Pow(10, f.gainDB/40)
		b0 = 1 + alpha*A
		b1 = -2 * cos
		b2 = 1 - alpha*A
		a0 = 1 + alpha/A
		a1 = -2 * cos
		a2 = 1 - alpha/A
	case LowShelf:
		A := math.Pow(10, f.gainDB/40)
		twoSqrtAAlpha := 2 * math.Sqrt(A) * alpha
		b0 = A * ((A + 1) - (A-1)*cos + twoSqrtAAlpha)
		b1 = 2 * A * ((A - 1) - (A+1)*cos)
		b2 = A * ((A + 1) - (A-1)*cos - twoSqrtAAlpha)
		a0 = (A + 1) + (A-1)*cos + twoSqrtAAlpha
		a1 = -2 * ((A - 1) + (A+1)*cos)
		a2 = (A + 1) + (A-1)*cos - twoSqrtAAlpha
	case HighShelf:
		A := math.Pow(10, f.gainDB/40)
		twoSqrtAAlpha := 2 * math.Sqrt(A) * alpha
		b0 = A * ((A + 1) + (A-1)*cos + twoSqrtAAlpha)
		b1 = -2 * A * ((A - 1) + (A+1)*cos)
		b2 = A * ((A + 1) + (A-1)*cos - twoSqrtAAlpha)
		a0 = (A + 1) - (A-1)*cos + twoSqrtAAlpha
		a1 = 2 * ((A - 1) - (A+1)*cos)
		a2 = (A + 1) - (A-1)*cos - twoSqrtAAlpha
	case Notch:
		b0 = 1
		b1 = -2 * cos
		b2 = 1
		a0, a1, a2 = 1+alpha, -2*cos, 1-alpha
	case Lowpass1P:
		k := math.Tan(w0 / 2)
		b0 = k
		b1 = k
		b2 = 0
		a0, a1, a2 = 1+k, k-1, 0
	case Highpass1P:
		k := math.Tan(w0 / 2)
		b0 = 1
		b1 = -1
		b2 = 0
		a0, a1, a2 = 1+k, k-1, 0
	default:
		b0 = (1 - cos) / 2
		b1 = 1 - cos
		b2 = (1 - cos) / 2
		a0, a1, a2 = 1+alpha, -2*cos, 1-alpha
	}
	f.b0 = b0 / a0
	f.b1 = b1 / a0
	f.b2 = b2 / a0
	f.a1 = a1 / a0
	f.a2 = a2 / a0
}

func (f *Biquad) ProcessSample(x float32) float32 {
	if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
		x = 0
	}
	xi := float64(x)
	y := f.b0*xi + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	if math.IsNaN(y) || math.IsInf(y, 0) {
		y = 0
		f.x1, f.x2, f.y1, f.y2 = 0, 0, 0, 0
	} else if math.Abs(y) < 1e-25 {
		y = 0
	}
	f.x2, f.x1 = f.x1, xi
	f.y2, f.y1 = f.y1, y
	return float32(y)
}

func (f *Biquad) Process(dst, src []float32) {
	for i, in := range src {
		y := f.ProcessSample(in)
		if i < len(dst) {
			dst[i] = y
		}
	}
}

func (f *Biquad) ProcessInPlace(b []float32) {
	f.Process(b, b)
}

const (
	butterworth4PQ1 = 0.541196100146197
	butterworth4PQ2 = 1.3065629648763765
	butterworth6PQ1 = 0.5176380902050415
	butterworth6PQ2 = 0.7071067811865476
	butterworth6PQ3 = 1.9318516525781366
)

type CascadeFilter struct {
	stages     [3]Biquad
	stageCount int
	mode       FilterMode
	freq       float64
	sampleRate float64
}

func NewCascadeFilter(stages int, mode FilterMode, freq, sampleRate float64) *CascadeFilter {
	if sampleRate <= 0 || math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) {
		sampleRate = 48000
	}
	c := &CascadeFilter{sampleRate: sampleRate}
	c.SetButterworth(stages, mode, freq, 0, sampleRate)
	return c
}

func (c *CascadeFilter) StageCount() int {
	return c.stageCount
}

func (c *CascadeFilter) SetStages(stages int) {
	if stages <= 1 {
		stages = 1
	} else if stages > 3 {
		stages = 3
	}
	c.stageCount = stages
}

func (c *CascadeFilter) Stage(i int) *Biquad {
	if i >= 0 && i < c.stageCount {
		return &c.stages[i]
	}
	return nil
}

func (c *CascadeFilter) Frequency() float64 {
	return c.freq
}

func (c *CascadeFilter) Mode() FilterMode {
	return c.mode
}

func (c *CascadeFilter) SampleRate() float64 {
	return c.sampleRate
}

func (c *CascadeFilter) SetSampleRate(sr float64) {
	if math.IsNaN(sr) || math.IsInf(sr, 0) || sr <= 0 {
		sr = 48000
	}
	c.sampleRate = sr
	for i := 0; i < c.stageCount; i++ {
		c.stages[i].SetSampleRate(sr)
	}
}

func (c *CascadeFilter) SetMode(mode FilterMode) {
	c.mode = mode
	for i := 0; i < c.stageCount; i++ {
		c.stages[i].SetMode(mode)
	}
}

func (c *CascadeFilter) SetFrequency(freq float64) {
	c.freq = freq
	for i := 0; i < c.stageCount; i++ {
		c.stages[i].SetFrequency(freq)
	}
}

func (c *CascadeFilter) SetQ(q float64) {
	for i := 0; i < c.stageCount; i++ {
		c.stages[i].SetQ(q)
	}
}

func (c *CascadeFilter) SetStageQ(stage int, q float64) {
	if stage >= 0 && stage < c.stageCount {
		c.stages[stage].SetQ(q)
	}
}

func (c *CascadeFilter) SetButterworth(stages int, mode FilterMode, freq, resonanceDB, sampleRate float64) {
	if stages <= 1 {
		stages = 1
	} else if stages > 3 {
		stages = 3
	}
	if sampleRate <= 0 || math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) {
		sampleRate = 48000
	}
	c.stageCount = stages
	c.mode = mode
	c.freq = freq
	c.sampleRate = sampleRate
	resGain := 1.0
	if !math.IsNaN(resonanceDB) && !math.IsInf(resonanceDB, 0) && resonanceDB > 0 {
		resGain = math.Pow(10, resonanceDB/20)
	}
	switch stages {
	case 2:
		qs := [2]float64{butterworth4PQ1, butterworth4PQ2}
		for i := 0; i < 2; i++ {
			c.stages[i].sampleRate = sampleRate
			c.stages[i].mode = mode
			c.stages[i].q = qs[i] * resGain
			c.stages[i].gainDB = 0
			c.stages[i].SetFrequency(freq)
		}
	case 3:
		qs := [3]float64{butterworth6PQ1, butterworth6PQ2, butterworth6PQ3}
		for i := 0; i < 3; i++ {
			c.stages[i].sampleRate = sampleRate
			c.stages[i].mode = mode
			c.stages[i].q = qs[i] * resGain
			c.stages[i].gainDB = 0
			c.stages[i].SetFrequency(freq)
		}
	default:
		c.stages[0].sampleRate = sampleRate
		c.stages[0].mode = mode
		c.stages[0].q = defaultQ * resGain
		c.stages[0].gainDB = 0
		c.stages[0].SetFrequency(freq)
	}
}

func (c *CascadeFilter) Reset() {
	for i := range c.stages {
		c.stages[i].Reset()
	}
}

func (c *CascadeFilter) ProcessSample(x float32) float32 {
	y := x
	for i := 0; i < c.stageCount; i++ {
		y = c.stages[i].ProcessSample(y)
	}
	return y
}

func (c *CascadeFilter) Process(dst, src []float32) {
	if c.stageCount <= 1 {
		c.stages[0].Process(dst, src)
		return
	}
	c.stages[0].Process(dst, src)
	for i := 1; i < c.stageCount; i++ {
		c.stages[i].ProcessInPlace(dst)
	}
}

func (c *CascadeFilter) ProcessInPlace(b []float32) {
	for i := 0; i < c.stageCount; i++ {
		c.stages[i].ProcessInPlace(b)
	}
}

func (c *CascadeFilter) ResponseAt(freq float64) float64 {
	resp := 1.0
	for i := 0; i < c.stageCount; i++ {
		resp *= c.stages[i].ResponseAt(freq)
	}
	return resp
}
