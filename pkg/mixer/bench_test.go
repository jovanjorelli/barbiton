package mixer

import "testing"

func BenchmarkMixer8Track(b *testing.B) {
	m := New(48000, 8, 1024)
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

func BenchmarkSoftClip(b *testing.B) {
	c := NewSoftClipper(0.9)
	buf := make([]float32, 1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.ProcessSlice(buf)
	}
}

func BenchmarkSoftClipOversampled(b *testing.B) {
	c := NewSoftClipper(0.9)
	c.SetOversampling(true)
	buf := make([]float32, 1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.ProcessSlice(buf)
	}
}
