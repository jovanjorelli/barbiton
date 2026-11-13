package dsp

import "math"

const (
	DefaultLookahead = 0.005

	DefaultLimiterThresholdDB = -6.0

	DefaultLimiterKneeDB = 6.0

	DefaultCeiling = 0.988

	DefaultTargetLUFS = -14.0

	detectorAttackSeconds  = 0.0002
	detectorReleaseSeconds = 0.06
	gainReleaseSeconds     = 0.1
	loudnessWindowSeconds  = 0.4
	autoGainTimeSeconds    = 0.5
	loudnessGateLUFS       = -50.0
	loudnessCalibration    = 0.691
	defaultMaxBoost        = 8.0
	envFloor               = 1e-9
)

type LookaheadBuffer struct {
	bufL, bufR []float32
	writeIdx   int
	mask       int
	delay      int
}

func NewLookaheadBuffer(delay int) *LookaheadBuffer {
	if delay < 1 {
		delay = 1
	}
	capacity := 1
	for capacity <= delay {
		capacity <<= 1
	}
	return &LookaheadBuffer{
		bufL:  make([]float32, capacity),
		bufR:  make([]float32, capacity),
		mask:  capacity - 1,
		delay: delay,
	}
}

func (d *LookaheadBuffer) Write(l, r float32) (float32, float32) {
	bufL := d.bufL
	bufR := d.bufR
	m := d.mask
	_ = bufL[m]
	_ = bufR[m]
	readIdx := (d.writeIdx - d.delay) & m
	wIdx := d.writeIdx & m
	outL, outR := bufL[readIdx], bufR[readIdx]
	bufL[wIdx] = l
	bufR[wIdx] = r
	d.writeIdx = (wIdx + 1) & m
	return outL, outR
}

func (d *LookaheadBuffer) Reset() {
	clear(d.bufL)
	clear(d.bufR)
	d.writeIdx = 0
}

func (d *LookaheadBuffer) Delay() int { return d.delay }

func (d *LookaheadBuffer) Cap() int { return d.mask + 1 }

type Limiter struct {
	sampleRate float64

	delay   LookaheadBuffer
	envRing []float32

	env      float32
	alphaAtt float32
	alphaRel float32

	thresholdDB  float32
	kneeDB       float32
	kneeStartDB  float32
	kneeStart    float32
	threshLinear float32
	kneeEnd      float32
	kneeInvRange float32
	kneeHermiteA float32
	kneeHermiteB float32
	ceiling      float32
	gain         float32
	alphaGainRel float32

	autoEnabled bool
	targetLUFS  float32
	targetMS    float32
	loudMS      float32
	loudGateMS  float32
	alphaLoud   float32
	alphaAuto   float32
	autoGain    float32
	maxBoost    float32
	loudHP      *Biquad

	tpHistL [12]float32
	tpHistR [12]float32
}

var truePeakFIR = [4][12]float32{
	{+0.0017089843750, +0.0109863281250, -0.0196533203125, +0.0332031250000, -0.0594482421875, +0.1373291015625, +0.9721679687500, -0.1022949218750, +0.0476074218750, -0.0266113281250, +0.0148925781250, -0.0083007812500},
	{-0.0291748046875, +0.0292968750000, -0.0517578125000, +0.0891113281250, -0.1665039062500, +0.4650878906250, +0.7797851562500, -0.2003173828125, +0.1015625000000, -0.0582275390625, +0.0330810546875, -0.0189208984375},
	{-0.0189208984375, +0.0330810546875, -0.0582275390625, +0.1015625000000, -0.2003173828125, +0.7797851562500, +0.4650878906250, -0.1665039062500, +0.0891113281250, -0.0517578125000, +0.0292968750000, -0.0291748046875},
	{-0.0083007812500, +0.0148925781250, -0.0266113281250, +0.0476074218750, -0.1022949218750, +0.9721679687500, +0.1373291015625, -0.0594482421875, +0.0332031250000, -0.0196533203125, +0.0109863281250, +0.0017089843750},
}

