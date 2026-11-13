package encoder

import (
	"fmt"
	"io"
	"strings"

	"barbiton/pkg/wav"
)

type Spec struct {
	Format     wav.Format
	Bits       int
	Channels   int
	SampleRate int
}

func Resolve(bitDepth string, channels, sampleRate int) (Spec, error) {
	var s Spec
	switch strings.ToLower(strings.TrimSpace(bitDepth)) {
	case "16":
		s.Format, s.Bits = wav.PCM16, 16
	case "24", "":
		s.Format, s.Bits = wav.PCM24, 24
	case "32":
		s.Format, s.Bits = wav.PCM32, 32
	case "32f", "float", "float32":
		s.Format, s.Bits = wav.Float32, 32
	default:
		return Spec{}, fmt.Errorf("encoder: unsupported bit depth %q (want 16, 24, 32, or 32f)", bitDepth)
	}
	switch {
	case channels == 0:
		s.Channels = 2
	case channels == 1 || channels == 2 || channels == 6 || channels == 8:
		s.Channels = channels
	default:
		return Spec{}, fmt.Errorf("encoder: unsupported channel count %d (want 1, 2, 6, or 8)", channels)
	}
	switch sampleRate {
	case 44100, 48000, 96000:
		s.SampleRate = sampleRate
	default:
		return Spec{}, fmt.Errorf("encoder: unsupported sample rate %d (want 44100, 48000, or 96000)", sampleRate)
	}
	return s, nil
}

func (s Spec) NewWriter(w io.WriteSeeker) (*wav.Writer, error) {
	return wav.NewWriterBits(w, s.Format, s.Bits, s.SampleRate, s.Channels)
}

func DownmixMono(dst, left, right []float32, n int) int {
	if n > len(dst) {
		n = len(dst)
	}
	if n > len(left) {
		n = len(left)
	}
	if n > len(right) {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		dst[i] = (left[i] + right[i]) * 0.5
	}
	return n
}
