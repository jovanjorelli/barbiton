package dsp

import (
	"math"
	"testing"
)

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func TestBlock2FDNCoprimeLengths(t *testing.T) {
	for i := 0; i < len(fdnBaseLengths); i++ {
		if fdnBaseLengths[i] <= 0 {
			t.Fatalf("fdn length at %d is non-positive: %d", i, fdnBaseLengths[i])
		}
		for j := i + 1; j < len(fdnBaseLengths); j++ {
			g := gcd(fdnBaseLengths[i], fdnBaseLengths[j])
			if g != 1 {
				t.Fatalf("fdnBaseLengths[%d]=%d and fdnBaseLengths[%d]=%d share divisor %d",
					i, fdnBaseLengths[i], j, fdnBaseLengths[j], g)
			}
		}
	}
}

func TestBlock2LimiterKnee(t *testing.T) {
	l := NewLimiter(48000)

	l.SetKneeDB(0)
	threshLin := float32(math.Pow(10, float64(l.ThresholdDB())/20))
	if g := l.kneeGain(threshLin * 0.5); g != 1 {
		t.Fatalf("hard knee below threshold got %v, want 1", g)
	}
	if g := l.kneeGain(threshLin * 2.0); math.Abs(float64(g-0.5)) > 1e-4 {
		t.Fatalf("hard knee above threshold got %v, want 0.5", g)
	}

	l.SetKneeDB(6)
	prevGain := float32(1.0)
	steps := 1000
	minEnv := l.kneeStart * 0.5
	maxEnv := l.kneeEnd * 2.0
	for i := 0; i <= steps; i++ {
		env := minEnv + float32(i)/float32(steps)*(maxEnv-minEnv)
		g := l.kneeGain(env)
		if g > prevGain+1e-6 {
			t.Fatalf("gain increased from %v to %v at env %v", prevGain, g, env)
		}
		if g <= 0 || g > 1 {
			t.Fatalf("gain %v out of (0, 1] at env %v", g, env)
		}
		prevGain = g
	}

	gStart := l.kneeGain(l.kneeStart)
	if math.Abs(float64(gStart-1.0)) > 1e-5 {
		t.Fatalf("gain at kneeStart = %v, want 1.0", gStart)
	}

	gEnd := l.kneeGain(l.kneeEnd)
	wantGEnd := l.threshLinear / l.kneeEnd
	if math.Abs(float64(gEnd-wantGEnd)) > 1e-4 {
		t.Fatalf("gain at kneeEnd = %v, want %v", gEnd, wantGEnd)
	}

	gSlightlyAbove := l.kneeGain(l.kneeEnd * 1.0001)
	if math.Abs(float64(gSlightlyAbove-gEnd)) > 1e-3 {
		t.Fatalf("discontinuity at kneeEnd: %v vs %v", gSlightlyAbove, gEnd)
	}

	l.SetKneeDB(24)
	l.SetThresholdDB(-60)
	for i := 0; i <= 100; i++ {
		env := float32(i) / 100.0
		g := l.kneeGain(env)
		if g <= 0 || g > 1 {
			t.Fatalf("gain %v out of (0, 1] at env %v", g, env)
		}
	}

	buf := NewStereoBuffer(128)
	buf.SetLen(128)
	for i := range buf.Left {
		buf.Left[i] = 0.8
		buf.Right[i] = 0.8
	}
	allocs := testing.AllocsPerRun(50, func() {
		l.ProcessStereo(buf)
	})
	if allocs != 0 {
		t.Fatalf("limiter ProcessStereo allocated %v, want 0", allocs)
	}
}

