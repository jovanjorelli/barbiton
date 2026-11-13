package dsp

import (
	"math"
	"testing"
)

func TestBlock3EnvelopeDtVariation(t *testing.T) {
	e := NewEnvelope(0.001, 0.05, 0.4, 0.05)
	e.Trigger()
	for e.Stage() == StageAttack {
		e.Next(1.0 / 48000.0)
	}
	if e.Stage() != StageDecay {
		t.Fatalf("expected decay, got %v", e.Stage())
	}
	dts := []float64{1.0 / 44100.0, 1.0 / 48000.0, 1.0 / 96000.0, 1.0 / 88200.0}
	prev := e.Value()
	for i := 0; i < 100 && e.Stage() == StageDecay; i++ {
		cur := e.Next(dts[i%len(dts)])
		if cur > prev+1e-9 {
			t.Fatalf("decay value increased: %v -> %v", prev, cur)
		}
		prev = cur
	}
	e.Release()
	if e.Stage() != StageRelease {
		t.Fatalf("expected release, got %v", e.Stage())
	}
	for i := 0; i < 100 && e.Stage() == StageRelease; i++ {
		cur := e.Next(dts[i%len(dts)])
		if cur > prev+1e-9 {
			t.Fatalf("release value increased: %v -> %v", prev, cur)
		}
		prev = cur
	}
}

func TestBlock3EnvelopeMidNoteParamChange(t *testing.T) {
	e := NewEnvelope(0.001, 0.1, 0.5, 0.1)
	e.Trigger()
	e.Next(0.002)
	if e.Stage() != StageDecay {
		t.Fatalf("expected decay, got %v", e.Stage())
	}
	e.SetParameters(0.001, 0.02, 0.3, 0.02)
	dt := 1.0 / 48000.0
	for i := 0; i < 5000 && e.Stage() == StageDecay; i++ {
		e.Next(dt)
	}
	if e.Stage() != StageSustain {
		t.Fatalf("expected sustain stage, got %v", e.Stage())
	}
	if math.Abs(e.Value()-0.3) > 1e-4 {
		t.Fatalf("expected sustain level ~0.3, got %v", e.Value())
	}
}

func TestBlock3EnvelopeZeroAllocs(t *testing.T) {
	e := NewEnvelope(0.01, 0.05, 0.5, 0.05)
	e.Trigger()
	dt := 1.0 / 48000.0
	buf := make([]float32, 128)
	allocs := testing.AllocsPerRun(100, func() {
		e.Next(dt)
		e.Fill(buf, dt)
		e.Apply(buf, dt)
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocations in audio envelope loop, got %v", allocs)
	}
}

func TestBlock3EnvelopeAccuracyVsMathExp(t *testing.T) {
	attack := 0.002
	decay := 0.04
	sustain := 0.6
	release := 0.05
	dt := 1.0 / 48000.0

	e := NewEnvelope(attack, decay, sustain, release)
	e.Trigger()
	for e.Stage() == StageAttack {
		e.Next(dt)
	}

	decayRate := expConvergence / decay
	refValue := 1.0
	for i := 0; i < 500 && e.Stage() == StageDecay; i++ {
		got := e.Next(dt)
		refValue = sustain + (refValue-sustain)*math.Exp(-decayRate*dt)
		if math.Abs(got-refValue) > 1e-9 {
			t.Fatalf("decay mismatch at step %d: got %v, want %v", i, got, refValue)
		}
	}

	e.Release()
	releaseRate := expConvergence / release
	refValue = e.Value()
	for i := 0; i < 500 && e.Stage() == StageRelease; i++ {
		got := e.Next(dt)
		refValue *= math.Exp(-releaseRate * dt)
		if math.Abs(got-refValue) > 1e-9 {
			t.Fatalf("release mismatch at step %d: got %v, want %v", i, got, refValue)
		}
	}
}

func TestBlock3EnvelopeZeroDeltaTime(t *testing.T) {
	e := NewEnvelope(0.01, 0.1, 0.5, 0.1)
	e.Trigger()
	e.Next(0.005)
	valAttack := e.Value()
	if e.Next(0) != valAttack {
		t.Fatalf("expected value %v on zero dt attack, got %v", valAttack, e.Value())
	}
	e.Next(0.01)
	valDecay := e.Value()
	if e.Next(0) != valDecay {
		t.Fatalf("expected value %v on zero dt decay, got %v", valDecay, e.Value())
	}
	e.Release()
	valRelease := e.Value()
	if e.Next(0) != valRelease {
		t.Fatalf("expected value %v on zero dt release, got %v", valRelease, e.Value())
	}
}
