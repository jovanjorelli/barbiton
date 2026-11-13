package dsp

import (
	"math"
	"sort"
)

type FrequencyPeak struct {
	Frequency float64
	Magnitude float64
	Decibels  float64
	Bin       int
}

type BandEnergy struct {
	Name       string
	LowHz      float64
	HighHz     float64
	Energy     float64
	Percentage float64
}

type SpectralAnalysis struct {
	SampleRate       int
	TotalFrames      int
	DurationSec      float64
	PeakFrequencies  []FrequencyPeak
	EnergyBands      []BandEnergy
	TotalEnergy      float64
	TailEnergy       float64
	TailRatio        float64
	SpectralCentroid float64
}

var standardBandDefs = []struct {
	name string
	low  float64
	high float64
}{
	{"Sub-Bass", 20, 60},
	{"Bass", 60, 250},
	{"Low-Mid", 250, 500},
	{"Mid", 500, 2000},
	{"High-Mid", 2000, 4000},
	{"Presence", 4000, 6000},
	{"Brilliance", 6000, 20000},
}

func defaultEnergyBands() []BandEnergy {
	bands := make([]BandEnergy, len(standardBandDefs))
	for b := range standardBandDefs {
		bands[b] = BandEnergy{
			Name:   standardBandDefs[b].name,
			LowHz:  standardBandDefs[b].low,
			HighHz: standardBandDefs[b].high,
		}
	}
	return bands
}

func AnalyzeSpectrum(samples []float32, sampleRate int, channels int) SpectralAnalysis {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channels <= 0 {
		channels = 1
	}
	frames := len(samples) / channels
	if frames == 0 {
		return SpectralAnalysis{
			SampleRate:  sampleRate,
			EnergyBands: defaultEnergyBands(),
		}
	}

	mono := make([]float64, frames)
	if channels == 1 {
		for i := 0; i < frames; i++ {
			mono[i] = float64(samples[i])
		}
	} else {
		for i := 0; i < frames; i++ {
			sum := 0.0
			for c := 0; c < channels; c++ {
				sum += float64(samples[i*channels+c])
			}
			mono[i] = sum / float64(channels)
		}
	}

	fftSize := 2048
	if frames < fftSize {
		fftSize = NextPowerOfTwo(frames)
		if fftSize < 64 {
			fftSize = 64
		}
	}

	hopSize := fftSize / 2
	window := HannWindow(fftSize)
	numBins := fftSize/2 + 1
	avgMagnitudes := make([]float64, numBins)
	buf := make([]complex128, fftSize)

	windowCount := 0
	for start := 0; start+fftSize <= frames; start += hopSize {
		for i := 0; i < fftSize; i++ {
			buf[i] = complex(mono[start+i]*window[i], 0.0)
		}
		FFT(buf)
		for k := 0; k < numBins; k++ {
			re := real(buf[k])
			im := imag(buf[k])
			mag := math.Sqrt(re*re+im*im) / float64(fftSize)
			if k > 0 && k < numBins-1 {
				mag *= 2.0
			}
			avgMagnitudes[k] += mag
		}
		windowCount++
	}

	if windowCount == 0 {
		for i := 0; i < frames; i++ {
			buf[i] = complex(mono[i]*window[i], 0.0)
		}
		for i := frames; i < fftSize; i++ {
			buf[i] = 0
		}
		FFT(buf)
		for k := 0; k < numBins; k++ {
			re := real(buf[k])
			im := imag(buf[k])
			mag := math.Sqrt(re*re+im*im) / float64(fftSize)
			if k > 0 && k < numBins-1 {
				mag *= 2.0
			}
			avgMagnitudes[k] = mag
		}
		windowCount = 1
	} else {
		for k := 0; k < numBins; k++ {
			avgMagnitudes[k] /= float64(windowCount)
		}
	}

	maxMag := 1e-12
	for _, m := range avgMagnitudes {
		if m > maxMag {
			maxMag = m
		}
	}

	var candidates []FrequencyPeak
	for k := 1; k < numBins-1; k++ {
		m := avgMagnitudes[k]
		if m > avgMagnitudes[k-1] && m > avgMagnitudes[k+1] {
			freq := float64(k) * float64(sampleRate) / float64(fftSize)
			db := 20.0 * math.Log10(m/maxMag)
			candidates = append(candidates, FrequencyPeak{
				Frequency: freq,
				Magnitude: m,
				Decibels:  db,
				Bin:       k,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Magnitude > candidates[j].Magnitude
	})

	maxPeaks := 8
	if len(candidates) < maxPeaks {
		maxPeaks = len(candidates)
	}
	peaks := candidates[:maxPeaks]

	bands := defaultEnergyBands()
	totalEnergy := 0.0
	weightedFreqSum := 0.0
	magSum := 0.0

	for k := 0; k < numBins; k++ {
		freq := float64(k) * float64(sampleRate) / float64(fftSize)
		power := avgMagnitudes[k] * avgMagnitudes[k]
		totalEnergy += power
		weightedFreqSum += freq * avgMagnitudes[k]
		magSum += avgMagnitudes[k]

		for b := range standardBandDefs {
			if freq >= standardBandDefs[b].low && freq < standardBandDefs[b].high {
				bands[b].Energy += power
				break
			}
		}
	}

	for b := range bands {
		if totalEnergy > 0 {
			bands[b].Percentage = (bands[b].Energy / totalEnergy) * 100.0
		}
	}

	spectralCentroid := 0.0
	if magSum > 0 && !math.IsNaN(magSum) && !math.IsInf(magSum, 0) {
		spectralCentroid = weightedFreqSum / magSum
		if math.IsNaN(spectralCentroid) || math.IsInf(spectralCentroid, 0) {
			spectralCentroid = 0
		}
	}

	totalSumSq := 0.0
	for i := 0; i < frames; i++ {
		totalSumSq += mono[i] * mono[i]
	}
	totalRMS := 0.0
	if frames > 0 {
		totalRMS = totalSumSq / float64(frames)
	}

	tailStart := int(float64(frames) * 0.70)
	tailLen := frames - tailStart
	tailSumSq := 0.0
	for i := tailStart; i < frames; i++ {
		tailSumSq += mono[i] * mono[i]
	}
	tailEnergy := 0.0
	if tailLen > 0 {
		tailEnergy = tailSumSq / float64(tailLen)
	}

	tailRatio := 0.0
	if totalRMS > 0 && !math.IsNaN(totalRMS) && !math.IsInf(totalRMS, 0) {
		tailRatio = tailEnergy / totalRMS
		if math.IsNaN(tailRatio) || math.IsInf(tailRatio, 0) {
			tailRatio = 0
		}
	}

	return SpectralAnalysis{
		SampleRate:       sampleRate,
		TotalFrames:      frames,
		DurationSec:      float64(frames) / float64(sampleRate),
		PeakFrequencies:  peaks,
		EnergyBands:      bands,
		TotalEnergy:      totalEnergy,
		TailEnergy:       tailEnergy,
		TailRatio:        tailRatio,
		SpectralCentroid: spectralCentroid,
	}
}
