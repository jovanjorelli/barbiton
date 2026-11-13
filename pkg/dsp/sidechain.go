package dsp

import (
	"math"
)

type SidechainMode int

const (
	SidechainPeak SidechainMode = iota
	SidechainRMS
)

type SidechainConfig struct {
	ThresholdDB float32
	Ratio       float32
	AttackMs    float32
	ReleaseMs   float32
	HoldMs      float32
	MakeupDB    float32
	Mode        SidechainMode
}

func DefaultSidechainConfig() SidechainConfig {
	return SidechainConfig{
		ThresholdDB: -20.0,
		Ratio:       4.0,
		AttackMs:    10.0,
		ReleaseMs:   100.0,
		HoldMs:      10.0,
		MakeupDB:    0.0,
		Mode:        SidechainPeak,
	}
}

type SidechainCompressor struct {
	sampleRate float64
	cfg        SidechainConfig

	envGainRedDB float32
	rmsAcc       float32
	alphaAttack  float32
	alphaRelease float32
	alphaRMS     float32
	holdSamples  int
	holdCounter  int
	makeupLinear float32
	threshDB     float32
	invRatio     float32
}

func NewSidechainCompressor(sampleRate float64, cfg SidechainConfig) *SidechainCompressor {
	if sampleRate <= 0 || math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) {
		sampleRate = 48000
	}
	sc := &SidechainCompressor{sampleRate: sampleRate}
	sc.SetConfig(cfg)
	return sc
}

func (sc *SidechainCompressor) SetConfig(cfg SidechainConfig) {
	if cfg.Ratio < 1 {
		cfg.Ratio = 1
	}
	if cfg.AttackMs < 0.1 {
		cfg.AttackMs = 0.1
	}
	if cfg.ReleaseMs < 1.0 {
		cfg.ReleaseMs = 1.0
	}
	if cfg.HoldMs < 0 {
		cfg.HoldMs = 0
	}
	sc.cfg = cfg
	sc.threshDB = cfg.ThresholdDB
	sc.invRatio = 1.0 / cfg.Ratio

	attSec := float64(cfg.AttackMs) * 0.001
	sc.alphaAttack = float32(1.0 - math.Exp(-1.0/(attSec*sc.sampleRate)))

	relSec := float64(cfg.ReleaseMs) * 0.001
	sc.alphaRelease = float32(1.0 - math.Exp(-1.0/(relSec*sc.sampleRate)))

	rmsWindowSec := 0.02
	sc.alphaRMS = float32(1.0 - math.Exp(-1.0/(rmsWindowSec*sc.sampleRate)))

	sc.holdSamples = int(math.Round(float64(cfg.HoldMs) * 0.001 * sc.sampleRate))
	sc.makeupLinear = float32(math.Pow(10, float64(cfg.MakeupDB)/20.0))
}

func (sc *SidechainCompressor) Reset() {
	sc.envGainRedDB = 0
	sc.rmsAcc = 0
	sc.holdCounter = 0
}

func (sc *SidechainCompressor) Config() SidechainConfig {
	return sc.cfg
}

func (sc *SidechainCompressor) ProcessFrame(targetL, targetR, scL, scR float32) (float32, float32) {
	if math.IsNaN(float64(targetL)) || math.IsInf(float64(targetL), 0) {
		targetL = 0
	}
	if math.IsNaN(float64(targetR)) || math.IsInf(float64(targetR), 0) {
		targetR = 0
	}
	if math.IsNaN(float64(scL)) || math.IsInf(float64(scL), 0) {
		scL = 0
	}
	if math.IsNaN(float64(scR)) || math.IsInf(float64(scR), 0) {
		scR = 0
	}

	var level float32
	if sc.cfg.Mode == SidechainRMS {
		sq := 0.5 * (scL*scL + scR*scR)
		if sq < 1e-25 {
			sq = 0
		}
		sc.rmsAcc += sc.alphaRMS * (sq - sc.rmsAcc)
		if sc.rmsAcc < 1e-25 {
			sc.rmsAcc = 0
		}
		level = float32(math.Sqrt(float64(sc.rmsAcc)))
	} else {
		aL := scL
		if aL < 0 {
			aL = -aL
		}
		aR := scR
		if aR < 0 {
			aR = -aR
		}
		if aL > aR {
			level = aL
		} else {
			level = aR
		}
	}

	if level < 1e-25 {
		level = 0
	}

	var levelDB float32
	if level > 1e-6 {
		levelDB = float32(20.0 * math.Log10(float64(level)))
	} else {
		levelDB = -120.0
	}

	var targetGRDB float32
	if levelDB > sc.threshDB {
		targetGRDB = (levelDB - sc.threshDB) * (1.0 - sc.invRatio)
	} else {
		targetGRDB = 0
	}

	if targetGRDB > sc.envGainRedDB {
		sc.holdCounter = sc.holdSamples
		sc.envGainRedDB += sc.alphaAttack * (targetGRDB - sc.envGainRedDB)
	} else {
		if sc.holdCounter > 0 {
			sc.holdCounter--
		} else {
			sc.envGainRedDB += sc.alphaRelease * (targetGRDB - sc.envGainRedDB)
		}
	}

	if sc.envGainRedDB < 1e-25 {
		sc.envGainRedDB = 0
	}

	gainDB := -sc.envGainRedDB
	gain := float32(math.Pow(10, float64(gainDB)/20.0)) * sc.makeupLinear
	if math.IsNaN(float64(gain)) || math.IsInf(float64(gain), 0) {
		gain = 1.0
	}

	return targetL * gain, targetR * gain
}

func (sc *SidechainCompressor) Process(targetL, targetR, scL, scR []float32) {
	n := len(targetL)
	if n == 0 {
		return
	}
	if len(targetR) < n {
		n = len(targetR)
	}
	if len(scL) < n {
		n = len(scL)
	}
	if len(scR) < n {
		n = len(scR)
	}
	if n == 0 {
		return
	}

	_ = targetL[n-1]
	_ = targetR[n-1]
	_ = scL[n-1]
	_ = scR[n-1]

	for i := 0; i < n; i++ {
		outL, outR := sc.ProcessFrame(targetL[i], targetR[i], scL[i], scR[i])
		targetL[i] = outL
		targetR[i] = outR
	}
}

func (sc *SidechainCompressor) ProcessInterleaved(target, sidechain []float32) {
	frames := len(target) / 2
	scFrames := len(sidechain) / 2
	if scFrames < frames {
		frames = scFrames
	}
	if frames == 0 {
		return
	}

	_ = target[2*frames-1]
	_ = sidechain[2*frames-1]

	for i := 0; i < frames; i++ {
		tL := target[2*i]
		tR := target[2*i+1]
		sL := sidechain[2*i]
		sR := sidechain[2*i+1]
		outL, outR := sc.ProcessFrame(tL, tR, sL, sR)
		target[2*i] = outL
		target[2*i+1] = outR
	}
}
