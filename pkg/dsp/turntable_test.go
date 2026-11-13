package dsp

import (
	"math"
	"testing"
)

func TestTurntableBypass(t *testing.T) {
	tt := NewTurntable(48000)
	inL := make([]float32, 256)
	inR := make([]float32, 256)
	for i := range inL {
		inL[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / 48000))
		inR[i] = float32(math.Cos(2 * math.Pi * 440 * float64(i) / 48000))
	}
	outL := make([]float32, 256)
	outR := make([]float32, 256)
	tt.Process(inL, inR, outL, outR)
	for i := range inL {
		if outL[i] != inL[i] || outR[i] != inR[i] {
			t.Fatalf("bypass mismatch at %d: inL=%f outL=%f", i, inL[i], outL[i])
		}
	}
}

func TestTurntableBrakeLinear(t *testing.T) {
	tt := NewTurntable(48000)
	frames := 4800
	inL := make([]float32, frames)
	inR := make([]float32, frames)
	for i := range inL {
		inL[i] = 0.5
		inR[i] = 0.5
	}
	tt.Brake(50.0, "linear")
	if tt.Mode() != TurntableModeBrake {
		t.Fatalf("expected Brake mode, got %v", tt.Mode())
	}
	outL := make([]float32, frames)
	outR := make([]float32, frames)
	tt.Process(inL, inR, outL, outR)
	last := outL[len(outL)-1]
	if last != 0 {
		t.Fatalf("expected 0 after brake complete, got %f", last)
	}
	if tt.Speed() != 0.0 {
		t.Fatalf("expected speed 0.0, got %f", tt.Speed())
	}
}

func TestTurntableBrakeExponential(t *testing.T) {
	tt := NewTurntable(48000)
	frames := 4800
	inL := make([]float32, frames)
	inR := make([]float32, frames)
	for i := range inL {
		inL[i] = 0.8
		inR[i] = 0.8
	}
	tt.Brake(80.0, "exponential")
	tt.ProcessStereo(inL, inR)
	last := inL[len(inL)-1]
	if last != 0 {
		t.Fatalf("expected 0 after exponential brake complete, got %f", last)
	}
}

func TestTurntableReleaseBrake(t *testing.T) {
	tt := NewTurntable(48000)
	bufL := make([]float32, 1024)
	bufR := make([]float32, 1024)
	for i := range bufL {
		bufL[i] = 0.5
		bufR[i] = 0.5
	}
	tt.Brake(10.0, "linear")
	tt.ProcessStereo(bufL, bufR)
	tt.ReleaseBrake(0, "")
	if tt.Mode() != TurntableModeBypass {
		t.Fatalf("expected Bypass mode, got %v", tt.Mode())
	}
	tt.ReleaseBrake(20.0, "exponential")
	if tt.Mode() != TurntableModeStart {
		t.Fatalf("expected Start mode, got %v", tt.Mode())
	}
	tt.ProcessStereo(bufL, bufR)
}

func TestTurntableScratchForwardAndBackward(t *testing.T) {
	tt := NewTurntable(48000)
	tt.SetInertiaMs(10.0)
	if tt.InertiaMs() != 10.0 {
		t.Fatalf("inertia mismatch: got %f, want 10.0", tt.InertiaMs())
	}
	buf := make([]float32, 2048)
	for i := range buf {
		buf[i] = float32(math.Sin(float64(i) * 0.1))
	}
	tt.ProcessInterleaved(buf)

	tt.Scratch(-2.5)
	if tt.Mode() != TurntableModeScratch {
		t.Fatalf("expected Scratch mode, got %v", tt.Mode())
	}
	scratchBuf := make([]float32, 1024)
	tt.ProcessInterleaved(scratchBuf)
	if tt.Speed() > -1.0 {
		t.Fatalf("scratch speed did not decelerate: %f", tt.Speed())
	}

	tt.Scratch(2.0)
	tt.ProcessInterleaved(scratchBuf)
	if tt.Speed() < 0.0 {
		t.Fatalf("scratch speed did not accelerate: %f", tt.Speed())
	}
}

