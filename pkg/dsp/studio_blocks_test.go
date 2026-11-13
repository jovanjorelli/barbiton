package dsp

import (
	"math"
	"testing"
)

func TestSidechainCompressorDucking(t *testing.T) {
	cfg := DefaultSidechainConfig()
	cfg.ThresholdDB = -20
	cfg.Ratio = 4
	cfg.AttackMs = 1
	cfg.ReleaseMs = 50
	cfg.HoldMs = 5
	sc := NewSidechainCompressor(48000, cfg)

	targetL := make([]float32, 480)
	targetR := make([]float32, 480)
	sidechainL := make([]float32, 480)
	sidechainR := make([]float32, 480)

	for i := range targetL {
		targetL[i] = 1.0
		targetR[i] = 1.0
		sidechainL[i] = 0.8
		sidechainR[i] = 0.8
	}

	sc.Process(targetL, targetR, sidechainL, sidechainR)

	if targetL[len(targetL)-1] >= 0.9 {
		t.Fatalf("expected ducking reduction, got %g", targetL[len(targetL)-1])
	}
}

func TestSidechainCompressorZeroAlloc(t *testing.T) {
	cfg := DefaultSidechainConfig()
	sc := NewSidechainCompressor(48000, cfg)

	targetL := make([]float32, 256)
	targetR := make([]float32, 256)
	scL := make([]float32, 256)
	scR := make([]float32, 256)
	inter := make([]float32, 512)

	for i := range targetL {
		targetL[i] = 0.5
		targetR[i] = 0.5
		scL[i] = 0.9
		scR[i] = 0.9
		inter[2*i] = 0.5
		inter[2*i+1] = 0.5
	}

	allocs := testing.AllocsPerRun(100, func() {
		sc.Process(targetL, targetR, scL, scR)
	})
	if allocs != 0 {
		t.Fatalf("SidechainCompressor.Process allocated %v, want 0", allocs)
	}

	allocsInter := testing.AllocsPerRun(100, func() {
		sc.ProcessInterleaved(inter, inter)
	})
	if allocsInter != 0 {
		t.Fatalf("SidechainCompressor.ProcessInterleaved allocated %v, want 0", allocsInter)
	}
}

func TestSidechainSanitizeNaNInf(t *testing.T) {
	sc := NewSidechainCompressor(48000, DefaultSidechainConfig())
	oL, oR := sc.ProcessFrame(float32(math.NaN()), float32(math.Inf(1)), float32(math.NaN()), float32(math.Inf(-1)))
	if math.IsNaN(float64(oL)) || math.IsInf(float64(oL), 0) || math.IsNaN(float64(oR)) || math.IsInf(float64(oR), 0) {
		t.Fatalf("expected clean 0 on NaN/Inf, got %g, %g", oL, oR)
	}
}

func TestConvolutionReverbDiracDelta(t *testing.T) {
	cfg := DefaultConvolutionConfig()
	cfg.DryWet = 1.0
	cfg.PreDelayMs = 0
	cfg.LowCutHz = 20
	cfg.HighCutHz = 20000
	cr := NewConvolutionReverb(48000, cfg)

	ir := make([]float32, 256)
	ir[0] = 1.0
	cr.SetImpulseResponse(ir, ir)

	inL := make([]float32, 256)
	inR := make([]float32, 256)
	outL := make([]float32, 256)
	outR := make([]float32, 256)
	inL[0] = 0.5
	inR[0] = 0.5

	cr.Process(inL, inR, outL, outR)
	cr.Process(inL, inR, outL, outR)

	hasSignal := false
	for _, v := range outL {
		if v != 0 {
			hasSignal = true
			break
		}
	}
	if !hasSignal {
		t.Fatalf("expected non-zero convolution response")
	}
}

func TestConvolutionReverbZeroAlloc(t *testing.T) {
	cfg := DefaultConvolutionConfig()
	cr := NewConvolutionReverb(48000, cfg)
	ir := make([]float32, 512)
	ir[0] = 1.0
	cr.SetImpulseResponse(ir, ir)

	inL := make([]float32, 128)
	inR := make([]float32, 128)
	outL := make([]float32, 128)
	outR := make([]float32, 128)
	inter := make([]float32, 256)

	allocs := testing.AllocsPerRun(100, func() {
		cr.Process(inL, inR, outL, outR)
	})
	if allocs != 0 {
		t.Fatalf("ConvolutionReverb.Process allocated %v, want 0", allocs)
	}

	allocsInter := testing.AllocsPerRun(100, func() {
		cr.ProcessInterleaved(inter, inter)
	})
	if allocsInter != 0 {
		t.Fatalf("ConvolutionReverb.ProcessInterleaved allocated %v, want 0", allocsInter)
	}
}

