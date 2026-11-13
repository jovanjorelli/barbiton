package dsp

import "testing"

func BenchmarkSine1024(b *testing.B) {
	osc := NewSine()
	osc.SetFrequency(440, 48000)
	buf := make([]float32, 1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		osc.Fill(buf)
	}
}

func BenchmarkBiquad1024(b *testing.B) {
	f := NewBiquadGain(Peaking, 1000, 1, 3, 48000)
	buf := make([]float32, 1024)
	for i := range buf {
		buf[i] = float32(i%64) / 64
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f.ProcessInPlace(buf)
	}
}

func BenchmarkLimiterProcess(b *testing.B) {
	lim := NewLimiter(48000)
	buf := make([]float32, 1024*2)
	for i := range buf {
		buf[i] = float32(i%19) / 19
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lim.ProcessInterleaved(buf)
	}
}

func BenchmarkReverbProcess(b *testing.B) {
	rev := NewReverb(48000, ReverbParams{RoomSize: 0.7, Damping: 0.5, Wet: 0.3})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rev.ProcessFrameWet(0.5, 0.5)
	}
}

func BenchmarkEnvelopeNextDecay(b *testing.B) {
	e := NewEnvelope(0.001, 1000.0, 0.0, 1000.0)
	e.Trigger()
	dt := 1.0 / 48000.0
	e.Next(0.002)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Next(dt)
	}
}

func BenchmarkEnvelopeNextRelease(b *testing.B) {
	e := NewEnvelope(0.001, 0.001, 0.5, 1000.0)
	e.Trigger()
	e.Next(0.01)
	e.Release()
	dt := 1.0 / 48000.0
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Next(dt)
	}
}

func BenchmarkEnvelopeFill(b *testing.B) {
	e := NewEnvelope(0.01, 0.1, 0.5, 0.1)
	e.Trigger()
	dt := 1.0 / 48000.0
	buf := make([]float32, 1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Fill(buf, dt)
	}
}

func BenchmarkMix(b *testing.B) {
	dst := NewStereoBuffer(1024)
	dst.SetLen(1024)
	src := NewStereoBuffer(1024)
	src.SetLen(1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dst.Mix(src)
	}
}

func BenchmarkMixGain(b *testing.B) {
	dst := NewStereoBuffer(1024)
	dst.SetLen(1024)
	src := NewStereoBuffer(1024)
	src.SetLen(1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dst.MixGain(src, 0.5)
	}
}

func BenchmarkFilter(b *testing.B) {
	f := NewBiquadGain(Peaking, 1000, 1, 3, 48000)
	buf := make([]float32, 1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f.ProcessInPlace(buf)
	}
}
