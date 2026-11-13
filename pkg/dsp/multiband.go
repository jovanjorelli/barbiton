package dsp

import (
	"math"
)

type MultibandBandConfig struct {
	ThresholdDB float32
	Ratio       float32
	AttackMs    float32
	ReleaseMs   float32
	KneeDB      float32
	AutoMakeup  bool
	MakeupDB    float32
	Saturation  float32
	OptoMode    bool
}

type MultibandConfig struct {
	LowMidFreq  float32
	MidHighFreq float32
	LowBand     MultibandBandConfig
	MidBand     MultibandBandConfig
	HighBand    MultibandBandConfig
	Bypass      bool
}

func DefaultMultibandConfig() MultibandConfig {
	return MultibandConfig{
		LowMidFreq:  120.0,
		MidHighFreq: 3500.0,
		LowBand: MultibandBandConfig{
			ThresholdDB: -18.0,
			Ratio:       3.0,
			AttackMs:    30.0,
			ReleaseMs:   150.0,
			KneeDB:      6.0,
			AutoMakeup:  true,
			MakeupDB:    0.0,
			Saturation:  0.05,
			OptoMode:    true,
		},
		MidBand: MultibandBandConfig{
			ThresholdDB: -15.0,
			Ratio:       2.5,
			AttackMs:    20.0,
			ReleaseMs:   100.0,
			KneeDB:      6.0,
			AutoMakeup:  true,
			MakeupDB:    0.0,
			Saturation:  0.02,
			OptoMode:    false,
		},
		HighBand: MultibandBandConfig{
			ThresholdDB: -12.0,
			Ratio:       2.0,
			AttackMs:    10.0,
			ReleaseMs:   80.0,
			KneeDB:      6.0,
			AutoMakeup:  true,
			MakeupDB:    0.0,
			Saturation:  0.0,
			OptoMode:    false,
		},
		Bypass: false,
	}
}

type lrBiquad struct {
	b0, b1, b2, a1, a2 float32
	x1, x2, y1, y2     float32
}

func (b *lrBiquad) ProcessSample(x float32) float32 {
	if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
		x = 0
	}
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	if math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) {
		y = 0
		b.x1, b.x2, b.y1, b.y2 = 0, 0, 0, 0
	} else if y > -1e-25 && y < 1e-25 {
		y = 0
	}
	b.x2, b.x1 = b.x1, x
	b.y2, b.y1 = b.y1, y
	return y
}

func (b *lrBiquad) Reset() {
	b.x1, b.x2, b.y1, b.y2 = 0, 0, 0, 0
}

type lrCascade struct {
	stage1 lrBiquad
	stage2 lrBiquad
}

func (c *lrCascade) ProcessSample(x float32) float32 {
	return c.stage2.ProcessSample(c.stage1.ProcessSample(x))
}

func (c *lrCascade) Reset() {
	c.stage1.Reset()
	c.stage2.Reset()
}

type bandCompressor struct {
	cfg         MultibandBandConfig
	sampleRate  float64
	envGRDB     float32
	alphaAttack float32
	alphaRel    float32
	makeupLin   float32
	autoMakeup  float32
}

func (bc *bandCompressor) Init(cfg MultibandBandConfig, sampleRate float64) {
	if cfg.Ratio < 1 {
		cfg.Ratio = 1
	}
	if cfg.AttackMs < 0.1 {
		cfg.AttackMs = 0.1
	}
	if cfg.ReleaseMs < 1 {
		cfg.ReleaseMs = 1
	}
	if cfg.KneeDB < 0 {
		cfg.KneeDB = 0
	}
	bc.cfg = cfg
	bc.sampleRate = sampleRate

	attSec := float64(cfg.AttackMs) * 0.001
	relSec := float64(cfg.ReleaseMs) * 0.001
	bc.alphaAttack = float32(1.0 - math.Exp(-1.0/(attSec*sampleRate)))
	bc.alphaRel = float32(1.0 - math.Exp(-1.0/(relSec*sampleRate)))

	if cfg.AutoMakeup && cfg.ThresholdDB < 0 {
		bc.autoMakeup = -cfg.ThresholdDB * (1.0 - 1.0/cfg.Ratio) * 0.5
	} else {
		bc.autoMakeup = 0
	}
	bc.makeupLin = float32(math.Pow(10, float64(cfg.MakeupDB+bc.autoMakeup)/20.0))
}

func (bc *bandCompressor) Reset() {
	bc.envGRDB = 0
}

