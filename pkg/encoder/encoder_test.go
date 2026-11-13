package encoder

import (
	"os"
	"path/filepath"
	"testing"

	"barbiton/pkg/wav"
)

func TestResolveMatrix(t *testing.T) {
	cases := []struct {
		depth          string
		channels, rate int
		format         wav.Format
		bits, ch, sr   int
	}{
		{"16", 1, 44100, wav.PCM16, 16, 1, 44100},
		{"24", 2, 48000, wav.PCM24, 24, 2, 48000},
		{"", 0, 96000, wav.PCM24, 24, 2, 96000},
		{"32", 2, 48000, wav.PCM32, 32, 2, 48000},
		{"32f", 2, 48000, wav.Float32, 32, 2, 48000},
		{"float", 1, 44100, wav.Float32, 32, 1, 44100},
		{"24", 6, 48000, wav.PCM24, 24, 6, 48000},
		{"32f", 8, 96000, wav.Float32, 32, 8, 96000},
	}
	for _, tc := range cases {
		s, err := Resolve(tc.depth, tc.channels, tc.rate)
		if err != nil {
			t.Fatalf("Resolve(%q,%d,%d): %v", tc.depth, tc.channels, tc.rate, err)
		}
		if s.Format != tc.format || s.Bits != tc.bits || s.Channels != tc.ch || s.SampleRate != tc.sr {
			t.Fatalf("Resolve(%q,%d,%d) = %+v", tc.depth, tc.channels, tc.rate, s)
		}
	}
	for _, tc := range []struct {
		depth          string
		channels, rate int
	}{
		{"12", 2, 48000},
		{"24", 3, 48000},
		{"16", -1, 44100},
		{"24", 2, 22050},
		{"64", 2, 48000},
	} {
		if _, err := Resolve(tc.depth, tc.channels, tc.rate); err == nil {
			t.Errorf("Resolve(%q,%d,%d): want error, got nil", tc.depth, tc.channels, tc.rate)
		}
	}
}

func TestSpecWriterRoundTrip(t *testing.T) {
	s, err := Resolve("16", 1, 44100)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "enc.wav")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.NewWriter(f)
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	mono := []float32{0.5, -0.5, 0.25, -0.25}
	if _, err := w.WriteFrames(mono); err != nil {
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
	if info.Channels != 1 || info.BitsPerSample != 16 || info.SampleRate != 44100 {
		t.Fatalf("info = %+v", info)
	}
	for i, want := range mono {
		if d := float64(back[i] - want); d < -0.0001 || d > 0.0001 {
			t.Fatalf("frame %d = %v, want %v", i, back[i], want)
		}
	}
}

func TestDownmixMono(t *testing.T) {
	left := []float32{1, 0.5, -0.5}
	right := []float32{-1, 0.5, 0.5}
	dst := make([]float32, 3)
	if n := DownmixMono(dst, left, right, 3); n != 3 {
		t.Fatalf("wrote %d frames, want 3", n)
	}
	for i, want := range []float32{0, 0.5, 0} {
		if dst[i] != want {
			t.Fatalf("dst[%d] = %v, want %v", i, dst[i], want)
		}
	}
	short := make([]float32, 2)
	if n := DownmixMono(short, left, right, 3); n != 2 {
		t.Fatalf("clamped write = %d, want 2", n)
	}
}

func TestMultichannelRoundTrip(t *testing.T) {
	cases := []struct {
		channels int
		mask     uint32
	}{
		{6, 0x3F},
		{8, 0x63F},
	}
	for _, tc := range cases {
		s, err := Resolve("24", tc.channels, 48000)
		if err != nil {
			t.Fatalf("Resolve channels %d: %v", tc.channels, err)
		}
		path := filepath.Join(t.TempDir(), "multi.wav")
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
			chans[ch] = []float32{float32(ch) * 0.1, -float32(ch) * 0.1}
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
			t.Fatalf("got channels %d, want %d", info.Channels, tc.channels)
		}
		if info.ChannelMask != tc.mask {
			t.Fatalf("got channel mask 0x%X, want 0x%X", info.ChannelMask, tc.mask)
		}
		if len(back) != 2*tc.channels {
			t.Fatalf("got %d samples, want %d", len(back), 2*tc.channels)
		}
	}
}
