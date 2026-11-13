package dsp

import (
	"math"
	"strings"
)

type TurntableMode int

const (
	TurntableModeBypass TurntableMode = iota
	TurntableModeBrake
	TurntableModeStart
	TurntableModeScratch
	TurntableModeBeatRoll
	TurntableModeBackspin
)

const (
	turntableBufferBits                 = 19
	turntableBufferSize                 = 1 << turntableBufferBits
	turntableBufferMask                 = turntableBufferSize - 1
	defaultPlatterInertiaMs             = 30.0
	minScratchSpeed                     = -3.0
	maxScratchSpeed                     = 3.0
	SubdivisionThirtySecond Subdivision = 0.125
)

func ParseTurntableSubdivision(s string) (Subdivision, bool) {
	if sub, ok := ParseSubdivision(s); ok {
		return sub, true
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1/32", "thirtysecond", "thirty-second":
		return SubdivisionThirtySecond, true
	}
	return 0, false
}

type TurntableConfig struct {
	InertiaMs float64
}

func DefaultTurntableConfig() TurntableConfig {
	return TurntableConfig{
		InertiaMs: defaultPlatterInertiaMs,
	}
}

type Turntable struct {
	sampleRate float64
	inertiaMs  float64

	ringL []float32
	ringR []float32

	writePos int64
	readPos  float64

	mode TurntableMode

	currentSpeed float64
	targetSpeed  float64
	inertiaCoeff float64

	brakeElapsedSec  float64
	brakeDurationSec float64
	brakeCurve       string

	startElapsedSec  float64
	startDurationSec float64
	startCurve       string

	rollActive  bool
	rollLen     int
	rollStart   int64
	rollPos     float64
	rollFadeLen int

	backspinElapsedSec  float64
	backspinDurationSec float64
	backspinStartSpeed  float64
}

func NewTurntable(sampleRate float64) *Turntable {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	t := &Turntable{
		sampleRate:   sampleRate,
		ringL:        make([]float32, turntableBufferSize),
		ringR:        make([]float32, turntableBufferSize),
		currentSpeed: 1.0,
		targetSpeed:  1.0,
		mode:         TurntableModeBypass,
	}
	t.SetInertiaMs(defaultPlatterInertiaMs)
	return t
}

func (t *Turntable) SetInertiaMs(ms float64) {
	if ms < 0 {
		ms = 0
	}
	t.inertiaMs = ms
	if ms == 0 {
		t.inertiaCoeff = 1.0
	} else {
		dt := 1.0 / t.sampleRate
		tau := ms / 1000.0
		t.inertiaCoeff = 1.0 - math.Exp(-dt/tau)
	}
}

func (t *Turntable) InertiaMs() float64 {
	return t.inertiaMs
}

func (t *Turntable) Mode() TurntableMode {
	return t.mode
}

func (t *Turntable) Speed() float64 {
	return t.currentSpeed
}

func (t *Turntable) Reset() {
	clear(t.ringL)
	clear(t.ringR)
	t.writePos = 0
	t.readPos = 0
	t.mode = TurntableModeBypass
	t.currentSpeed = 1.0
	t.targetSpeed = 1.0
	t.brakeElapsedSec = 0
	t.brakeDurationSec = 0
	t.startElapsedSec = 0
	t.startDurationSec = 0
	t.rollActive = false
	t.backspinElapsedSec = 0
}

func (t *Turntable) Bypass() {
	t.mode = TurntableModeBypass
	t.currentSpeed = 1.0
	t.targetSpeed = 1.0
	t.readPos = float64(t.writePos - 3)
	t.rollActive = false
	t.brakeElapsedSec = 0
	t.brakeDurationSec = 0
	t.startElapsedSec = 0
	t.startDurationSec = 0
	t.backspinElapsedSec = 0
}

func (t *Turntable) Brake(brakeTimeMs float64, curve string) {
	if brakeTimeMs <= 0 {
		brakeTimeMs = 1.0
	}
	t.mode = TurntableModeBrake
	t.brakeDurationSec = brakeTimeMs / 1000.0
	t.brakeElapsedSec = 0
	t.brakeCurve = strings.ToLower(strings.TrimSpace(curve))
	t.readPos = float64(t.writePos - 3)
	t.currentSpeed = 1.0
	t.targetSpeed = 0.0
}

func (t *Turntable) ReleaseBrake(startTimeMs float64, curve string) {
	if startTimeMs <= 0 {
		t.Bypass()
		return
	}
	t.mode = TurntableModeStart
	t.startDurationSec = startTimeMs / 1000.0
	t.startElapsedSec = 0
	t.startCurve = strings.ToLower(strings.TrimSpace(curve))
	t.currentSpeed = 0.0
	t.targetSpeed = 1.0
}

