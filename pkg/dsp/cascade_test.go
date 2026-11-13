package dsp

import (
	"math"
	"testing"
)

func TestCascadeButterworthQDistribution(t *testing.T) {
	c4 := NewCascadeFilter(2, Lowpass, 1000, 48000)
	if c4.StageCount() != 2 {
		t.Fatalf("expected 2 stages, got %d", c4.StageCount())
	}
	if math.Abs(c4.Stage(0).Q()-butterworth4PQ1) > 1e-6 {
		t.Fatalf("stage 0 Q = %v, want %v", c4.Stage(0).Q(), butterworth4PQ1)
	}
	if math.Abs(c4.Stage(1).Q()-butterworth4PQ2) > 1e-6 {
		t.Fatalf("stage 1 Q = %v, want %v", c4.Stage(1).Q(), butterworth4PQ2)
	}

	c6 := NewCascadeFilter(3, Lowpass, 1000, 48000)
	if c6.StageCount() != 3 {
		t.Fatalf("expected 3 stages, got %d", c6.StageCount())
	}
	if math.Abs(c6.Stage(0).Q()-butterworth6PQ1) > 1e-6 {
		t.Fatalf("stage 0 Q = %v, want %v", c6.Stage(0).Q(), butterworth6PQ1)
	}
	if math.Abs(c6.Stage(1).Q()-butterworth6PQ2) > 1e-6 {
		t.Fatalf("stage 1 Q = %v, want %v", c6.Stage(1).Q(), butterworth6PQ2)
	}
	if math.Abs(c6.Stage(2).Q()-butterworth6PQ3) > 1e-6 {
		t.Fatalf("stage 2 Q = %v, want %v", c6.Stage(2).Q(), butterworth6PQ3)
	}
}