func (bc *bandCompressor) Process(l, r float32) (float32, float32) {
	peak := l
	if peak < 0 {
		peak = -peak
	}
	pR := r
	if pR < 0 {
		pR = -pR
	}
	if pR > peak {
		peak = pR
	}
	if peak < 1e-25 {
		peak = 0
	}

	var peakDB float32
	if peak > 1e-6 {
		peakDB = float32(20.0 * math.Log10(float64(peak)))
	} else {
		peakDB = -120.0
	}

	thresh := bc.cfg.ThresholdDB
	knee := bc.cfg.KneeDB
	ratio := bc.cfg.Ratio
	invRatio := 1.0 / ratio

	var targetGR float32
	halfKnee := knee * 0.5
	if knee > 0 && peakDB > thresh-halfKnee && peakDB < thresh+halfKnee {
		excess := peakDB - thresh + halfKnee
		targetGR = ((1.0 - invRatio) / (2.0 * knee)) * excess * excess
	} else if peakDB >= thresh+halfKnee {
		targetGR = (peakDB - thresh) * (1.0 - invRatio)
	} else {
		targetGR = 0
	}

	if targetGR > bc.envGRDB {
		bc.envGRDB += bc.alphaAttack * (targetGR - bc.envGRDB)
	} else {
		if bc.cfg.OptoMode {
			scale := bc.envGRDB*0.05 + 0.2
			if scale > 1 {
				scale = 1
			}
			bc.envGRDB += bc.alphaRel * scale * (targetGR - bc.envGRDB)
		} else {
			bc.envGRDB += bc.alphaRel * (targetGR - bc.envGRDB)
		}
	}

	if bc.envGRDB < 1e-25 {
		bc.envGRDB = 0
	}

	gainDB := -bc.envGRDB
	gain := float32(math.Pow(10, float64(gainDB)/20.0)) * bc.makeupLin
	if math.IsNaN(float64(gain)) || math.IsInf(float64(gain), 0) {
		gain = 1.0
	}

	outL := l * gain
	outR := r * gain

	sat := bc.cfg.Saturation
	if sat > 0 {
		drive := 1.0 + sat*2.0
		invDrive := 1.0 / drive
		sL := float32(math.Tanh(float64(outL*drive))) * invDrive
		sR := float32(math.Tanh(float64(outR*drive))) * invDrive
		outL = (1.0-sat)*outL + sat*sL
		outR = (1.0-sat)*outR + sat*sR
	}

	return outL, outR
}

type MultibandDynamics struct {
	sampleRate float64
	cfg        MultibandConfig

	lowL   lrCascade
	lowR   lrCascade
	midHPL lrCascade
	midHPR lrCascade
	midLPL lrCascade
	midLPR lrCascade
	highL  lrCascade
	highR  lrCascade
	apL    lrCascade
	apR    lrCascade

	compLow  bandCompressor
	compMid  bandCompressor
	compHigh bandCompressor
}

func NewMultibandDynamics(sampleRate float64, cfg MultibandConfig) *MultibandDynamics {
	if sampleRate <= 0 || math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) {
		sampleRate = 48000
	}
	mb := &MultibandDynamics{sampleRate: sampleRate}
	mb.SetConfig(cfg)
	return mb
}

func (mb *MultibandDynamics) SetConfig(cfg MultibandConfig) {
	if cfg.LowMidFreq < 20 {
		cfg.LowMidFreq = 20
	}
	if cfg.MidHighFreq <= cfg.LowMidFreq {
		cfg.MidHighFreq = cfg.LowMidFreq + 100
	}
	maxFreq := float32(mb.sampleRate * 0.45)
	if cfg.MidHighFreq > maxFreq {
		cfg.MidHighFreq = maxFreq
	}

	mb.cfg = cfg

	setupButterworthLP(&mb.lowL, float64(cfg.LowMidFreq), mb.sampleRate)
	setupButterworthLP(&mb.lowR, float64(cfg.LowMidFreq), mb.sampleRate)
	setupButterworthHP(&mb.midHPL, float64(cfg.LowMidFreq), mb.sampleRate)
	setupButterworthHP(&mb.midHPR, float64(cfg.LowMidFreq), mb.sampleRate)

	setupButterworthLP(&mb.midLPL, float64(cfg.MidHighFreq), mb.sampleRate)
	setupButterworthLP(&mb.midLPR, float64(cfg.MidHighFreq), mb.sampleRate)
	setupButterworthHP(&mb.highL, float64(cfg.MidHighFreq), mb.sampleRate)
	setupButterworthHP(&mb.highR, float64(cfg.MidHighFreq), mb.sampleRate)

	setupAllpass(&mb.apL, float64(cfg.MidHighFreq), mb.sampleRate)
	setupAllpass(&mb.apR, float64(cfg.MidHighFreq), mb.sampleRate)

	mb.compLow.Init(cfg.LowBand, mb.sampleRate)
	mb.compMid.Init(cfg.MidBand, mb.sampleRate)
	mb.compHigh.Init(cfg.HighBand, mb.sampleRate)
}

func (mb *MultibandDynamics) Config() MultibandConfig {
	return mb.cfg
}

func (mb *MultibandDynamics) Reset() {
	mb.lowL.Reset()
	mb.lowR.Reset()
	mb.midHPL.Reset()
	mb.midHPR.Reset()
	mb.midLPL.Reset()
	mb.midLPR.Reset()
	mb.highL.Reset()
	mb.highR.Reset()
	mb.apL.Reset()
	mb.apR.Reset()

	mb.compLow.Reset()
	mb.compMid.Reset()
	mb.compHigh.Reset()
}

