package synth

import (
	"math"

	"barbiton/pkg/dsp"
)

type Waveform int

const (
	WaveSaw Waveform = iota
	WaveSquare
	WaveSine
	WaveTriangle
	WaveNoise
	WavePinkNoise
)

func waveParam(v float32) Waveform {
	if math.IsNaN(float64(v)) {
		return WaveSaw
	}
	switch int(math.Round(float64(v))) {
	case 1:
		return WaveSquare
	case 2:
		return WaveSine
	case 3:
		return WaveTriangle
	case 4:
		return WaveNoise
	case 5:
		return WavePinkNoise
	default:
		return WaveSaw
	}
}

func polyBLEP(t, dt float64) float64 {
	if dt <= 0 || dt >= 0.5 {
		return 0
	}
	if t >= 0 && t < dt {
		u := t / dt
		return u + u - u*u - 1.0
	}
	if t > 1.0-dt && t <= 1.0 {
		u := (t - 1.0) / dt
		return u*u + u + u + 1.0
	}
	return 0
}

func polyBLAMP(t, dt float64) float64 {
	if dt <= 0 || dt >= 0.5 {
		return 0
	}
	if t >= 0 && t < dt {
		u := t/dt - 1.0
		return -1.0 / 3.0 * u * u * u
	}
	if t > 1.0-dt && t <= 1.0 {
		u := (t-1.0)/dt + 1.0
		return 1.0 / 3.0 * u * u * u
	}
	return 0
}

func oscWaveBlepPWM(w Waveform, phase, dt, duty float64) float64 {
	frac := phase - math.Floor(phase)
	switch w {
	case WaveSquare:
		d := duty
		if math.IsNaN(d) || d < 0.05 {
			d = 0.05
		} else if d > 0.95 {
			d = 0.95
		}
		naive := 1.0
		if frac >= d {
			naive = -1.0
		}
		if dt <= 0 {
			return naive
		}
		frac2 := frac - d
		if frac2 < 0 {
			frac2 += 1.0
		}
		return naive + polyBLEP(frac, dt) - polyBLEP(frac2, dt)
	case WaveSine:
		return float64(dsp.SineFast(frac))
	case WaveTriangle:
		t := frac
		t1 := t + 0.25
		t1 -= math.Floor(t1)
		t2 := t + 0.75
		t2 -= math.Floor(t2)
		y := t * 4.0
		if y >= 3.0 {
			y -= 4.0
		} else if y > 1.0 {
			y = 2.0 - y
		}
		if dt > 0 {
			y += 4.0 * dt * (polyBLAMP(t1, dt) - polyBLAMP(t2, dt))
		}
		return y
	default:
		naive := 2*frac - 1
		if dt <= 0 {
			return naive
		}
		return naive - polyBLEP(frac, dt)
	}
}

func oscWaveBlep(w Waveform, phase, dt float64) float64 {
	return oscWaveBlepPWM(w, phase, dt, 0.5)
}

func oscWave(w Waveform, phase float64) float64 {
	return oscWaveBlep(w, phase, 0)
}

func xorshift64Step(state *uint64) float64 {
	x := *state
	if x == 0 {
		x = dsp.DefaultNoiseSeed
	}
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*state = x
	return float64(x>>40)*(1.0/8388608.0) - 1.0
}

func kelletPinkStep(b *[7]float64, white float64) float64 {
	b[0] = 0.99886*b[0] + white*0.0555179
	b[1] = 0.99332*b[1] + white*0.0750759
	b[2] = 0.96900*b[2] + white*0.1538520
	b[3] = 0.86650*b[3] + white*0.3104856
	b[4] = 0.55000*b[4] + white*0.5329522
	b[5] = -0.7616*b[5] - white*0.0168980
	pink := b[0] + b[1] + b[2] + b[3] + b[4] + b[5] + b[6] + white*0.5362
	b[6] = white * 0.115926
	return pink * 0.11
}
