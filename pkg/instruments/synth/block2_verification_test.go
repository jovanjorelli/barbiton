package synth

import (
	"math"
	"testing"

	"barbiton/pkg/dsp"
)

func TestBlock2PolyBLEPDiscontinuityCorrection(t *testing.T) {
	c5Freq := 523.25
	dt := c5Freq / 48000.0

	sawAtZero := oscWaveBlep(WaveSaw, 0.0, dt)
	if math.Abs(sawAtZero) > 1e-4 {
		t.Fatalf("polyBLEP saw at phase 0 got %v, want ~0", sawAtZero)
	}

	squareAtZero := oscWaveBlep(WaveSquare, 0.0, dt)
	if math.Abs(squareAtZero) > 1e-4 {
		t.Fatalf("polyBLEP square at phase 0 got %v, want ~0", squareAtZero)
	}

	squareAtHalf := oscWaveBlep(WaveSquare, 0.5, dt)
	if math.Abs(squareAtHalf) > 1e-4 {
		t.Fatalf("polyBLEP square at phase 0.5 got %v, want ~0", squareAtHalf)
	}

	sawMid := oscWaveBlep(WaveSaw, 0.25, dt)
	if math.Abs(sawMid-(-0.5)) > 1e-4 {
		t.Fatalf("polyBLEP saw away from step got %v, want -0.5", sawMid)
	}

	squareMid := oscWaveBlep(WaveSquare, 0.25, dt)
	if math.Abs(squareMid-1.0) > 1e-4 {
		t.Fatalf("polyBLEP square away from step got %v, want 1.0", squareMid)
	}

	if v := polyBLEP(-0.1, dt); v != 0 {
		t.Fatalf("polyBLEP(-0.1) = %v, want 0", v)
	}
	if v := polyBLEP(1.1, dt); v != 0 {
		t.Fatalf("polyBLEP(1.1) = %v, want 0", v)
	}
}

func TestBlock2FilterResonanceSaturation(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"cutoff":    1000,
		"resonance": 1.0,
		"attack":    0.001,
		"sustain":   1.0,
	})

	buf := dsp.NewStereoBuffer(1024)
	buf.SetLen(1024)
	s.NoteOn(60, 1.0)
	s.Process(buf, 1.0/48000)

	maxPeak := float32(0)
	for i := range buf.Left {
		if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
			t.Fatalf("filter produced NaN or Inf at frame %d", i)
		}
		if a := float32(math.Abs(float64(buf.Left[i]))); a > maxPeak {
			maxPeak = a
		}
	}

	if maxPeak > 2.5 {
		t.Fatalf("resonance exploded without saturation: peak = %v", maxPeak)
	}
}

func TestBlock2ZeroAllocations(t *testing.T) {
	s := New(48000)
	buf := dsp.NewStereoBuffer(256)
	buf.SetLen(256)
	s.NoteOn(72, 0.9)
	s.Process(buf, 1.0/48000)

	allocs := testing.AllocsPerRun(50, func() {
		s.Process(buf, 1.0/48000)
	})
	if allocs != 0 {
		t.Fatalf("Synth Process allocated %v, want 0", allocs)
	}
}
