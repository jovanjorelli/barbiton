package dsp

import (
	"math"
)

type OTTConfig struct {
	Depth     float32
	TimeScale float32
	InGainDB  float32
	OutGainDB float32
	BandGains [3]float32
	Bypass    bool
}

func DefaultOTTConfig() OTTConfig {
	return OTTConfig{
		Depth:     1.0,
		TimeScale: 1.0,
		InGainDB:  0.0,
		OutGainDB: 0.0,
		BandGains: [3]float32{0.0, 0.0, 0.0},
		Bypass:    false,
	}
}

type ottBand struct {
	sampleRate float64
	env        float32
	gainDB     float32

	threshDown float32
	ratioDown  float32
	threshUp   float32
	ratioUp    float32

	alphaAtt float32
	alphaRel float32
	bandGain float32
}

func (b *ottBand) init(sampleRate float64, tDown, rDown, tUp, rUp float32, attSec, relSec float32, bandGainDB float32) {
	b.sampleRate = sampleRate
	b.threshDown = tDown
	b.ratioDown = rDown
	b.threshUp = tUp
	b.ratioUp = rUp
	b.bandGain = bandGainDB

	if attSec < 0.0001 {
		attSec = 0.0001
	}
	if relSec < 0.0001 {
		relSec = 0.0001
	}

	b.alphaAtt = float32(1.0 - math.Exp(-1.0/(float64(attSec)*sampleRate)))
	b.alphaRel = float32(1.0 - math.Exp(-1.0/(float64(relSec)*sampleRate)))
}

func (b *ottBand) reset() {
	b.env = 0
	b.gainDB = 0
}

func (b *ottBand) process(l, r float32) (float32, float32) {
	pkL := l
	if pkL < 0 {
		pkL = -pkL
	}
	pkR := r
	if pkR < 0 {
		pkR = -pkR
	}
	peak := pkL
	if pkR > peak {
		peak = pkR
	}

	if peak > b.env {
		b.env += b.alphaAtt * (peak - b.env)
	} else {
		b.env += b.alphaRel * (peak - b.env)
	}
	if b.env > -1e-25 && b.env < 1e-25 {
		b.env = 0
	}

	var envDB float32
	if b.env > 1e-6 {
		envDB = float32(20.0 * math.Log10(float64(b.env)))
	} else {
		envDB = -120.0
	}

	var targetDB float32
	if envDB > b.threshDown {
		targetDB = -(envDB - b.threshDown) * (1.0 - 1.0/b.ratioDown)
	} else if envDB < b.threshUp && envDB > -80.0 {
		boost := (b.threshUp - envDB) * (1.0 - 1.0/b.ratioUp)
		if boost > 24.0 {
			boost = 24.0
		}
		if envDB < -60.0 {
			gate := (envDB + 80.0) / 20.0
			if gate < 0 {
				gate = 0
			}
			boost *= gate
		}
		targetDB = boost
	}

	if targetDB < b.gainDB {
		b.gainDB += b.alphaAtt * (targetDB - b.gainDB)
	} else {
		b.gainDB += b.alphaRel * (targetDB - b.gainDB)
	}
	if b.gainDB > -1e-25 && b.gainDB < 1e-25 {
		b.gainDB = 0
	}

	totalDB := b.gainDB + b.bandGain
	gain := float32(math.Pow(10.0, float64(totalDB)/20.0))
	if math.IsNaN(float64(gain)) || math.IsInf(float64(gain), 0) {
		gain = 1.0
	}

	return l * gain, r * gain
}

type OTT struct {
	sampleRate float64
	cfg        OTTConfig

	inGainLin  float32
	outGainLin float32

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

	bandLow  ottBand
	bandMid  ottBand
	bandHigh ottBand
}

func NewOTT(sampleRate float64, cfg OTTConfig) *OTT {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	o := &OTT{sampleRate: sampleRate}
	o.SetConfig(cfg)
	return o
}

func (o *OTT) SetConfig(cfg OTTConfig) {
	if cfg.Depth < 0 {
		cfg.Depth = 0
	} else if cfg.Depth > 1 {
		cfg.Depth = 1
	}
	if cfg.TimeScale <= 0 {
		cfg.TimeScale = 1.0
	}

	o.cfg = cfg
	o.inGainLin = float32(math.Pow(10.0, float64(cfg.InGainDB)/20.0))
	o.outGainLin = float32(math.Pow(10.0, float64(cfg.OutGainDB)/20.0))

	lowFreq := 120.0
	highFreq := 2500.0
	maxCrossover := o.sampleRate * 0.45
	if highFreq > maxCrossover {
		highFreq = maxCrossover
	}
	if lowFreq >= highFreq {
		lowFreq = highFreq * 0.5
	}

	setupButterworthLP(&o.lowL, lowFreq, o.sampleRate)
	setupButterworthLP(&o.lowR, lowFreq, o.sampleRate)
	setupButterworthHP(&o.midHPL, lowFreq, o.sampleRate)
	setupButterworthHP(&o.midHPR, lowFreq, o.sampleRate)

	setupButterworthLP(&o.midLPL, highFreq, o.sampleRate)
	setupButterworthLP(&o.midLPR, highFreq, o.sampleRate)
	setupButterworthHP(&o.highL, highFreq, o.sampleRate)
	setupButterworthHP(&o.highR, highFreq, o.sampleRate)

	setupAllpass(&o.apL, highFreq, o.sampleRate)
	setupAllpass(&o.apR, highFreq, o.sampleRate)

	ts := cfg.TimeScale
	o.bandLow.init(o.sampleRate, -24.0, 4.0, -36.0, 2.0, 0.020*ts, 0.100*ts, cfg.BandGains[0])
	o.bandMid.init(o.sampleRate, -20.0, 4.0, -36.0, 2.0, 0.010*ts, 0.050*ts, cfg.BandGains[1])
	o.bandHigh.init(o.sampleRate, -18.0, 4.0, -36.0, 2.0, 0.005*ts, 0.025*ts, cfg.BandGains[2])
}