func TestBlock2EQPreservesState(t *testing.T) {
	spec1 := EQSpec{MidPeak: EQBandSpec{Freq: 1000, GainDB: 6, Q: 1}}
	spec2 := EQSpec{MidPeak: EQBandSpec{Freq: 1200, GainDB: 3, Q: 1}}

	eq := NewEQ(48000, spec1)
	for i := 0; i < 500; i++ {
		x := float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
		eq.ProcessSample(x)
	}

	savedX1 := eq.mid.x1
	savedX2 := eq.mid.x2
	savedY1 := eq.mid.y1
	savedY2 := eq.mid.y2

	if savedX1 == 0 && savedX2 == 0 && savedY1 == 0 && savedY2 == 0 {
		t.Fatal("biquad state registers unexpectedly zero after feeding signal")
	}

	eq.SetSpec(spec2)

	if eq.mid.x1 != savedX1 || eq.mid.x2 != savedX2 || eq.mid.y1 != savedY1 || eq.mid.y2 != savedY2 {
		t.Fatalf("SetSpec reset biquad delay registers: got (%v, %v, %v, %v), want (%v, %v, %v, %v)",
			eq.mid.x1, eq.mid.x2, eq.mid.y1, eq.mid.y2, savedX1, savedX2, savedY1, savedY2)
	}

	y := eq.ProcessSample(float32(math.Sin(2 * math.Pi * 1000 * 501 / 48000)))
	if math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) {
		t.Fatalf("output after SetSpec is NaN or Inf: %v", y)
	}
}

func TestBlock2StereoDelayFractional(t *testing.T) {
	d := NewStereoDelay(48000, 1.0)
	d.Configure(DelayConfig{Mode: DelayModeStereo, Feedback: 0, DampingHz: 20000, Wet: 1.0}, 120)

	d.SetDelayFractional(10.5)
	if math.Abs(d.DelayFractional()-10.5) > 1e-6 {
		t.Fatalf("DelayFractional = %v, want 10.5", d.DelayFractional())
	}
	if d.LengthSamples() != 11 {
		t.Fatalf("LengthSamples = %d, want 11", d.LengthSamples())
	}

	d.Reset()
	d.ProcessFrameWet(1.0, 1.0)
	var outL10, outL11 float32
	for i := 1; i <= 20; i++ {
		l, _ := d.ProcessFrameWet(0, 0)
		if i == 10 {
			outL10 = l
		}
		if i == 11 {
			outL11 = l
		}
	}

	if math.Abs(float64(outL10-0.5)) > 0.05 {
		t.Fatalf("frame 10 = %v, want ~0.5", outL10)
	}
	if math.Abs(float64(outL11-0.5)) > 0.05 {
		t.Fatalf("frame 11 = %v, want ~0.5", outL11)
	}

	d.Reset()
	buf := NewStereoBuffer(100)
	buf.SetLen(100)
	for i := 0; i < 100; i++ {
		d.SetDelayFractional(10.0 + float64(i)*0.01)
		l, r := d.ProcessFrame(0.5, 0.5)
		if math.IsNaN(float64(l)) || math.IsNaN(float64(r)) {
			t.Fatalf("modulation produced NaN at frame %d", i)
		}
	}

	d.Reset()
	d.SetDelayFractional(0.5)
	d.ProcessFrameWet(1.0, 1.0)
	wL, _ := d.ProcessFrameWet(0, 0)
	if math.Abs(float64(wL-0.927)) > 0.05 {
		t.Fatalf("sub-sample delay clamped got %v, want ~0.927", wL)
	}

	d.Reset()
	d.SetDelayFractional(float64(d.mask))
	for i := 0; i < 10; i++ {
		l, r := d.ProcessFrameWet(0.5, 0.5)
		if math.IsNaN(float64(l)) || math.IsNaN(float64(r)) {
			t.Fatal("max capacity delay produced NaN")
		}
	}

	allocs := testing.AllocsPerRun(50, func() {
		d.SetDelayFractional(15.25)
		d.ProcessFrame(0.1, 0.1)
	})
	if allocs != 0 {
		t.Fatalf("SetDelayFractional/ProcessFrame allocated %v, want 0", allocs)
	}
}
