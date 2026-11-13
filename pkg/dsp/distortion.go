package dsp

import (
	"math"
	"strings"
)

type DistortionMode int

const (
	DistortionDiode DistortionMode = iota
	DistortionTube
	DistortionHardClip
	DistortionFoldback
)

func ParseDistortionMode(s string) DistortionMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "diode":
		return DistortionDiode
	case "hardclip", "hard_clip", "hard":
		return DistortionHardClip
	case "foldback", "fold", "wavefold":
		return DistortionFoldback
	default:
		return DistortionTube
	}
}

func (m DistortionMode) String() string {
	switch m {
	case DistortionDiode:
		return "diode"
	case DistortionHardClip:
		return "hardclip"
	case DistortionFoldback:
		return "foldback"
	default:
		return "tube"
	}
}

type DistortionConfig struct {
	Mode         DistortionMode
	Drive        float32
	Tone         float32
	Mix          float32
	OutputGainDB float32
}

func DefaultDistortionConfig() DistortionConfig {
	return DistortionConfig{
		Mode:         DistortionTube,
		Drive:        1.0,
		Tone:         10000.0,
		Mix:          1.0,
		OutputGainDB: 0.0,
	}
}

type Distortion struct {
	sampleRate float64
	cfg        DistortionConfig

	outGainLin float32
	toneAlpha  float32
	toneYL     float32
	toneYR     float32

	ringL        [32]float32
	ringR        [32]float32
	ringPos      int
	decRingEvenL [32]float32
	decRingEvenR [32]float32
	decRingOddL  [32]float32
	decRingOddR  [32]float32
	decPos       int
}

func NewDistortion(sampleRate float64, cfg DistortionConfig) *Distortion {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	d := &Distortion{sampleRate: sampleRate}
	d.SetConfig(cfg)
	return d
}

func (d *Distortion) SetConfig(cfg DistortionConfig) {
	if cfg.Drive < 0.1 {
		cfg.Drive = 0.1
	} else if cfg.Drive > 50.0 {
		cfg.Drive = 50.0
	}
	if cfg.Mix < 0 {
		cfg.Mix = 0
	} else if cfg.Mix > 1 {
		cfg.Mix = 1
	}
	if cfg.Tone < 0 {
		cfg.Tone = 0
	}
	d.cfg = cfg
	d.outGainLin = float32(math.Pow(10.0, float64(cfg.OutputGainDB)/20.0))

	var cutoffHz float64
	if cfg.Tone <= 0 {
		cutoffHz = 20000.0
	} else if cfg.Tone <= 1.0 {
		cutoffHz = 200.0 + float64(cfg.Tone)*19800.0
	} else {
		cutoffHz = float64(cfg.Tone)
	}
	if cutoffHz < 200.0 {
		cutoffHz = 200.0
	} else if cutoffHz > d.sampleRate*0.48 {
		cutoffHz = d.sampleRate * 0.48
	}
	d.toneAlpha = float32(1.0 - math.Exp(-2.0*math.Pi*cutoffHz/d.sampleRate))
}

func (d *Distortion) Config() DistortionConfig {
	return d.cfg
}

func (d *Distortion) SetDrive(drive float32) {
	d.cfg.Drive = drive
	d.SetConfig(d.cfg)
}

func (d *Distortion) Drive() float32 {
	return d.cfg.Drive
}

func (d *Distortion) SetMix(mix float32) {
	d.cfg.Mix = mix
	d.SetConfig(d.cfg)
}

func (d *Distortion) Mix() float32 {
	return d.cfg.Mix
}

func (d *Distortion) SetMode(mode DistortionMode) {
	d.cfg.Mode = mode
}

func (d *Distortion) Mode() DistortionMode {
	return d.cfg.Mode
}

func (d *Distortion) SetTone(tone float32) {
	d.cfg.Tone = tone
	d.SetConfig(d.cfg)
}

func (d *Distortion) Tone() float32 {
	return d.cfg.Tone
}

func (d *Distortion) Reset() {
	clear(d.ringL[:])
	clear(d.ringR[:])
	clear(d.decRingEvenL[:])
	clear(d.decRingEvenR[:])
	clear(d.decRingOddL[:])
	clear(d.decRingOddR[:])
	d.ringPos = 0
	d.decPos = 0
	d.toneYL = 0
	d.toneYR = 0
}

