package dsp

import (
	"math"
	"testing"
)

func TestDistortionModes(t *testing.T) {
	sr := 48000.0
	modes := []DistortionMode{DistortionDiode, DistortionTube, DistortionHardClip, DistortionFoldback}

	for _, mode := range modes {
		cfg := DefaultDistortionConfig()
		cfg.Mode = mode
		cfg.Drive = 5.0
		cfg.Mix = 1.0
		dist := NewDistortion(sr, cfg)

		inL, inR := float32(0.8), float32(-0.8)
		for i := 0; i < 100; i++ {
			oL, oR := dist.ProcessFrame(inL, inR)
			if math.IsNaN(float64(oL)) || math.IsInf(float64(oL), 0) {
				t.Fatalf("mode %v produced NaN/Inf left: %g", mode, oL)
			}
			if math.IsNaN(float64(oR)) || math.IsInf(float64(oR), 0) {
				t.Fatalf("mode %v produced NaN/Inf right: %g", mode, oR)
			}
		}
	}
}

func TestDistortionMixBypass(t *testing.T) {
	sr := 48000.0
	cfg := DefaultDistortionConfig()
	cfg.Mix = 0.0
	dist := NewDistortion(sr, cfg)

	inL, inR := float32(0.42), float32(-0.73)
	outL, outR := dist.ProcessFrame(inL, inR)
	if outL != inL || outR != inR {
		t.Fatalf("mix 0 failed: got (%g, %g), want (%g, %g)", outL, outR, inL, inR)
	}
}

func TestDistortionHardClipShape(t *testing.T) {
	res := shapeSample(2.5, DistortionHardClip)
	if res != 1.0 {
		t.Fatalf("hardclip positive want 1.0, got %g", res)
	}
	resNeg := shapeSample(-3.0, DistortionHardClip)
	if resNeg != -1.0 {
		t.Fatalf("hardclip negative want -1.0, got %g", resNeg)
	}
}

func TestDistortionFoldbackShape(t *testing.T) {
	res := shapeSample(1.5, DistortionFoldback)
	if res >= 1.0 || res <= -1.0 {
		t.Fatalf("foldback of 1.5 must fold back into (-1, 1), got %g", res)
	}
}

func TestDistortionAllocs(t *testing.T) {
	sr := 48000.0
	dist := NewDistortion(sr, DefaultDistortionConfig())
	buf := make([]float32, 2048)

	allocs := testing.AllocsPerRun(100, func() {
		dist.ProcessInterleaved(buf)
	})
	if allocs != 0 {
		t.Fatalf("ProcessInterleaved allocated %g heap objects, want 0", allocs)
	}

	allocsFrame := testing.AllocsPerRun(1000, func() {
		_, _ = dist.ProcessFrame(0.5, -0.5)
	})
	if allocsFrame != 0 {
		t.Fatalf("ProcessFrame allocated %g heap objects, want 0", allocsFrame)
	}
}

func TestDistortionToneFiltering(t *testing.T) {
	sr := 48000.0
	cfg := DefaultDistortionConfig()
	cfg.Mix = 1.0
	cfg.Tone = 8000.0
	distHz := NewDistortion(sr, cfg)
	if distHz.Tone() != 8000.0 {
		t.Fatalf("Tone in Hz got %g, want 8000.0", distHz.Tone())
	}

	cfgNorm := DefaultDistortionConfig()
	cfgNorm.Mix = 1.0
	cfgNorm.Tone = 0.5
	distNorm := NewDistortion(sr, cfgNorm)
	if distNorm.Tone() != 0.5 {
		t.Fatalf("Tone normalized got %g, want 0.5", distNorm.Tone())
	}

	cfgZero := DefaultDistortionConfig()
	cfgZero.Tone = 0
	distZero := NewDistortion(sr, cfgZero)
	if distZero.Tone() != 0 {
		t.Fatalf("Tone zero got %g, want 0", distZero.Tone())
	}
}

func BenchmarkDistortionProcessInterleaved(b *testing.B) {
	sr := 48000.0
	dist := NewDistortion(sr, DefaultDistortionConfig())
	buf := make([]float32, 2048)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dist.ProcessInterleaved(buf)
	}
}
