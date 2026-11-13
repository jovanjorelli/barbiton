package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestFFTRoundTrip(t *testing.T) {
	n := 256
	orig := make([]complex128, n)
	for i := 0; i < n; i++ {
		orig[i] = complex(math.Sin(2*math.Pi*float64(i)/32.0), math.Cos(2*math.Pi*float64(i)/16.0))
	}

	work := make([]complex128, n)
	copy(work, orig)

	FFT(work)
	IFFT(work)

	for i := 0; i < n; i++ {
		diff := cmplx.Abs(work[i] - orig[i])
		if diff > 1e-12 {
			t.Fatalf("index %d: got %v, want %v (diff %v)", i, work[i], orig[i], diff)
		}
	}
}

func TestAnalyzeSpectrumSinePeak(t *testing.T) {
	sampleRate := 48000
	freq := 440.0
	seconds := 0.5
	numSamples := int(float64(sampleRate) * seconds)

	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		tSec := float64(i) / float64(sampleRate)
		samples[i] = float32(0.8 * math.Sin(2*math.Pi*freq*tSec))
	}

	analysis := AnalyzeSpectrum(samples, sampleRate, 1)

	if len(analysis.PeakFrequencies) == 0 {
		t.Fatal("expected at least one peak frequency")
	}

	topPeak := analysis.PeakFrequencies[0]
	if math.Abs(topPeak.Frequency-freq) > 25.0 {
		t.Fatalf("top peak frequency expected ~440 Hz, got %v", topPeak.Frequency)
	}

	foundBand := false
	for _, band := range analysis.EnergyBands {
		if band.Name == "Low-Mid" {
			foundBand = true
			if band.Percentage < 80.0 {
				t.Fatalf("expected Low-Mid band to have majority energy, got %v%%", band.Percentage)
			}
		}
	}
	if !foundBand {
		t.Fatal("Low-Mid band not found")
	}

	if analysis.SpectralCentroid < 350 || analysis.SpectralCentroid > 550 {
		t.Fatalf("expected spectral centroid around 440 Hz, got %v", analysis.SpectralCentroid)
	}
}

func TestAnalyzeSpectrumTailDetection(t *testing.T) {
	sampleRate := 48000
	numSamples := 48000
	samples := make([]float32, numSamples)

	for i := 0; i < 4800; i++ {
		samples[i] = 0.9
	}
	for i := 36000; i < 40000; i++ {
		samples[i] = 0.5
	}

	analysis := AnalyzeSpectrum(samples, sampleRate, 1)

	if analysis.TailEnergy <= 0 {
		t.Fatal("expected non-zero tail energy")
	}
	if analysis.TailRatio <= 0 {
		t.Fatal("expected non-zero tail ratio")
	}
}

func TestAnalyzeSpectrumEdgeCases(t *testing.T) {
	empty := AnalyzeSpectrum(nil, 48000, 1)
	if empty.TotalFrames != 0 || len(empty.PeakFrequencies) != 0 {
		t.Fatalf("unexpected empty analysis: %+v", empty)
	}
	if len(empty.EnergyBands) != 7 {
		t.Fatalf("expected 7 default energy bands for empty input, got %d", len(empty.EnergyBands))
	}

	single := AnalyzeSpectrum([]float32{0.5}, 0, 0)
	if single.TotalFrames != 1 || single.SampleRate != 48000 {
		t.Fatalf("unexpected single-sample analysis: %+v", single)
	}

	silence := make([]float32, 2048)
	silenceAnalysis := AnalyzeSpectrum(silence, 48000, 1)
	if silenceAnalysis.SpectralCentroid != 0.0 || silenceAnalysis.TotalEnergy != 0.0 || silenceAnalysis.TailRatio != 0.0 {
		t.Fatalf("unexpected silence analysis values: centroid=%v, total=%v, tailRatio=%v",
			silenceAnalysis.SpectralCentroid, silenceAnalysis.TotalEnergy, silenceAnalysis.TailRatio)
	}
}

func TestFFTNonPowerOfTwoSafety(t *testing.T) {
	for _, size := range []int{0, 1, 3, 5, 7, 10, 100} {
		buf := make([]complex128, size)
		FFT(buf)
		IFFT(buf)
	}
}