func NewLimiter(sampleRate float64) *Limiter {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	delayFrames := int(math.Round(DefaultLookahead * sampleRate))
	delay := NewLookaheadBuffer(delayFrames)
	l := &Limiter{
		sampleRate:   sampleRate,
		delay:        *delay,
		envRing:      make([]float32, delay.Cap()),
		alphaAtt:     float32(math.Exp(-1.0 / (sampleRate * detectorAttackSeconds))),
		alphaRel:     float32(math.Exp(-1.0 / (sampleRate * detectorReleaseSeconds))),
		thresholdDB:  DefaultLimiterThresholdDB,
		kneeDB:       DefaultLimiterKneeDB,
		ceiling:      DefaultCeiling,
		gain:         1,
		alphaGainRel: float32(math.Exp(-1.0 / (sampleRate * gainReleaseSeconds))),
		autoEnabled:  true,
		targetLUFS:   DefaultTargetLUFS,
		loudGateMS:   float32(math.Pow(10, (loudnessGateLUFS-loudnessCalibration)/10)),
		alphaLoud:    float32(1 - math.Exp(-1.0/(sampleRate*loudnessWindowSeconds))),
		alphaAuto:    float32(1 - math.Exp(-1.0/(sampleRate*autoGainTimeSeconds))),
		autoGain:     1,
		maxBoost:     defaultMaxBoost,
		loudHP:       NewBiquad(Highpass, 60, defaultQ, sampleRate),
	}
	l.SetTargetLUFS(DefaultTargetLUFS)
	l.SetThresholdDB(DefaultLimiterThresholdDB)
	return l
}

func (l *Limiter) SetThresholdDB(db float32) {
	l.thresholdDB = ClampF32(db, -60, 0)
	l.kneeStartDB = l.thresholdDB - l.kneeDB/2
	l.kneeStart = float32(math.Pow(10, float64(l.kneeStartDB)/20))
	l.threshLinear = float32(math.Pow(10, float64(l.thresholdDB)/20))
	kneeEndDB := l.thresholdDB + l.kneeDB/2
	l.kneeEnd = float32(math.Pow(10, float64(kneeEndDB)/20))
	if l.kneeEnd > l.kneeStart {
		l.kneeInvRange = 1 / (l.kneeEnd - l.kneeStart)
		g1 := l.threshLinear / l.kneeEnd
		dg1 := -g1 / l.kneeEnd * (l.kneeEnd - l.kneeStart)
		l.kneeHermiteB = dg1 - 2*(g1-1)
		l.kneeHermiteA = (g1 - 1) - l.kneeHermiteB
	} else {
		l.kneeInvRange = 0
		l.kneeHermiteA = 0
		l.kneeHermiteB = 0
	}
}

func (l *Limiter) ThresholdDB() float32 { return l.thresholdDB }

func (l *Limiter) SetKneeDB(db float32) {
	l.kneeDB = ClampF32(db, 0, 24)
	l.SetThresholdDB(l.thresholdDB)
}

func (l *Limiter) SetCeiling(c float32) {
	l.ceiling = ClampF32(c, 0.01, 1)
}

func (l *Limiter) Ceiling() float32 { return l.ceiling }

func (l *Limiter) SetAutoGain(enabled bool) {
	l.autoEnabled = enabled
	if !enabled {
		l.autoGain = 1
		l.loudMS = 0
	}
}

func (l *Limiter) AutoGainEnabled() bool { return l.autoEnabled }

func (l *Limiter) SetTargetLUFS(lufs float32) {
	l.targetLUFS = ClampF32(lufs, -40, 0)
	l.targetMS = float32(math.Pow(10, float64(l.targetLUFS-loudnessCalibration)/10))
}

func (l *Limiter) TargetLUFS() float32 { return l.targetLUFS }

func (l *Limiter) Gain() float32 { return l.gain }

func (l *Limiter) AutoGain() float32 { return l.autoGain }

func (l *Limiter) GainReductionDB() float32 {
	if l.gain <= 1e-6 {
		return -120
	}
	return float32(20 * math.Log10(float64(l.gain)))
}

func (l *Limiter) LoudnessLUFS() float32 {
	if l.loudMS <= 1e-7 {
		return -70
	}
	v := float32(10*math.Log10(float64(l.loudMS)) + loudnessCalibration)
	if v < -70 {
		return -70
	}
	return v
}

func (l *Limiter) DelayFrames() int { return l.delay.Delay() }

func (l *Limiter) Reset() {
	l.delay.Reset()
	clear(l.envRing)
	l.tpHistL = [12]float32{}
	l.tpHistR = [12]float32{}
	l.env = 0
	l.gain = 1
	l.autoGain = 1
	l.loudMS = 0
	l.loudHP.Reset()
}