func (o *OTT) Config() OTTConfig {
	return o.cfg
}

func (o *OTT) SetDepth(depth float32) {
	o.cfg.Depth = depth
	o.SetConfig(o.cfg)
}

func (o *OTT) Depth() float32 {
	return o.cfg.Depth
}

func (o *OTT) SetTimeScale(ts float32) {
	o.cfg.TimeScale = ts
	o.SetConfig(o.cfg)
}

func (o *OTT) TimeScale() float32 {
	return o.cfg.TimeScale
}

func (o *OTT) SetInGainDB(g float32) {
	o.cfg.InGainDB = g
	o.SetConfig(o.cfg)
}

func (o *OTT) InGainDB() float32 {
	return o.cfg.InGainDB
}

func (o *OTT) SetOutGainDB(g float32) {
	o.cfg.OutGainDB = g
	o.SetConfig(o.cfg)
}

func (o *OTT) OutGainDB() float32 {
	return o.cfg.OutGainDB
}

func (o *OTT) SetBandGains(gains [3]float32) {
	o.cfg.BandGains = gains
	o.SetConfig(o.cfg)
}

func (o *OTT) BandGains() [3]float32 {
	return o.cfg.BandGains
}

func (o *OTT) Reset() {
	o.lowL.Reset()
	o.lowR.Reset()
	o.midHPL.Reset()
	o.midHPR.Reset()
	o.midLPL.Reset()
	o.midLPR.Reset()
	o.highL.Reset()
	o.highR.Reset()
	o.apL.Reset()
	o.apR.Reset()

	o.bandLow.reset()
	o.bandMid.reset()
	o.bandHigh.reset()
}

func (o *OTT) ProcessFrame(inL, inR float32) (float32, float32) {
	if o.cfg.Bypass || o.cfg.Depth <= 0 {
		return inL, inR
	}

	if math.IsNaN(float64(inL)) || math.IsInf(float64(inL), 0) {
		inL = 0
	}
	if math.IsNaN(float64(inR)) || math.IsInf(float64(inR), 0) {
		inR = 0
	}

	drivenL := inL * o.inGainLin
	drivenR := inR * o.inGainLin

	rawLowL := o.lowL.ProcessSample(drivenL)
	rawLowR := o.lowR.ProcessSample(drivenR)
	lowL := o.apL.ProcessSample(rawLowL)
	lowR := o.apR.ProcessSample(rawLowR)

	hpL := o.midHPL.ProcessSample(drivenL)
	hpR := o.midHPR.ProcessSample(drivenR)

	midL := o.midLPL.ProcessSample(hpL)
	midR := o.midLPR.ProcessSample(hpR)

	highL := o.highL.ProcessSample(hpL)
	highR := o.highR.ProcessSample(hpR)

	cLowL, cLowR := o.bandLow.process(lowL, lowR)
	cMidL, cMidR := o.bandMid.process(midL, midR)
	cHighL, cHighR := o.bandHigh.process(highL, highR)

	wetL := (cLowL + cMidL + cHighL) * o.outGainLin
	wetR := (cLowR + cMidR + cHighR) * o.outGainLin

	depth := o.cfg.Depth
	outL := (1.0-depth)*inL + depth*wetL
	outR := (1.0-depth)*inR + depth*wetR

	if math.IsNaN(float64(outL)) || math.IsInf(float64(outL), 0) {
		outL = 0
	}
	if math.IsNaN(float64(outR)) || math.IsInf(float64(outR), 0) {
		outR = 0
	}

	return outL, outR
}

func (o *OTT) ProcessInterleaved(buffer []float32) {
	frames := len(buffer) / 2
	if frames == 0 {
		return
	}
	_ = buffer[2*frames-1]
	for i := 0; i < frames; i++ {
		oL, oR := o.ProcessFrame(buffer[2*i], buffer[2*i+1])
		buffer[2*i] = oL
		buffer[2*i+1] = oR
	}
}

func (o *OTT) ProcessStereo(left, right []float32) {
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
		oL, oR := o.ProcessFrame(left[i], right[i])
		left[i] = oL
		right[i] = oR
	}
}

func (o *OTT) ProcessSlice(buffer []float32) {
	n := len(buffer)
	if n == 0 {
		return
	}
	_ = buffer[n-1]
	for i := 0; i < n; i++ {
		oL, _ := o.ProcessFrame(buffer[i], 0)
		buffer[i] = oL
	}
}