func (t *Turntable) Scratch(speed float32) {
	sp := float64(speed)
	if sp < minScratchSpeed {
		sp = minScratchSpeed
	} else if sp > maxScratchSpeed {
		sp = maxScratchSpeed
	}
	if t.mode != TurntableModeScratch {
		t.mode = TurntableModeScratch
		t.readPos = float64(t.writePos - 3)
	}
	t.targetSpeed = sp
}

func (t *Turntable) BeatRoll(active bool, sub Subdivision, bpm float64) {
	if !active {
		if t.mode == TurntableModeBeatRoll {
			t.Bypass()
		}
		return
	}
	if bpm <= 0 {
		bpm = 120.0
	}
	if sub <= 0 {
		sub = SubdivisionQuarter
	}
	secPerBeat := 60.0 / bpm
	durSec := float64(sub) * secPerBeat
	samples := int(math.Round(durSec * t.sampleRate))
	t.BeatRollSamples(true, samples)
}

func (t *Turntable) BeatRollSamples(active bool, loopLen int) {
	if !active {
		if t.mode == TurntableModeBeatRoll {
			t.Bypass()
		}
		return
	}
	if loopLen < 16 {
		loopLen = 16
	}
	if loopLen > turntableBufferSize/2 {
		loopLen = turntableBufferSize / 2
	}
	t.mode = TurntableModeBeatRoll
	t.rollActive = true
	t.rollLen = loopLen
	t.rollStart = t.writePos - 3 - int64(loopLen)
	t.rollPos = float64(t.rollStart)
	fade := loopLen / 8
	if fade > 64 {
		fade = 64
	}
	if fade < 4 {
		fade = 4
	}
	t.rollFadeLen = fade
	t.currentSpeed = 1.0
	t.targetSpeed = 1.0
}

func (t *Turntable) Backspin(initialSpeed float32, decayMs float64) {
	sp := float64(initialSpeed)
	if sp == 0 {
		sp = -3.0
	}
	if sp > 0 {
		sp = -sp
	}
	if sp < minScratchSpeed {
		sp = minScratchSpeed
	}
	if decayMs <= 0 {
		decayMs = 800.0
	}
	t.mode = TurntableModeBackspin
	t.backspinStartSpeed = sp
	t.backspinDurationSec = decayMs / 1000.0
	t.backspinElapsedSec = 0
	t.readPos = float64(t.writePos - 3)
	t.currentSpeed = sp
	t.targetSpeed = 0.0
}

func sampleRingHermite(ring []float32, pos float64) float32 {
	_ = ring[turntableBufferMask]
	i := int64(math.Floor(pos))
	f := float32(pos - float64(i))
	y0 := ring[(i-1)&turntableBufferMask]
	y1 := ring[i&turntableBufferMask]
	y2 := ring[(i+1)&turntableBufferMask]
	y3 := ring[(i+2)&turntableBufferMask]
	c0 := y1
	c1 := 0.5 * (y2 - y0)
	c2 := y0 - 2.5*y1 + 2.0*y2 - 0.5*y3
	c3 := 0.5*(y3-y0) + 1.5*(y1-y2)
	return ((c3*f+c2)*f+c1)*f + c0
}

