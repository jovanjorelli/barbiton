package dsp

import (
	"math"
	"testing"
)

func TestBlock4TruePeakDetectionInterSample(t *testing.T) {
	const sr = 48000
	const n = 1024
	l := NewLimiter(sr)
	l.SetAutoGain(false)
	l.SetThresholdDB(-3.0)
	l.SetCeiling(0.85)

	inL := make([]float32, n)
	inR := make([]float32, n)
	for i := 0; i < n; i++ {
		v := float32(0.9 * math.Sin(math.Pi*float64(i)/2.0+math.Pi/4.0))
		inL[i] = v
		inR[i] = v
	}

	buf := NewStereoBuffer(n)
	buf.SetLen(n)
	copy(buf.Left, inL)
	copy(buf.Right, inR)
	l.ProcessStereo(buf)

	if l.Gain() >= 0.99 {
		t.Fatalf("expected gain reduction for true peak, got gain %f", l.Gain())
	}
	for i := l.DelayFrames() + 20; i < n; i++ {
		if absF32(buf.Left[i]) > 0.8501 || absF32(buf.Right[i]) > 0.8501 {
			t.Fatalf("ceiling breached at %d: %f / %f", i, buf.Left[i], buf.Right[i])
		}
	}
}

func TestBlock4TruePeakAdjacentImpulse(t *testing.T) {
	const sr = 48000
	const n = 512
	l := NewLimiter(sr)
	l.SetAutoGain(false)
	l.SetThresholdDB(-6.0)
	l.SetCeiling(0.85)

	buf := NewStereoBuffer(n)
	buf.SetLen(n)
	for i := 100; i < 200; i++ {
		v := float32(0.9 * math.Sin(math.Pi*float64(i)/2.0+math.Pi/4.0))
		buf.Left[i] = v
		buf.Right[i] = v
	}
	minGain := float32(1.0)
	for i := 0; i < n; i++ {
		buf.Left[i], buf.Right[i] = l.processFrame(buf.Left[i], buf.Right[i])
		if g := l.Gain(); g < minGain {
			minGain = g
		}
	}

	if minGain >= 0.95 {
		t.Fatalf("expected gain reduction for adjacent impulses producing true peak, got min gain %f", minGain)
	}
	for i := range buf.Left {
		if absF32(buf.Left[i]) > 0.8501 || absF32(buf.Right[i]) > 0.8501 {
			t.Fatalf("ceiling exceeded at %d: %f / %f", i, buf.Left[i], buf.Right[i])
		}
	}
}

func TestBlock4LimiterExtremeTruePeakAndCeiling(t *testing.T) {
	const sr = 48000
	const n = 2048
	l := NewLimiter(sr)
	l.SetAutoGain(false)
	l.SetThresholdDB(-6.0)
	l.SetCeiling(0.988)

	in := make([]float32, n)
	for i := 0; i < n; i++ {
		if i%4 == 1 || i%4 == 2 {
			in[i] = 4.0
		} else {
			in[i] = -4.0
		}
	}

	interleaved := make([]float32, n*2)
	for i := 0; i < n; i++ {
		interleaved[2*i] = in[i]
		interleaved[2*i+1] = -in[i]
	}
	l.ProcessInterleaved(interleaved)

	for i := l.DelayFrames() + 50; i < n; i++ {
		outL := interleaved[2*i]
		outR := interleaved[2*i+1]
		if absF32(outL) > 0.9881 || absF32(outR) > 0.9881 {
			t.Fatalf("ceiling violated at %d: L=%f R=%f", i, outL, outR)
		}
	}
}

func TestBlock4LimiterResetClearsTruePeakHistory(t *testing.T) {
	l := NewLimiter(48000)
	l.processFrame(0.9, -0.9)
	hasNonZero := false
	for j := 0; j < 12; j++ {
		if l.tpHistL[j] != 0 || l.tpHistR[j] != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Fatal("expected non-zero true peak history after processing")
	}

	l.Reset()
	for j := 0; j < 12; j++ {
		if l.tpHistL[j] != 0 || l.tpHistR[j] != 0 {
			t.Fatalf("history not cleared at %d: L=%f R=%f", j, l.tpHistL[j], l.tpHistR[j])
		}
	}
}

func BenchmarkBlock4LimiterProcessStereo(b *testing.B) {
	l := NewLimiter(48000)
	l.SetAutoGain(false)
	buf := NewStereoBuffer(512)
	buf.SetLen(512)
	for i := 0; i < 512; i++ {
		buf.Left[i] = 0.5
		buf.Right[i] = -0.5
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.ProcessStereo(buf)
	}
}

func BenchmarkBlock4LimiterProcessInterleaved(b *testing.B) {
	l := NewLimiter(48000)
	l.SetAutoGain(false)
	data := make([]float32, 1024)
	for i := range data {
		data[i] = 0.5
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.ProcessInterleaved(data)
	}
}