func (mb *MultibandDynamics) ProcessFrame(inL, inR float32) (float32, float32) {
	if mb.cfg.Bypass {
		return inL, inR
	}

	if math.IsNaN(float64(inL)) || math.IsInf(float64(inL), 0) {
		inL = 0
	}
	if math.IsNaN(float64(inR)) || math.IsInf(float64(inR), 0) {
		inR = 0
	}

	rawLowL := mb.lowL.ProcessSample(inL)
	rawLowR := mb.lowR.ProcessSample(inR)
	lowL := mb.apL.ProcessSample(rawLowL)
	lowR := mb.apR.ProcessSample(rawLowR)

	hpL := mb.midHPL.ProcessSample(inL)
	hpR := mb.midHPR.ProcessSample(inR)

	midL := mb.midLPL.ProcessSample(hpL)
	midR := mb.midLPR.ProcessSample(hpR)

	highL := mb.highL.ProcessSample(hpL)
	highR := mb.highR.ProcessSample(hpR)

	cLowL, cLowR := mb.compLow.Process(lowL, lowR)
	cMidL, cMidR := mb.compMid.Process(midL, midR)
	cHighL, cHighR := mb.compHigh.Process(highL, highR)

	outL := cLowL + cMidL + cHighL
	outR := cLowR + cMidR + cHighR

	if math.IsNaN(float64(outL)) || math.IsInf(float64(outL), 0) {
		outL = 0
	}
	if math.IsNaN(float64(outR)) || math.IsInf(float64(outR), 0) {
		outR = 0
	}

	return outL, outR
}

func (mb *MultibandDynamics) Process(inL, inR, outL, outR []float32) {
	n := len(inL)
	if len(inR) < n {
		n = len(inR)
	}
	if len(outL) < n {
		n = len(outL)
	}
	if len(outR) < n {
		n = len(outR)
	}
	if n == 0 {
		return
	}

	_ = inL[n-1]
	_ = inR[n-1]
	_ = outL[n-1]
	_ = outR[n-1]

	for i := 0; i < n; i++ {
		oL, oR := mb.ProcessFrame(inL[i], inR[i])
		outL[i] = oL
		outR[i] = oR
	}
}

func (mb *MultibandDynamics) ProcessInterleaved(interleaved []float32) {
	frames := len(interleaved) / 2
	if frames == 0 {
		return
	}

	_ = interleaved[2*frames-1]

	for i := 0; i < frames; i++ {
		oL, oR := mb.ProcessFrame(interleaved[2*i], interleaved[2*i+1])
		interleaved[2*i] = oL
		interleaved[2*i+1] = oR
	}
}

func setupButterworthLP(cas *lrCascade, fc, fs float64) {
	w0 := 2.0 * math.Pi * fc / fs
	cosW := math.Cos(w0)
	sinW := math.Sin(w0)
	alpha := sinW * 0.7071067811865475

	b0 := float32((1.0 - cosW) * 0.5)
	b1 := float32(1.0 - cosW)
	b2 := float32((1.0 - cosW) * 0.5)
	a0 := float32(1.0 + alpha)
	a1 := float32(-2.0 * cosW)
	a2 := float32(1.0 - alpha)

	invA0 := 1.0 / a0
	bq := lrBiquad{
		b0: b0 * invA0,
		b1: b1 * invA0,
		b2: b2 * invA0,
		a1: a1 * invA0,
		a2: a2 * invA0,
	}
	cas.stage1 = bq
	cas.stage2 = bq
}

func setupButterworthHP(cas *lrCascade, fc, fs float64) {
	w0 := 2.0 * math.Pi * fc / fs
	cosW := math.Cos(w0)
	sinW := math.Sin(w0)
	alpha := sinW * 0.7071067811865475

	b0 := float32((1.0 + cosW) * 0.5)
	b1 := float32(-(1.0 + cosW))
	b2 := float32((1.0 + cosW) * 0.5)
	a0 := float32(1.0 + alpha)
	a1 := float32(-2.0 * cosW)
	a2 := float32(1.0 - alpha)

	invA0 := 1.0 / a0
	bq := lrBiquad{
		b0: b0 * invA0,
		b1: b1 * invA0,
		b2: b2 * invA0,
		a1: a1 * invA0,
		a2: a2 * invA0,
	}
	cas.stage1 = bq
	cas.stage2 = bq
}

func setupAllpass(cas *lrCascade, fc, fs float64) {
	w0 := 2.0 * math.Pi * fc / fs
	cosW := math.Cos(w0)
	sinW := math.Sin(w0)
	alpha := sinW * 0.7071067811865475

	b0 := float32(1.0 - alpha)
	b1 := float32(-2.0 * cosW)
	b2 := float32(1.0 + alpha)
	a0 := float32(1.0 + alpha)
	a1 := float32(-2.0 * cosW)
	a2 := float32(1.0 - alpha)

	invA0 := 1.0 / a0
	bq := lrBiquad{
		b0: b0 * invA0,
		b1: b1 * invA0,
		b2: b2 * invA0,
		a1: a1 * invA0,
		a2: a2 * invA0,
	}
	cas.stage1 = bq
	cas.stage2 = bq
}