func (t *Turntable) processSample(inL, inR float32) (float32, float32) {
	_ = t.ringL[turntableBufferMask]
	_ = t.ringR[turntableBufferMask]
	wIdx := t.writePos & turntableBufferMask
	t.ringL[wIdx] = inL
	t.ringR[wIdx] = inR
	t.writePos++

	switch t.mode {
	case TurntableModeBypass:
		t.readPos = float64(t.writePos - 3)
		t.currentSpeed = 1.0
		return inL, inR

	case TurntableModeBrake:
		dt := 1.0 / t.sampleRate
		t.brakeElapsedSec += dt
		u := t.brakeElapsedSec / t.brakeDurationSec
		if u >= 1.0 {
			t.currentSpeed = 0.0
			return 0, 0
		}
		var v float64
		if t.brakeCurve == "exponential" || t.brakeCurve == "exp" {
			k := 4.0
			v = (math.Exp(-k*u) - math.Exp(-k)) / (1.0 - math.Exp(-k))
		} else {
			v = 1.0 - u
		}
		if v < 0 {
			v = 0
		}
		t.currentSpeed = v
		t.readPos += v
		maxPos := float64(t.writePos - 3)
		if t.readPos > maxPos {
			t.readPos = maxPos
		}
		outL := sampleRingHermite(t.ringL, t.readPos)
		outR := sampleRingHermite(t.ringR, t.readPos)
		if u > 0.98 {
			fade := float32((1.0 - u) / 0.02)
			outL *= fade
			outR *= fade
		}
		return outL, outR

	case TurntableModeStart:
		dt := 1.0 / t.sampleRate
		t.startElapsedSec += dt
		u := t.startElapsedSec / t.startDurationSec
		if u >= 1.0 {
			t.mode = TurntableModeBypass
			t.currentSpeed = 1.0
			t.readPos = float64(t.writePos - 3)
			return inL, inR
		}
		var v float64
		if t.startCurve == "exponential" || t.startCurve == "exp" {
			k := 4.0
			v = (math.Exp(k*u) - 1.0) / (math.Exp(k) - 1.0)
		} else {
			v = u
		}
		if v < 0 {
			v = 0
		} else if v > 1.0 {
			v = 1.0
		}
		t.currentSpeed = v
		t.readPos += v
		maxPos := float64(t.writePos - 3)
		if t.readPos > maxPos {
			t.readPos = maxPos
		}
		outL := sampleRingHermite(t.ringL, t.readPos)
		outR := sampleRingHermite(t.ringR, t.readPos)
		if u > 0.95 {
			mix := float32((1.0 - u) / 0.05)
			outL = mix*outL + (1.0-mix)*inL
			outR = mix*outR + (1.0-mix)*inR
		}
		return outL, outR

	case TurntableModeScratch:
		t.currentSpeed += t.inertiaCoeff * (t.targetSpeed - t.currentSpeed)
		t.readPos += t.currentSpeed
		maxPos := float64(t.writePos - 3)
		if t.readPos > maxPos {
			t.readPos = maxPos
		}
		minPos := float64(t.writePos - turntableBufferSize + 4)
		if t.readPos < minPos {
			t.readPos = minPos
		}
		gain := float32(1.0)
		absSpeed := math.Abs(t.currentSpeed)
		if absSpeed < 0.05 {
			gain = float32(absSpeed / 0.05)
		}
		outL := sampleRingHermite(t.ringL, t.readPos) * gain
		outR := sampleRingHermite(t.ringR, t.readPos) * gain
		return outL, outR

	case TurntableModeBeatRoll:
		offset := t.rollPos - float64(t.rollStart)
		if offset >= float64(t.rollLen) {
			t.rollPos -= float64(t.rollLen)
			offset = t.rollPos - float64(t.rollStart)
		}
		outL := sampleRingHermite(t.ringL, t.rollPos)
		outR := sampleRingHermite(t.ringR, t.rollPos)
		if offset < float64(t.rollFadeLen) {
			w := float32(offset / float64(t.rollFadeLen))
			wrapPos := t.rollPos + float64(t.rollLen)
			maxPos := float64(t.writePos - 3)
			if wrapPos > maxPos {
				wrapPos = maxPos
			}
			endL := sampleRingHermite(t.ringL, wrapPos)
			endR := sampleRingHermite(t.ringR, wrapPos)
			outL = w*outL + (1.0-w)*endL
			outR = w*outR + (1.0-w)*endR
		}
		t.rollPos += 1.0
		return outL, outR

	case TurntableModeBackspin:
		dt := 1.0 / t.sampleRate
		t.backspinElapsedSec += dt
		u := t.backspinElapsedSec / t.backspinDurationSec
		if u >= 1.0 {
			t.mode = TurntableModeBypass
			t.currentSpeed = 1.0
			t.readPos = float64(t.writePos - 3)
			return inL, inR
		}
		v := t.backspinStartSpeed * math.Exp(-3.5*u)
		t.currentSpeed = v
		t.readPos += v
		minPos := float64(t.writePos - turntableBufferSize + 4)
		if t.readPos < minPos {
			t.readPos = minPos
		}
		outL := sampleRingHermite(t.ringL, t.readPos)
		outR := sampleRingHermite(t.ringR, t.readPos)
		gain := float32(1.0 - u)
		outL *= gain
		outR *= gain
		if u > 0.95 {
			mix := float32((1.0 - u) / 0.05)
			outL += (1.0 - mix) * inL
			outR += (1.0 - mix) * inR
		}
		return outL, outR

	default:
		return inL, inR
	}
}

func (t *Turntable) ProcessStereo(left, right []float32) {
	n := len(left)
	if n == 0 || len(right) < n {
		return
	}
	_ = left[n-1]
	_ = right[n-1]
	for i := 0; i < n; i++ {
		l, r := t.processSample(left[i], right[i])
		left[i] = l
		right[i] = r
	}
}

func (t *Turntable) ProcessInterleaved(stereo []float32) {
	frames := len(stereo) / 2
	if frames == 0 {
		return
	}
	_ = stereo[2*frames-1]
	for i := 0; i < frames; i++ {
		idx := 2 * i
		l, r := t.processSample(stereo[idx], stereo[idx+1])
		stereo[idx] = l
		stereo[idx+1] = r
	}
}

func (t *Turntable) Process(inL, inR, outL, outR []float32) {
	n := len(inL)
	if n == 0 || len(inR) < n || len(outL) < n || len(outR) < n {
		return
	}
	_ = inL[n-1]
	_ = inR[n-1]
	_ = outL[n-1]
	_ = outR[n-1]
	for i := 0; i < n; i++ {
		l, r := t.processSample(inL[i], inR[i])
		outL[i] = l
		outR[i] = r
	}
}