func shapeSample(x float32, mode DistortionMode) float32 {
	if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
		return 0
	}
	switch mode {
	case DistortionDiode:
		d := float64(x)
		if d > 0.5 {
			return float32(0.5 + 0.5*(1.0-math.Exp(-(d-0.5)/0.5)))
		} else if d < -0.8 {
			return float32(-0.8 - 0.2*(1.0-math.Exp(-(-d-0.8)/0.4)))
		}
		return x
	case DistortionHardClip:
		if x > 1.0 {
			return 1.0
		}
		if x < -1.0 {
			return -1.0
		}
		return x
	case DistortionFoldback:
		norm := float64(x+1.0) * 0.25
		frac := norm - math.Floor(norm)
		return float32(4.0*math.Abs(frac-0.5) - 1.0)
	default:
		d := float64(x)
		var y float64
		if d >= 0 {
			y = math.Tanh(d) + 0.15*d/(1.0+0.5*d*d)
		} else {
			y = math.Tanh(d*1.2) * 0.85
		}
		return float32(y * 0.8)
	}
}

func (d *Distortion) ProcessFrame(inL, inR float32) (float32, float32) {
	if inL > -1e-25 && inL < 1e-25 {
		inL = 0
	}
	if inR > -1e-25 && inR < 1e-25 {
		inR = 0
	}
	if math.IsNaN(float64(inL)) || math.IsInf(float64(inL), 0) {
		inL = 0
	}
	if math.IsNaN(float64(inR)) || math.IsInf(float64(inR), 0) {
		inR = 0
	}

	m := 31
	p := d.ringPos
	d.ringL[p] = inL
	d.ringR[p] = inR

	evenL := d.ringL[(p-7)&m]
	evenR := d.ringR[(p-7)&m]

	var oddL, oddR float32
	oddL += halfBandCoeffs[0] * (d.ringL[(p-6)&m] + d.ringL[(p-7)&m])
	oddR += halfBandCoeffs[0] * (d.ringR[(p-6)&m] + d.ringR[(p-7)&m])

	oddL += halfBandCoeffs[1] * (d.ringL[(p-5)&m] + d.ringL[(p-8)&m])
	oddR += halfBandCoeffs[1] * (d.ringR[(p-5)&m] + d.ringR[(p-8)&m])

	oddL += halfBandCoeffs[2] * (d.ringL[(p-4)&m] + d.ringL[(p-9)&m])
	oddR += halfBandCoeffs[2] * (d.ringR[(p-4)&m] + d.ringR[(p-9)&m])

	oddL += halfBandCoeffs[3] * (d.ringL[(p-3)&m] + d.ringL[(p-10)&m])
	oddR += halfBandCoeffs[3] * (d.ringR[(p-3)&m] + d.ringR[(p-10)&m])

	oddL += halfBandCoeffs[4] * (d.ringL[(p-2)&m] + d.ringL[(p-11)&m])
	oddR += halfBandCoeffs[4] * (d.ringR[(p-2)&m] + d.ringR[(p-11)&m])

	oddL += halfBandCoeffs[5] * (d.ringL[(p-1)&m] + d.ringL[(p-12)&m])
	oddR += halfBandCoeffs[5] * (d.ringR[(p-1)&m] + d.ringR[(p-12)&m])

	oddL += halfBandCoeffs[6] * (d.ringL[p&m] + d.ringL[(p-13)&m])
	oddR += halfBandCoeffs[6] * (d.ringR[p&m] + d.ringR[(p-13)&m])

	d.ringPos = (p + 1) & m

	drv := d.cfg.Drive
	mode := d.cfg.Mode
	satEvenL := shapeSample(evenL*drv, mode)
	satEvenR := shapeSample(evenR*drv, mode)
	satOddL := shapeSample(oddL*drv, mode)
	satOddR := shapeSample(oddR*drv, mode)

	dp := d.decPos
	d.decRingEvenL[dp] = satEvenL
	d.decRingEvenR[dp] = satEvenR
	d.decRingOddL[dp] = satOddL
	d.decRingOddR[dp] = satOddR

	evenValL := d.decRingEvenL[(dp-7)&m]
	evenValR := d.decRingEvenR[(dp-7)&m]

	var decOddL, decOddR float32
	decOddL += halfBandCoeffs[0] * (d.decRingOddL[(dp-7)&m] + d.decRingOddL[(dp-8)&m])
	decOddR += halfBandCoeffs[0] * (d.decRingOddR[(dp-7)&m] + d.decRingOddR[(dp-8)&m])

	decOddL += halfBandCoeffs[1] * (d.decRingOddL[(dp-6)&m] + d.decRingOddL[(dp-9)&m])
	decOddR += halfBandCoeffs[1] * (d.decRingOddR[(dp-6)&m] + d.decRingOddR[(dp-9)&m])

	decOddL += halfBandCoeffs[2] * (d.decRingOddL[(dp-5)&m] + d.decRingOddL[(dp-10)&m])
	decOddR += halfBandCoeffs[2] * (d.decRingOddR[(dp-5)&m] + d.decRingOddR[(dp-10)&m])

	decOddL += halfBandCoeffs[3] * (d.decRingOddL[(dp-4)&m] + d.decRingOddL[(dp-11)&m])
	decOddR += halfBandCoeffs[3] * (d.decRingOddR[(dp-4)&m] + d.decRingOddR[(dp-11)&m])

	decOddL += halfBandCoeffs[4] * (d.decRingOddL[(dp-3)&m] + d.decRingOddL[(dp-12)&m])
	decOddR += halfBandCoeffs[4] * (d.decRingOddR[(dp-3)&m] + d.decRingOddR[(dp-12)&m])

	decOddL += halfBandCoeffs[5] * (d.decRingOddL[(dp-2)&m] + d.decRingOddL[(dp-13)&m])
	decOddR += halfBandCoeffs[5] * (d.decRingOddR[(dp-2)&m] + d.decRingOddR[(dp-13)&m])

	decOddL += halfBandCoeffs[6] * (d.decRingOddL[(dp-1)&m] + d.decRingOddL[(dp-14)&m])
	decOddR += halfBandCoeffs[6] * (d.decRingOddR[(dp-1)&m] + d.decRingOddR[(dp-14)&m])

	d.decPos = (dp + 1) & m

	wetL := 0.5 * (evenValL + decOddL)
	wetR := 0.5 * (evenValR + decOddR)

	d.toneYL += d.toneAlpha * (wetL - d.toneYL)
	d.toneYR += d.toneAlpha * (wetR - d.toneYR)
	if d.toneYL > -1e-25 && d.toneYL < 1e-25 {
		d.toneYL = 0
	}
	if d.toneYR > -1e-25 && d.toneYR < 1e-25 {
		d.toneYR = 0
	}

	wetL = d.toneYL * d.outGainLin
	wetR = d.toneYR * d.outGainLin

	mix := d.cfg.Mix
	outL := (1.0-mix)*inL + mix*wetL
	outR := (1.0-mix)*inR + mix*wetR

	if math.IsNaN(float64(outL)) || math.IsInf(float64(outL), 0) {
		outL = 0
	}
	if math.IsNaN(float64(outR)) || math.IsInf(float64(outR), 0) {
		outR = 0
	}

	return outL, outR
}

func (d *Distortion) ProcessInterleaved(buffer []float32) {
	frames := len(buffer) / 2
	if frames == 0 {
		return
	}
	_ = buffer[2*frames-1]
	for i := 0; i < frames; i++ {
		oL, oR := d.ProcessFrame(buffer[2*i], buffer[2*i+1])
		buffer[2*i] = oL
		buffer[2*i+1] = oR
	}
}

func (d *Distortion) ProcessStereo(left, right []float32) {
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
		oL, oR := d.ProcessFrame(left[i], right[i])
		left[i] = oL
		right[i] = oR
	}
}

func (d *Distortion) ProcessSlice(buffer []float32) {
	n := len(buffer)
	if n == 0 {
		return
	}
	_ = buffer[n-1]
	for i := 0; i < n; i++ {
		oL, _ := d.ProcessFrame(buffer[i], 0)
		buffer[i] = oL
	}
}
