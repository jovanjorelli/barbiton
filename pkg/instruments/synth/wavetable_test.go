package synth

import (
	"math"
	"testing"

	"barbiton/pkg/dsp"
)

func TestWavetableReferenceTables(t *testing.T) {
	names := WavetableNames()
	if len(names) != 4 {
		t.Fatalf("expected 4 tables, got %d", len(names))
	}
	expected := []string{"saw_to_square", "metallic_growl", "vocal_formant_talk", "aggressive_modern"}
	for i, name := range expected {
		if names[i] != name {
			t.Fatalf("table %d: got %s, want %s", i, names[i], name)
		}
		wt := GetWavetable(name)
		if wt == nil {
			t.Fatalf("GetWavetable(%s) returned nil", name)
		}
		for k := 0; k < NumWavetableFrames; k++ {
			for s := 0; s < WavetableFrameSize; s++ {
				val := wt[k][s]
				if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
					t.Fatalf("table %s frame %d sample %d is NaN/Inf", name, k, s)
				}
				if val > 1.05 || val < -1.05 {
					t.Fatalf("table %s frame %d sample %d out of bounds: %g", name, k, s, val)
				}
			}
		}
	}
}

func TestWavetableHermiteInterpolation(t *testing.T) {
	wt := GetWavetable("saw_to_square")
	prev := SampleWavetableHermite(wt, 0.5, 0.0)
	for i := 1; i <= 2048; i++ {
		phi := float64(i) / 2048.0
		val := SampleWavetableHermite(wt, 0.5, phi)
		diff := math.Abs(val - prev)
		if diff > 0.3 {
			t.Fatalf("unexpected step in bandlimited table at phi=%g: diff=%g", phi, diff)
		}
		prev = val
	}

	prevPos := SampleWavetableHermite(wt, 0.0, 0.25)
	for i := 1; i <= 100; i++ {
		pos := float64(i) / 100.0
		val := SampleWavetableHermite(wt, pos, 0.25)
		diff := math.Abs(val - prevPos)
		if diff > 0.1 {
			t.Fatalf("discontinuity in wavetable pos morph at pos=%g: diff=%g", pos, diff)
		}
		prevPos = val
	}
}

func TestWavetableAllocs(t *testing.T) {
	wt := GetWavetable("metallic_growl")
	allocs := testing.AllocsPerRun(1000, func() {
		_ = SampleWavetableHermite(wt, 0.42, 0.33)
	})
	if allocs != 0 {
		t.Fatalf("SampleWavetableHermite allocated %g heap objects, want 0", allocs)
	}
}

func TestSynthWavetableModeRender(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"mode":                3,
		"wavetable":           1,
		"wavetable_pos":       0.3,
		"wavetable_env_depth": 0.5,
		"wavetable_lfo_depth": 0.2,
		"pitch_bend":          2.0,
		"distortion_drive":    3.0,
		"distortion_mode":     1,
		"distortion_mix":      0.5,
	})

	if s.WavetableIndex() != 1 {
		t.Fatalf("WavetableIndex: got %d, want 1", s.WavetableIndex())
	}
	if s.WavetablePos() != 0.3 {
		t.Fatalf("WavetablePos: got %g, want 0.3", s.WavetablePos())
	}
	if s.PitchBend() != 2.0 {
		t.Fatalf("PitchBend: got %g, want 2.0", s.PitchBend())
	}

	buf := dsp.NewStereoBuffer(256)
	buf.SetLen(256)
	s.NoteOn(60, 0.8)
	s.Process(buf, 1.0/48000.0)

	var hasAudio bool
	for _, v := range buf.Left {
		if math.Abs(float64(v)) > 1e-4 {
			hasAudio = true
			break
		}
	}
	if !hasAudio {
		t.Fatal("wavetable mode produced only silence")
	}

	allocs := testing.AllocsPerRun(100, func() {
		s.Process(buf, 1.0/48000.0)
	})
	if allocs != 0 {
		t.Fatalf("Synth.Process in wavetable mode allocated %g objects, want 0", allocs)
	}
}

func BenchmarkSynthWavetableProcess(b *testing.B) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"mode":          3,
		"wavetable_pos": 0.5,
	})
	buf := dsp.NewStereoBuffer(1024)
	buf.SetLen(1024)
	s.NoteOn(60, 0.8)
	s.Process(buf, 1.0/48000.0)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.Process(buf, 1.0/48000.0)
	}
}
