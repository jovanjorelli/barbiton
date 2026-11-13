package encoder

import (
	"os"
	"path/filepath"
	"testing"

	"barbiton/pkg/wav"
)

func TestBlock4EncoderResolveChannelsMatrix(t *testing.T) {
	validChannels := []struct {
		in   int
		want int
	}{
		{0, 2},
		{1, 1},
		{2, 2},
		{6, 6},
		{8, 8},
	}
	for _, tc := range validChannels {
		s, err := Resolve("24", tc.in, 48000)
		if err != nil {
			t.Fatalf("Resolve channels %d failed: %v", tc.in, err)
		}
		if s.Channels != tc.want {
			t.Fatalf("Resolve channels %d got %d, want %d", tc.in, s.Channels, tc.want)
		}
	}

	invalidChannels := []int{-10, -1, 3, 4, 5, 7, 9, 10, 16}
	for _, ch := range invalidChannels {
		if _, err := Resolve("24", ch, 48000); err == nil {
			t.Fatalf("Resolve invalid channels %d expected error, got nil", ch)
		}
	}
}

func TestBlock4EncoderMultichannelWAVRoundTrips(t *testing.T) {
	cases := []struct {
		depth    string
		channels int
		rate     int
		format   wav.Format
		bits     int
	}{
		{"16", 1, 44100, wav.PCM16, 16},
		{"16", 6, 48000, wav.PCM16, 16},
		{"16", 8, 48000, wav.PCM16, 16},
		{"24", 2, 48000, wav.PCM24, 24},
		{"24", 6, 48000, wav.PCM24, 24},
		{"24", 8, 48000, wav.PCM24, 24},
		{"32", 1, 48000, wav.PCM32, 32},
		{"32", 6, 48000, wav.PCM32, 32},
		{"32", 8, 48000, wav.PCM32, 32},
		{"32f", 2, 96000, wav.Float32, 32},
		{"32f", 6, 48000, wav.Float32, 32},
		{"32f", 8, 96000, wav.Float32, 32},
	}
	for _, tc := range cases {
		s, err := Resolve(tc.depth, tc.channels, tc.rate)
		if err != nil {
			t.Fatalf("Resolve(%s, %d, %d): %v", tc.depth, tc.channels, tc.rate, err)
		}
		path := filepath.Join(t.TempDir(), "enc_roundtrip.wav")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		w, err := s.NewWriter(f)
		if err != nil {
			f.Close()
			t.Fatal(err)
		}

		chans := make([][]float32, tc.channels)
		for ch := 0; ch < tc.channels; ch++ {
			chans[ch] = make([]float32, 64)
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

		info, back, err := wav.DecodeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Channels != tc.channels {
			t.Fatalf("decoded channels %d, want %d", info.Channels, tc.channels)
		}
		if info.SampleRate != tc.rate {
			t.Fatalf("decoded rate %d, want %d", info.SampleRate, tc.rate)
		}
		if info.BitsPerSample != tc.bits {
			t.Fatalf("decoded bits %d, want %d", info.BitsPerSample, tc.bits)
		}
		if len(back) != 64*tc.channels {
			t.Fatalf("decoded sample count %d, want %d", len(back), 64*tc.channels)
		}
	}
}

func BenchmarkBlock4DownmixMono(b *testing.B) {
	dst := make([]float32, 512)
	left := make([]float32, 512)
	right := make([]float32, 512)
	for i := range left {
		left[i] = 0.5
		right[i] = -0.5
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		DownmixMono(dst, left, right, 512)
	}
}