func TestMultibandDynamicsPhaseAlignment(t *testing.T) {
	cfg := DefaultMultibandConfig()
	cfg.LowBand.ThresholdDB = 0
	cfg.MidBand.ThresholdDB = 0
	cfg.HighBand.ThresholdDB = 0
	cfg.LowBand.Ratio = 1
	cfg.MidBand.Ratio = 1
	cfg.HighBand.Ratio = 1
	cfg.LowBand.AutoMakeup = false
	cfg.MidBand.AutoMakeup = false
	cfg.HighBand.AutoMakeup = false
	cfg.LowBand.Saturation = 0
	cfg.MidBand.Saturation = 0
	cfg.HighBand.Saturation = 0

	mb := NewMultibandDynamics(48000, cfg)

	for i := 0; i < 500; i++ {
		mb.ProcessFrame(0, 0)
	}

	freqs := []float64{60, 120, 500, 1000, 3500, 8000}
	for _, f := range freqs {
		mb.Reset()
		w := 2 * math.Pi * f / 48000.0
		var maxAmp float32
		for n := 0; n < 2000; n++ {
			in := float32(math.Sin(w * float64(n)))
			oL, _ := mb.ProcessFrame(in, in)
			if n > 1000 {
				a := oL
				if a < 0 {
					a = -a
				}
				if a > maxAmp {
					maxAmp = a
				}
			}
		}
		if maxAmp < 0.90 || maxAmp > 1.10 {
			t.Fatalf("crossover sum at %g Hz deviates from unity: amp = %g", f, maxAmp)
		}
	}
}

func TestMultibandDynamicsZeroAlloc(t *testing.T) {
	mb := NewMultibandDynamics(48000, DefaultMultibandConfig())
	inter := make([]float32, 256)
	for i := range inter {
		inter[i] = 0.5
	}

	allocs := testing.AllocsPerRun(100, func() {
		mb.ProcessInterleaved(inter)
	})
	if allocs != 0 {
		t.Fatalf("MultibandDynamics.ProcessInterleaved allocated %v, want 0", allocs)
	}
}

func TestOversampledSaturatorAntiAliasing(t *testing.T) {
	sat := NewOversampledSaturator(0.5)
	sat.SetDrive(3.0)

	inter := make([]float32, 256)
	for i := range inter {
		inter[i] = float32(math.Sin(float64(i) * 0.2))
	}

	allocs := testing.AllocsPerRun(100, func() {
		sat.ProcessInterleaved(inter)
	})
	if allocs != 0 {
		t.Fatalf("OversampledSaturator.ProcessInterleaved allocated %v, want 0", allocs)
	}

	peak := float32(0)
	for _, v := range inter {
		a := v
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	if peak > 1.0 {
		t.Fatalf("saturation output exceeded 1.0, got %g", peak)
	}
}

func BenchmarkSidechainProcess(b *testing.B) {
	sc := NewSidechainCompressor(48000, DefaultSidechainConfig())
	target := make([]float32, 1024)
	sidechain := make([]float32, 1024)
	for i := range target {
		target[i] = 0.5
		sidechain[i] = 0.8
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sc.ProcessInterleaved(target, sidechain)
	}
}

func BenchmarkConvolutionProcess(b *testing.B) {
	cr := NewConvolutionReverb(48000, DefaultConvolutionConfig())
	ir := make([]float32, 512)
	ir[0] = 1.0
	cr.SetImpulseResponse(ir, ir)
	in := make([]float32, 256)
	out := make([]float32, 256)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cr.ProcessInterleaved(in, out)
	}
}

func BenchmarkMultibandProcess(b *testing.B) {
	mb := NewMultibandDynamics(48000, DefaultMultibandConfig())
	buf := make([]float32, 512)
	for i := range buf {
		buf[i] = 0.5
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		mb.ProcessInterleaved(buf)
	}
}

func BenchmarkOversampledSaturatorProcess(b *testing.B) {
	sat := NewOversampledSaturator(0.9)
	buf := make([]float32, 512)
	for i := range buf {
		buf[i] = 0.8
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sat.ProcessInterleaved(buf)
	}
}
