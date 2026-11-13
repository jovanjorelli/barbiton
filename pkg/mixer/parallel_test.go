package mixer

import (
	"testing"

	"barbiton/pkg/dsp"
)

func parFixture() *Mixer {
	m := New(48000, 4, 256)
	m.SetReverbWet(0.5)
	a := m.AddTrack(&dcInstrument{level: 0.2}, -0.5, 0.8)
	m.SetSend(a, 0.4)
	b := m.AddTrack(&dcInstrument{level: -0.3}, 0.5, 0.7)
	m.SetTrackDelay(b, dsp.DelayConfig{DelaySeconds: 0.05, Feedback: 0.4, Wet: 0.5}, 120)
	c := m.AddTrack(&dcInstrument{level: 0.1}, 0, 1)
	m.SetMuted(c, true)
	return m
}

func snap(t *testing.T, m *Mixer, blocks int) [][]float32 {
	t.Helper()
	var out [][]float32
	for i := 0; i < blocks; i++ {
		m.Process(256)
		out = append(out, append([]float32{}, m.Master()...))
	}
	return out
}

func TestParallelIdentical(t *testing.T) {
	seq, par := parFixture(), parFixture()
	par.SetParallel(true)
	if !par.Parallel() || seq.Parallel() {
		t.Fatal("parallel flag misreported")
	}
	a, b := snap(t, seq, 8), snap(t, par, 8)
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Fatalf("block %d lens %d/%d", i, len(a[i]), len(b[i]))
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Fatalf("block %d frame %d: %v vs %v", i, j, a[i][j], b[i][j])
			}
		}
	}
}

func TestParallelZeroAlloc(t *testing.T) {
	m := parFixture()
	m.SetParallel(true)
	m.Process(256)
	if allocs := testing.AllocsPerRun(30, func() { m.Process(256) }); allocs != 0 {
		t.Fatalf("parallel Process allocs = %v", allocs)
	}
}

func TestParallelToggle(t *testing.T) {
	a, b := parFixture(), parFixture()
	b.SetParallel(true)
	for i := 0; i < 4; i++ {
		a.Process(256)
		b.Process(256)
	}
	a.SetParallel(true)
	b.SetParallel(false)
	for i := 0; i < 4; i++ {
		a.Process(256)
		b.Process(256)
		am, bm := a.Master(), b.Master()
		for j := range am {
			if am[j] != bm[j] {
				t.Fatalf("toggled block %d frame %d differs", i, j)
			}
		}
	}
}

type panicInstrument struct{}

func (p *panicInstrument) Process(buffer *dsp.Buffer, deltaTime float64) {
	panic("boom")
}
func (p *panicInstrument) NoteOn(pitch int, velocity float32)      {}
func (p *panicInstrument) NoteOff(pitch int)                       {}
func (p *panicInstrument) SetParameters(params map[string]float32) {}

func TestParallelPanicRecovery(t *testing.T) {
	m := New(48000, 2, 256)
	m.SetParallel(true)
	m.AddTrack(&panicInstrument{}, 0, 1)
	m.Process(256)
	if m.Err() == nil {
		t.Fatal("expected error after worker panic, got nil")
	}
}

type flakyInstrument struct {
	shouldPanic bool
}

func (f *flakyInstrument) Process(buffer *dsp.Buffer, deltaTime float64) {
	if f.shouldPanic {
		panic("flaky boom")
	}
	for i := 0; i < buffer.Len(); i++ {
		buffer.Left[i] = 0.5
		buffer.Right[i] = 0.5
	}
}
func (f *flakyInstrument) NoteOn(pitch int, velocity float32)      {}
func (f *flakyInstrument) NoteOff(pitch int)                       {}
func (f *flakyInstrument) SetParameters(params map[string]float32) {}

func TestParallelPanicClearsBuffer(t *testing.T) {
	m := New(48000, 2, 256)
	m.SetParallel(true)
	inst := &flakyInstrument{shouldPanic: false}
	m.AddTrack(inst, 0, 1)
	m.Process(256)
	master1 := m.Master()
	if master1[0] == 0 {
		t.Fatal("expected non-zero audio in first block")
	}
	inst.shouldPanic = true
	m.Process(256)
	if m.Err() == nil {
		t.Fatal("expected error on second block panic")
	}
	master2 := m.Master()
	for i, v := range master2 {
		if v != 0 {
			t.Fatalf("frame %d = %v; expected 0 silence on panic block, not stale buffer", i, v)
		}
	}
}

func BenchmarkMixerParallel8Track(b *testing.B) {
	m := New(48000, 8, 1024)
	m.SetParallel(true)
	m.SetReverbWet(0.25)
	for i := 0; i < 8; i++ {
		m.AddTrack(&dcInstrument{level: 0.1}, 0, 0.8)
		m.SetSend(i, 0.3)
	}
	m.Process(1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Process(1024)
	}
}

func BenchmarkMixer16Track(b *testing.B) {
	m := New(48000, 16, 1024)
	m.SetReverbWet(0.25)
	for i := 0; i < 16; i++ {
		m.AddTrack(&dcInstrument{level: 0.1}, 0, 0.8)
		m.SetSend(i, 0.3)
	}
	m.Process(1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Process(1024)
	}
}

func BenchmarkMixerParallel16Track(b *testing.B) {
	m := New(48000, 16, 1024)
	m.SetParallel(true)
	m.SetReverbWet(0.25)
	for i := 0; i < 16; i++ {
		m.AddTrack(&dcInstrument{level: 0.1}, 0, 0.8)
		m.SetSend(i, 0.3)
	}
	m.Process(1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Process(1024)
	}
}

func TestParallelStress1000(t *testing.T) {
	m := New(48000, 8, 256)
	m.SetParallel(true)
	for i := 0; i < 8; i++ {
		m.AddTrack(&dcInstrument{level: 0.1 * float32(i+1)}, 0, 0.8)
		m.SetSend(i, 0.2)
	}
	for i := 0; i < 1000; i++ {
		m.Process(256)
	}
	if m.Err() != nil {
		t.Fatalf("parallel stress 1000 failed: %v", m.Err())
	}
}