func (l *Limiter) ProcessStereo(buf *StereoBuffer) {
	if buf == nil {
		return
	}
	left, right := buf.Left, buf.Right
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		left[i], right[i] = l.processFrame(left[i], right[i])
	}
}

func (l *Limiter) ProcessInterleaved(b []float32) {
	for i := 0; i+1 < len(b); i += 2 {
		b[i], b[i+1] = l.processFrame(b[i], b[i+1])
	}
}

func (l *Limiter) processFrame(inL, inR float32) (float32, float32) {
	inL = sanitizeF32(inL)
	inR = sanitizeF32(inR)
	l.updateLoudness((inL + inR) * 0.5)

	g := l.autoGain
	if math.IsNaN(float64(g)) || g <= 0 {
		g = 1
		l.autoGain = 1
	}
	xL, xR := inL*g, inR*g

	peak := absF32(xL)
	if a := absF32(xR); a > peak {
		peak = a
	}
	copy(l.tpHistL[1:], l.tpHistL[:11])
	l.tpHistL[0] = xL
	copy(l.tpHistR[1:], l.tpHistR[:11])
	l.tpHistR[0] = xR
	for p := 0; p < 4; p++ {
		var sL, sR float32
		for j := 0; j < 12; j++ {
			c := truePeakFIR[p][j]
			sL += c * l.tpHistL[j]
			sR += c * l.tpHistR[j]
		}
		if a := absF32(sL); a > peak {
			peak = a
		}
		if a := absF32(sR); a > peak {
			peak = a
		}
	}
	if peak > l.env {
		l.env = peak + l.alphaAtt*(l.env-peak)
	} else {
		l.env = peak + l.alphaRel*(l.env-peak)
	}

	guaranteed := l.env
	if peak > guaranteed {
		guaranteed = peak
	}
	readIdx := (l.delay.writeIdx - l.delay.delay) & l.delay.mask
	envDelayed := l.envRing[readIdx]
	l.envRing[l.delay.writeIdx] = guaranteed

	target := l.kneeGain(l.env)
	if envDelayed > envFloor {
		if safe := l.ceiling / envDelayed; safe < target {
			target = safe
		}
	}
	if target > 1 {
		target = 1
	}
	if target < l.gain {
		l.gain = target
	} else {
		l.gain += l.alphaGainRel * (target - l.gain)
	}

	outL, outR := l.delay.Write(xL, xR)
	outL = clampCeiling(outL*l.gain, l.ceiling)
	outR = clampCeiling(outR*l.gain, l.ceiling)
	return outL, outR
}

func (l *Limiter) kneeGain(env float32) float32 {
	if env <= l.kneeStart {
		return 1
	}
	if env >= l.kneeEnd || l.kneeEnd <= l.kneeStart {
		return l.threshLinear / env
	}
	u := (env - l.kneeStart) * l.kneeInvRange
	return 1 + u*u*(l.kneeHermiteA+l.kneeHermiteB*u)
}

func (l *Limiter) updateLoudness(mono float32) {
	if !l.autoEnabled {
		return
	}
	hp := l.loudHP.ProcessSample(mono)
	inst := hp * hp
	if inst <= l.loudGateMS && l.loudMS <= l.loudGateMS {
		return
	}
	l.loudMS += l.alphaLoud * (inst - l.loudMS)
	if l.loudMS <= l.loudGateMS || l.loudMS <= 0 || math.IsNaN(float64(l.loudMS)) {
		return
	}
	desired := float32(math.Sqrt(float64(l.targetMS / l.loudMS)))
	if math.IsNaN(float64(desired)) || math.IsInf(float64(desired), 0) {
		desired = 1
	}
	if math.IsNaN(float64(l.autoGain)) || l.autoGain <= 0 {
		l.autoGain = 1
	}
	desired = ClampF32(desired, 1/l.maxBoost, l.maxBoost)
	l.autoGain += l.alphaAuto * (desired - l.autoGain)
}

func sanitizeF32(v float32) float32 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0
	}
	if math.Abs(float64(v)) < 1e-30 {
		return 0
	}
	return v
}

func absF32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func clampCeiling(v, c float32) float32 {
	if math.IsNaN(float64(v)) {
		return 0
	}
	if v > c {
		return c
	}
	if v < -c {
		return -c
	}
	return v
}
