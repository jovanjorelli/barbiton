package dsp

import (
	"math"
	"testing"
)

func TestOTTNeutralCrossover(t *testing.T) {
	sr := 48000.0
	cfg := DefaultOTTConfig()
	cfg.Depth = 1.0
	cfg.InGainDB = 0
	cfg.OutGainDB = 0
	ott := NewOTT(sr, cfg)

	bufL := make([]float32, 1024)
	bufR := make([]float32, 1024)
	for i := range bufL {
		bufL[i] = float32(0.1 * math.Sin(2.0*math.Pi*1000.0*float64(i)/sr))
		bufR[i] = bufL[i]
	}

	for warm := 0; warm < 5; warm++ {
		ott.ProcessStereo(bufL, bufR)
	}

	for i := range bufL {
		if math.IsNaN(float64(bufL[i])) || math.IsInf(float64(bufL[i]), 0) {
			t.Fatalf("NaN/Inf at sample %d", i)
		}
	}
}

func TestOTTUpwardAndDownwardCompression(t *testing.T) {
	sr := 48000.0
	cfg := DefaultOTTConfig()
	cfg.Depth = 1.0
	ott := NewOTT(sr, cfg)

	quietL, quietR := float32(0.01), float32(0.01)
	var qOutL float32
	for i := 0; i < 2000; i++ {
		qOutL, _ = ott.ProcessFrame(quietL, quietR)
	}
	if qOutL <= quietL {
		t.Fatalf("expected upward compression to boost quiet signal: in=%g, out=%g", quietL, qOutL)
	}

	ott.Reset()
	loudL, loudR := float32(1.5), float32(1.5)
	var lOutL float32
	for i := 0; i < 2000; i++ {
		lOutL, _ = ott.ProcessFrame(loudL, loudR)
	}
	if lOutL >= loudL {
		t.Fatalf("expected downward compression to attenuate loud signal: in=%g, out=%g", loudL, lOutL)
	}
}

func TestOTTDepthBypass(t *testing.T) {
	sr := 48000.0
	cfg := DefaultOTTConfig()
	cfg.Depth = 0.0
	ott := NewOTT(sr, cfg)

	inL, inR := float32(0.5), float32(-0.3)
	outL, outR := ott.ProcessFrame(inL, inR)
	if outL != inL || outR != inR {
		t.Fatalf("depth 0 failed: got (%g, %g), want (%g, %g)", outL, outR, inL, inR)
	}
}

func TestOTTAllocs(t *testing.T) {
	sr := 48000.0
	ott := NewOTT(sr, DefaultOTTConfig())
	buf := make([]float32, 2048)

	allocs := testing.AllocsPerRun(100, func() {
		ott.ProcessInterleaved(buf)
	})
	if allocs != 0 {
		t.Fatalf("ProcessInterleaved allocated %g heap objects, want 0", allocs)
	}

	allocsFrame := testing.AllocsPerRun(1000, func() {
		_, _ = ott.ProcessFrame(0.5, -0.5)
	})
	if allocsFrame != 0 {
		t.Fatalf("ProcessFrame allocated %g heap objects, want 0", allocsFrame)
	}
}

func BenchmarkOTTProcessInterleaved(b *testing.B) {
	sr := 48000.0
	ott := NewOTT(sr, DefaultOTTConfig())
	buf := make([]float32, 2048)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ott.ProcessInterleaved(buf)
	}
}
