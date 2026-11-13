package dsp

import (
	"math"
	"strings"
)

const MaxDelaySeconds = 4.0

const DefaultDelayDampingHz = 4000.0

type DelayMode int

const (
	DelayModeStereo DelayMode = iota

	DelayModePingPong
)

type Subdivision float64

const (
	SubdivisionWhole Subdivision = 4.0

	SubdivisionHalf Subdivision = 2.0

	SubdivisionQuarter Subdivision = 1.0

	SubdivisionEighth Subdivision = 0.5

	SubdivisionEighthD Subdivision = 0.75

	SubdivisionSixteenth Subdivision = 0.25

	SubdivisionSixteenthT Subdivision = 1.0 / 3.0
)

func ParseSubdivision(s string) (Subdivision, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1/1", "whole":
		return SubdivisionWhole, true
	case "1/2", "half":
		return SubdivisionHalf, true
	case "1/4", "quarter":
		return SubdivisionQuarter, true
	case "1/8", "eighth":
		return SubdivisionEighth, true
	case "1/8d", "1/8.", "dotted-eighth", "eighthd":
		return SubdivisionEighthD, true
	case "1/16", "sixteenth":
		return SubdivisionSixteenth, true
	case "1/16t", "sixteentht", "1/16t.", "triplet":
		return SubdivisionSixteenthT, true
	}
	return 0, false
}

func DelaySamplesFor(bpm float64, sub Subdivision, sampleRate int) int {
	if !(bpm > 0) || sampleRate <= 0 || !(sub > 0) {
		return 0
	}
	secs := 60.0 / bpm * float64(sub)
	return int(math.Round(secs * float64(sampleRate)))
}

func DampingCoeff(fc float32, sampleRate int) float32 {
	if sampleRate <= 0 {
		return 0
	}
	if fc <= 0 {
		return 0
	}
	nyquist := float64(sampleRate) / 2
	f := float64(fc)
	if f >= nyquist {
		f = math.Nextafter(nyquist, 0)
	}
	return float32(math.Exp(-2 * math.Pi * f / float64(sampleRate)))
}

type DelayConfig struct {
	Mode         DelayMode
	Subdivision  Subdivision
	DelaySeconds float64
	Feedback     float32
	DampingHz    float32
	Wet          float32
}

type StereoDelay struct {
	bufferL, bufferR []float32
	mask             int
	writeIdx         int
	lengthSamples    int
	fracDelay        float64

	feedback   float32
	dampCoeff  float32
	dampStateL float32
	dampStateR float32
	wet        float32
	mode       DelayMode

	sampleRate int
	maxSamples int
}

func NewStereoDelay(sampleRate int, maxSeconds float64) *StereoDelay {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if !(maxSeconds > 0) {
		maxSeconds = MaxDelaySeconds
	}
	n := int(float64(sampleRate)*maxSeconds) + 1
	if n < 2 {
		n = 2
	}
	size := 1
	for size < n {
		size <<= 1
	}
	return &StereoDelay{
		bufferL:    make([]float32, size),
		bufferR:    make([]float32, size),
		mask:       size - 1,
		sampleRate: sampleRate,
		maxSamples: size,
		wet:        0.4,
	}
}

func (d *StereoDelay) Configure(cfg DelayConfig, bpm float64) {
	fb := ClampF32(cfg.Feedback, 0, 0.98)
	wet := ClampF32(cfg.Wet, 0, 1)
	dampHz := cfg.DampingHz
	if dampHz <= 0 {
		dampHz = DefaultDelayDampingHz
	}
	length := 0
	fracLen := 0.0
	if cfg.DelaySeconds > 0 {
		fracLen = cfg.DelaySeconds * float64(d.sampleRate)
		length = int(math.Round(fracLen))
	} else if cfg.Subdivision > 0 && bpm > 0 {
		fracLen = 60.0 / bpm * float64(cfg.Subdivision) * float64(d.sampleRate)
		length = int(math.Round(fracLen))
	}
	if length < 0 {
		length = 0
	}
	if length >= d.maxSamples {
		length = d.maxSamples - 1
	}
	if fracLen < 0 {
		fracLen = 0
	}
	if fracLen >= float64(d.maxSamples) {
		fracLen = float64(d.maxSamples - 1)
	}
	d.lengthSamples = length
	d.fracDelay = fracLen
	d.feedback = fb
	d.wet = wet
	d.mode = cfg.Mode
	d.dampCoeff = DampingCoeff(dampHz, d.sampleRate)
}

func (d *StereoDelay) SetDelaySamples(n int) {
	if n < 0 {
		n = 0
	}
	if n >= d.maxSamples {
		n = d.maxSamples - 1
	}
	d.lengthSamples = n
	d.fracDelay = float64(n)
}