func TestTurntableBeatRollSeamless(t *testing.T) {
	tt := NewTurntable(48000)
	seedL := make([]float32, 4800)
	seedR := make([]float32, 4800)
	for i := range seedL {
		seedL[i] = float32(math.Sin(2 * math.Pi * 220 * float64(i) / 48000))
		seedR[i] = float32(math.Cos(2 * math.Pi * 220 * float64(i) / 48000))
	}
	tt.ProcessStereo(seedL, seedR)

	subs := []Subdivision{
		SubdivisionQuarter,
		SubdivisionEighth,
		SubdivisionSixteenth,
		SubdivisionThirtySecond,
	}

	for _, sub := range subs {
		tt.BeatRoll(true, sub, 120.0)
		if tt.Mode() != TurntableModeBeatRoll {
			t.Fatalf("expected BeatRoll mode for sub %v", sub)
		}
		rollL := make([]float32, 1024)
		rollR := make([]float32, 1024)
		tt.ProcessStereo(rollL, rollR)
		for j := range rollL {
			if math.IsNaN(float64(rollL[j])) || math.IsInf(float64(rollL[j]), 0) {
				t.Fatalf("NaN/Inf in beat roll output at %d", j)
			}
		}
	}

	tt.BeatRoll(false, SubdivisionQuarter, 120.0)
	if tt.Mode() != TurntableModeBypass {
		t.Fatalf("expected Bypass mode after roll release")
	}
}

func TestTurntableBackspinDecay(t *testing.T) {
	tt := NewTurntable(48000)
	buf := make([]float32, 4800)
	for i := range buf {
		buf[i] = 0.5
	}
	tt.ProcessInterleaved(buf)

	tt.Backspin(-3.0, 100.0)
	if tt.Mode() != TurntableModeBackspin {
		t.Fatalf("expected Backspin mode")
	}
	out := make([]float32, 10000)
	tt.ProcessInterleaved(out)
	if tt.Mode() != TurntableModeBypass {
		t.Fatalf("expected Bypass mode after backspin finishes, got %v", tt.Mode())
	}
}

func TestTurntableReset(t *testing.T) {
	tt := NewTurntable(48000)
	tt.Brake(100.0, "linear")
	tt.Reset()
	if tt.Mode() != TurntableModeBypass {
		t.Fatalf("expected Bypass after reset")
	}
	if tt.Speed() != 1.0 {
		t.Fatalf("expected speed 1.0 after reset")
	}
}

func TestTurntableBypassMethod(t *testing.T) {
	tt := NewTurntable(48000)
	tt.Scratch(-2.0)
	tt.Bypass()
	if tt.Mode() != TurntableModeBypass {
		t.Fatalf("expected Bypass after Bypass()")
	}
	if tt.Speed() != 1.0 {
		t.Fatalf("expected speed 1.0 after Bypass()")
	}
}

func TestTurntableScratchZeroCrossingSmoothness(t *testing.T) {
	tt := NewTurntable(48000)
	tt.SetInertiaMs(0.0)
	warmup := make([]float32, 1024)
	for i := range warmup {
		warmup[i] = 0.8
	}
	tt.ProcessInterleaved(warmup)

	tt.Scratch(0.04)
	buf1 := make([]float32, 2)
	buf1[0] = 0.8
	buf1[1] = 0.8
	tt.ProcessInterleaved(buf1)

	tt.Scratch(0.002)
	buf2 := make([]float32, 2)
	buf2[0] = 0.8
	buf2[1] = 0.8
	tt.ProcessInterleaved(buf2)

	diff := math.Abs(float64(buf1[0] - buf2[0]))
	if diff > 0.8 {
		t.Fatalf("detected step discontinuity across near-zero scratch speed: diff=%f", diff)
	}
}

func TestTurntableAllocations(t *testing.T) {
	tt := NewTurntable(48000)
	inL := make([]float32, 256)
	inR := make([]float32, 256)
	outL := make([]float32, 256)
	outR := make([]float32, 256)
	stereo := make([]float32, 512)

	tt.Scratch(1.5)

	allocsStereo := testing.AllocsPerRun(100, func() {
		tt.ProcessStereo(inL, inR)
	})
	if allocsStereo != 0 {
		t.Fatalf("ProcessStereo allocated %f times, want 0", allocsStereo)
	}

	allocsInterleaved := testing.AllocsPerRun(100, func() {
		tt.ProcessInterleaved(stereo)
	})
	if allocsInterleaved != 0 {
		t.Fatalf("ProcessInterleaved allocated %f times, want 0", allocsInterleaved)
	}

	allocsProcess := testing.AllocsPerRun(100, func() {
		tt.Process(inL, inR, outL, outR)
	})
	if allocsProcess != 0 {
		t.Fatalf("Process allocated %f times, want 0", allocsProcess)
	}
}

func BenchmarkTurntableProcessStereo(b *testing.B) {
	tt := NewTurntable(48000)
	tt.Scratch(-1.5)
	left := make([]float32, 256)
	right := make([]float32, 256)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tt.ProcessStereo(left, right)
	}
}

func BenchmarkTurntableProcessInterleaved(b *testing.B) {
	tt := NewTurntable(48000)
	tt.Brake(500.0, "exponential")
	stereo := make([]float32, 512)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tt.ProcessInterleaved(stereo)
	}
}