func TestCascadeAttenuationSlopes(t *testing.T) {
	fc := 1000.0
	fs := 48000.0
	fOctaveAbove := 2000.0
	fOctaveBelow := 500.0

	lp1 := NewCascadeFilter(1, Lowpass1P, fc, fs)
	lp2 := NewCascadeFilter(1, Lowpass, fc, fs)
	lp4 := NewCascadeFilter(2, Lowpass, fc, fs)
	lp6 := NewCascadeFilter(3, Lowpass, fc, fs)

	resp1Cutoff := lp1.ResponseAt(fc)
	resp2Cutoff := lp2.ResponseAt(fc)
	resp4Cutoff := lp4.ResponseAt(fc)
	resp6Cutoff := lp6.ResponseAt(fc)

	if math.Abs(resp1Cutoff-0.7071) > 0.05 {
		t.Fatalf("lp1 cutoff response = %v, want ~0.7071", resp1Cutoff)
	}
	if math.Abs(resp2Cutoff-0.7071) > 0.05 {
		t.Fatalf("lp2 cutoff response = %v, want ~0.7071", resp2Cutoff)
	}
	if math.Abs(resp4Cutoff-0.7071) > 0.05 {
		t.Fatalf("lp4 cutoff response = %v, want ~0.7071", resp4Cutoff)
	}
	if math.Abs(resp6Cutoff-0.7071) > 0.05 {
		t.Fatalf("lp6 cutoff response = %v, want ~0.7071", resp6Cutoff)
	}

	resp1Above := lp1.ResponseAt(fOctaveAbove)
	resp2Above := lp2.ResponseAt(fOctaveAbove)
	resp4Above := lp4.ResponseAt(fOctaveAbove)
	resp6Above := lp6.ResponseAt(fOctaveAbove)

	db1 := 20 * math.Log10(resp1Above)
	db2 := 20 * math.Log10(resp2Above)
	db4 := 20 * math.Log10(resp4Above)
	db6 := 20 * math.Log10(resp6Above)

	if math.Abs(db1-(-7.0)) > 1.0 {
		t.Fatalf("lp1 at 2kHz = %v dB, want ~-7 dB", db1)
	}
	if math.Abs(db2-(-12.3)) > 1.5 {
		t.Fatalf("lp2 at 2kHz = %v dB, want ~-12 dB", db2)
	}
	if math.Abs(db4-(-24.5)) > 2.0 {
		t.Fatalf("lp4 at 2kHz = %v dB, want ~-24 dB", db4)
	}
	if math.Abs(db6-(-36.6)) > 3.0 {
		t.Fatalf("lp6 at 2kHz = %v dB, want ~-36 dB", db6)
	}

	if !(resp6Above < resp4Above && resp4Above < resp2Above && resp2Above < resp1Above) {
		t.Fatalf("expected monotonic attenuation lp6 < lp4 < lp2 < lp1, got %v, %v, %v, %v", resp6Above, resp4Above, resp2Above, resp1Above)
	}

	hp1 := NewCascadeFilter(1, Highpass1P, fc, fs)
	hp2 := NewCascadeFilter(1, Highpass, fc, fs)
	hp4 := NewCascadeFilter(2, Highpass, fc, fs)
	hp6 := NewCascadeFilter(3, Highpass, fc, fs)

	respHp1Below := hp1.ResponseAt(fOctaveBelow)
	respHp2Below := hp2.ResponseAt(fOctaveBelow)
	respHp4Below := hp4.ResponseAt(fOctaveBelow)
	respHp6Below := hp6.ResponseAt(fOctaveBelow)

	hpDb1 := 20 * math.Log10(respHp1Below)
	hpDb2 := 20 * math.Log10(respHp2Below)
	hpDb4 := 20 * math.Log10(respHp4Below)
	hpDb6 := 20 * math.Log10(respHp6Below)

	if math.Abs(hpDb1-(-7.0)) > 1.0 {
		t.Fatalf("hp1 at 500Hz = %v dB, want ~-7 dB", hpDb1)
	}
	if math.Abs(hpDb2-(-12.3)) > 1.5 {
		t.Fatalf("hp2 at 500Hz = %v dB, want ~-12 dB", hpDb2)
	}
	if math.Abs(hpDb4-(-24.5)) > 2.0 {
		t.Fatalf("hp4 at 500Hz = %v dB, want ~-24 dB", hpDb4)
	}
	if math.Abs(hpDb6-(-36.6)) > 3.0 {
		t.Fatalf("hp6 at 500Hz = %v dB, want ~-36 dB", hpDb6)
	}
	if !(respHp6Below < respHp4Below && respHp4Below < respHp2Below && respHp2Below < respHp1Below) {
		t.Fatalf("expected monotonic HP attenuation hp6 < hp4 < hp2 < hp1, got %v, %v, %v, %v", respHp6Below, respHp4Below, respHp2Below, respHp1Below)
	}
}