func (d *StereoDelay) SetDelayFractional(samples float64) {
	if samples < 0 {
		samples = 0
	}
	if samples >= float64(d.maxSamples) {
		samples = float64(d.maxSamples - 1)
	}
	d.fracDelay = samples
	d.lengthSamples = int(math.Round(samples))
}

func (d *StereoDelay) DelayFractional() float64 {
	if d.fracDelay > 0 {
		return d.fracDelay
	}
	return float64(d.lengthSamples)
}

func (d *StereoDelay) LengthSamples() int { return d.lengthSamples }

func (d *StereoDelay) MemoryBytes() int { return 2 * len(d.bufferL) * 4 }

func (d *StereoDelay) SampleRate() int { return d.sampleRate }

func (d *StereoDelay) Mode() DelayMode { return d.mode }

func (d *StereoDelay) Feedback() float32 { return d.feedback }

func (d *StereoDelay) Wet() float32 { return d.wet }

func (d *StereoDelay) DampingHz() float32 {
	if d.dampCoeff <= 0 {
		return 0
	}
	if d.dampCoeff >= 1 {
		return float32(d.sampleRate) / 2
	}
	return float32(-math.Log(float64(d.dampCoeff)) * float64(d.sampleRate) / (2 * math.Pi))
}

func (d *StereoDelay) Reset() {
	clear(d.bufferL)
	clear(d.bufferR)
	d.writeIdx = 0
	d.dampStateL = 0
	d.dampStateR = 0
}

func (d *StereoDelay) ProcessFrameWet(inL, inR float32) (float32, float32) {
	delay := d.fracDelay
	if delay <= 0 {
		delay = float64(d.lengthSamples)
	}
	if delay <= 0 {
		return 0, 0
	}
	if math.IsNaN(float64(inL)) || math.IsInf(float64(inL), 0) {
		inL = 0
	}
	if math.IsNaN(float64(inR)) || math.IsInf(float64(inR), 0) {
		inR = 0
	}
	if delay < 1.0 {
		delay = 1.0
	}
	maxValid := float64(d.mask)
	if delay > maxValid {
		delay = maxValid
	}
	intPart := int(delay)
	frac := float32(delay - float64(intPart))
	if intPart >= d.mask {
		intPart = d.mask
		frac = 0
	}
	readIdx0 := (d.writeIdx - intPart) & d.mask
	readIdx1 := (readIdx0 - 1) & d.mask
	rawL := d.bufferL[readIdx0]
	rawR := d.bufferR[readIdx0]
	if frac > 0 {
		rawL += frac * (d.bufferL[readIdx1] - rawL)
		rawR += frac * (d.bufferR[readIdx1] - rawR)
	}
	if math.IsNaN(float64(rawL)) || math.IsInf(float64(rawL), 0) {
		rawL = 0
	}
	if math.IsNaN(float64(rawR)) || math.IsInf(float64(rawR), 0) {
		rawR = 0
	}

	oneMinus := 1 - d.dampCoeff
	stateL := oneMinus*rawL + d.dampCoeff*d.dampStateL
	stateR := oneMinus*rawR + d.dampCoeff*d.dampStateR
	if math.IsNaN(float64(stateL)) || math.IsInf(float64(stateL), 0) || math.Abs(float64(stateL)) < 1e-25 {
		stateL = 0
	}
	if math.IsNaN(float64(stateR)) || math.IsInf(float64(stateR), 0) || math.Abs(float64(stateR)) < 1e-25 {
		stateR = 0
	}
	d.dampStateL, d.dampStateR = stateL, stateR
	wetL, wetR := stateL, stateR

	if d.mode == DelayModePingPong {
		d.bufferL[d.writeIdx] = inL + wetR*d.feedback
		d.bufferR[d.writeIdx] = inR + wetL*d.feedback
	} else {
		d.bufferL[d.writeIdx] = inL + wetL*d.feedback
		d.bufferR[d.writeIdx] = inR + wetR*d.feedback
	}
	d.writeIdx = (d.writeIdx + 1) & d.mask
	return wetL, wetR
}

func (d *StereoDelay) ProcessFrame(inL, inR float32) (float32, float32) {
	if d.lengthSamples <= 0 && d.fracDelay <= 0 {
		return inL, inR
	}
	wetL, wetR := d.ProcessFrameWet(inL, inR)
	return inL*(1-d.wet) + wetL*d.wet, inR*(1-d.wet) + wetR*d.wet
}

func (d *StereoDelay) ProcessStereo(buf *StereoBuffer) {
	if buf == nil {
		return
	}
	left, right := buf.Left, buf.Right
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		left[i], right[i] = d.ProcessFrame(left[i], right[i])
	}
}

func (d *StereoDelay) ProcessInterleaved(buf []float32) {
	for i := 0; i+1 < len(buf); i += 2 {
		buf[i], buf[i+1] = d.ProcessFrame(buf[i], buf[i+1])
	}
}
