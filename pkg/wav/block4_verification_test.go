package wav

import (
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestBlock4TPDFDitherStatistics(t *testing.T) {
	const n = 100000
	var sum float64
	var sumSq float64
	counts := make(map[int16]int)
	for i := 0; i < n; i++ {
		v := floatToInt16(0.0)
		if v < -2 || v > 2 {
			t.Fatalf("dither on zero out of bounds: %d", v)
		}
		counts[v]++
		vf := float64(v)
		sum += vf
		sumSq += vf * vf
	}
	mean := sum / n
	if math.Abs(mean) > 0.02 {
		t.Fatalf("expected near-zero mean, got %f", mean)
	}
	variance := (sumSq / n) - (mean * mean)
	if variance < 0.1 || variance > 0.3 {
		t.Fatalf("expected variance near 0.1667, got %f", variance)
	}
	if counts[0] <= counts[1] || counts[0] <= counts[-1] {
		t.Fatalf("expected peak at 0 for TPDF: %+v", counts)
	}
}

func TestBlock4ScaleAsymmetryAndBoundaries(t *testing.T) {
	nan := float32(math.NaN())
	posInf := float32(math.Inf(1))
	negInf := float32(math.Inf(-1))

	if got := floatToInt16(nan); got != 0 {
		t.Fatalf("floatToInt16(NaN) = %d, want 0", got)
	}
	if got := floatToInt16(posInf); got != int16Max {
		t.Fatalf("floatToInt16(+Inf) = %d, want %d", got, int16Max)
	}
	if got := floatToInt16(negInf); got != int16Min {
		t.Fatalf("floatToInt16(-Inf) = %d, want %d", got, int16Min)
	}
	if got := floatToInt16(1.0); got != int16Max {
		t.Fatalf("floatToInt16(1.0) = %d, want %d", got, int16Max)
	}
	if got := floatToInt16(-1.0); got != int16Min {
		t.Fatalf("floatToInt16(-1.0) = %d, want %d", got, int16Min)
	}
	if got := floatToInt16(10.0); got != int16Max {
		t.Fatalf("floatToInt16(10.0) = %d, want %d", got, int16Max)
	}
	if got := floatToInt16(-10.0); got != int16Min {
		t.Fatalf("floatToInt16(-10.0) = %d, want %d", got, int16Min)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "unity_16.wav")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWriterBits(f, PCM16, 16, 48000, 1)
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	in := []float32{-1.0, 1.0, 0.0}
	if _, err := w.WriteFrames(in); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	info, out, err := DecodeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.BitsPerSample != 16 || info.Channels != 1 {
		t.Fatalf("unexpected info: %+v", info)
	}
	if out[0] != -1.0 {
		t.Fatalf("unity negative peak mismatch: got %f, want -1.0", out[0])
	}
	if math.Abs(float64(out[1]-32767.0/32768.0)) > 1e-6 {
		t.Fatalf("positive peak mismatch: got %f, want %f", out[1], 32767.0/32768.0)
	}
}

func TestBlock4DitherConcurrency(t *testing.T) {
	const goroutines = 16
	const iters = 2000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				v := floatToInt16(0.0)
				if v < -2 || v > 2 {
					t.Errorf("out of range: %d", v)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestBlock4MultichannelExtensibleHeaders(t *testing.T) {
	cases := []struct {
		channels int
		mask     uint32
	}{
		{6, 0x3F},
		{8, 0x63F},
	}
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "multi.wav")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		w, err := NewWriterBits(f, PCM16, 16, 48000, tc.channels)
		if err != nil {
			f.Close()
			t.Fatal(err)
		}
		chans := make([][]float32, tc.channels)
		for ch := 0; ch < tc.channels; ch++ {
			chans[ch] = make([]float32, 100)
			for i := range chans[ch] {
				chans[ch][i] = float32(ch) * 0.1
			}
		}
		if _, err := w.WriteFrames(chans...); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		f.Close()

		info, back, err := DecodeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Channels != tc.channels {
			t.Fatalf("channels = %d, want %d", info.Channels, tc.channels)
		}
		if info.ChannelMask != tc.mask {
			t.Fatalf("channel mask = 0x%X, want 0x%X", info.ChannelMask, tc.mask)
		}
		if len(back) != 100*tc.channels {
			t.Fatalf("sample count = %d, want %d", len(back), 100*tc.channels)
		}
	}
}

func BenchmarkBlock4FloatToInt16(b *testing.B) {
	b.ReportAllocs()
	var acc int16
	for i := 0; i < b.N; i++ {
		acc += floatToInt16(0.123)
	}
	_ = acc
}

func BenchmarkBlock4WriteFramesPCM16(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench16.wav")
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	w, err := NewWriterBits(f, PCM16, 16, 48000, 2)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()

	left := make([]float32, 512)
	right := make([]float32, 512)
	for i := range left {
		left[i] = 0.25
		right[i] = -0.25
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := w.WriteFrames(left, right); err != nil {
			b.Fatal(err)
		}
	}
}