func TestCascadeFilterMethodsAndResonance(t *testing.T) {
	c := NewCascadeFilter(0, Lowpass, 500, 44100)
	if c.StageCount() != 1 {
		t.Fatalf("clamped stage count got %d, want 1", c.StageCount())
	}
	c.SetStages(5)
	if c.StageCount() != 3 {
		t.Fatalf("clamped upper stage count got %d, want 3", c.StageCount())
	}
	if c.Stage(-1) != nil || c.Stage(10) != nil {
		t.Fatalf("invalid index Stage should return nil")
	}

	c.SetButterworth(2, Highpass, 800, 6.0, 48000)
	if c.Mode() != Highpass || c.Frequency() != 800 || c.SampleRate() != 48000 {
		t.Fatalf("state mismatch: mode=%v freq=%v sr=%v", c.Mode(), c.Frequency(), c.SampleRate())
	}
	resMult := math.Pow(10, 6.0/20.0)
	if math.Abs(c.Stage(0).Q()-(butterworth4PQ1*resMult)) > 1e-5 {
		t.Fatalf("resonance Q0 mismatch: %v vs %v", c.Stage(0).Q(), butterworth4PQ1*resMult)
	}

	c.SetFrequency(1200)
	if c.Frequency() != 1200 || c.Stage(0).Frequency() != 1200 || c.Stage(1).Frequency() != 1200 {
		t.Fatalf("SetFrequency not applied to stages")
	}

	c.SetMode(Lowpass)
	if c.Mode() != Lowpass || c.Stage(0).mode != Lowpass {
		t.Fatalf("SetMode not applied")
	}

	c.SetSampleRate(96000)
	if c.SampleRate() != 96000 || c.Stage(0).sampleRate != 96000 {
		t.Fatalf("SetSampleRate not applied")
	}

	c.SetQ(1.5)
	if c.Stage(0).Q() != 1.5 || c.Stage(1).Q() != 1.5 {
		t.Fatalf("SetQ not applied to all stages")
	}

	c.SetStageQ(1, 2.0)
	if c.Stage(1).Q() != 2.0 {
		t.Fatalf("SetStageQ not applied")
	}
	c.SetStageQ(-1, 2.0)
	c.SetStageQ(5, 2.0)

	c.Reset()
	for i := 0; i < c.StageCount(); i++ {
		st := c.Stage(i)
		if st.x1 != 0 || st.x2 != 0 || st.y1 != 0 || st.y2 != 0 {
			t.Fatalf("Reset did not zero delay lines")
		}
	}
}

func TestCascadeProcessingAndSanitization(t *testing.T) {
	c := NewCascadeFilter(3, Lowpass, 1000, 48000)
	src := make([]float32, 256)
	dst := make([]float32, 256)
	for i := range src {
		src[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / 48000))
	}

	c.Process(dst, src)
	maxVal := float32(0)
	for _, v := range dst {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("NaN or Inf in Process output")
		}
		if absVal := float32(math.Abs(float64(v))); absVal > maxVal {
			maxVal = absVal
		}
	}
	if maxVal < 0.1 {
		t.Fatalf("signal unexpectedly attenuated: %v", maxVal)
	}

	inPlace := make([]float32, len(src))
	copy(inPlace, src)
	c.Reset()
	c.ProcessInPlace(inPlace)
	for i := range inPlace {
		if math.Abs(float64(inPlace[i]-dst[i])) > 1e-4 {
			t.Fatalf("Process vs ProcessInPlace mismatch at %d: %v vs %v", i, inPlace[i], dst[i])
		}
	}

	nanVal := float32(math.NaN())
	c.ProcessSample(nanVal)
	infVal := float32(math.Inf(1))
	c.ProcessSample(infVal)

	c.Reset()
	small := float32(1e-28)
	flushed := c.ProcessSample(small)
	if flushed != 0 {
		t.Fatalf("anti-denormal flush failed: %v", flushed)
	}
}

func TestCascadeZeroAllocations(t *testing.T) {
	c := NewCascadeFilter(3, Lowpass, 1000, 48000)
	buf := make([]float32, 512)
	for i := range buf {
		buf[i] = 0.5
	}
	allocs := testing.AllocsPerRun(1000, func() {
		c.ProcessInPlace(buf)
	})
	if allocs != 0 {
		t.Fatalf("ProcessInPlace allocated %v times, want 0", allocs)
	}

	dst := make([]float32, 512)
	allocs2 := testing.AllocsPerRun(1000, func() {
		c.Process(dst, buf)
	})
	if allocs2 != 0 {
		t.Fatalf("Process allocated %v times, want 0", allocs2)
	}
}

func BenchmarkCascadeFilter3Stage1024(b *testing.B) {
	c := NewCascadeFilter(3, Lowpass, 1000, 48000)
	buf := make([]float32, 1024)
	for i := range buf {
		buf[i] = float32(i%64) / 64
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.ProcessInPlace(buf)
	}
}
