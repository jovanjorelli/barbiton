package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"barbiton/pkg/dsp"
	"barbiton/pkg/wav"
)

func run(args []string, stdout, stderr io.Writer) error {
	path := "delay_test.wav"
	if len(args) > 1 {
		path = args[1]
	}

	fi, err := os.Stat(path)
	if err != nil {
		if root, rerr := findRepoRoot(); rerr == nil {
			cand := filepath.Join(root, path)
			if cfi, cerr := os.Stat(cand); cerr == nil {
				path = cand
				fi = cfi
				err = nil
			}
		}
	}

	if err != nil {
		fmt.Fprintf(stderr, "missing file: %s\n", path)
		return err
	}

	info, samples, err := wav.DecodeFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "decode error: %v\n", err)
		return err
	}

	analysis := dsp.AnalyzeSpectrum(samples, info.SampleRate, info.Channels)

	fmt.Fprintf(stdout, "file: %s (%d bytes)\n", path, fi.Size())
	fmt.Fprintf(stdout, "format: %d Hz, %d channels, %d frames (%.3f s)\n",
		analysis.SampleRate, info.Channels, analysis.TotalFrames, analysis.DurationSec)
	fmt.Fprintf(stdout, "spectral centroid: %.1f Hz\n", analysis.SpectralCentroid)

	fmt.Fprintln(stdout, "peak frequency bins:")
	if len(analysis.PeakFrequencies) == 0 {
		fmt.Fprintln(stdout, "  (no prominent peaks detected)")
	} else {
		for i, p := range analysis.PeakFrequencies {
			fmt.Fprintf(stdout, "  #%d: %8.1f Hz (mag: %8.5f, %6.1f dB)\n", i+1, p.Frequency, p.Magnitude, p.Decibels)
		}
	}

	fmt.Fprintln(stdout, "energy distribution:")
	for _, b := range analysis.EnergyBands {
		fmt.Fprintf(stdout, "  %-12s (%5.0f - %5.0f Hz): %6.2f%% (energy: %8.5f)\n",
			b.Name, b.LowHz, b.HighHz, b.Percentage, b.Energy)
	}

	echoPresent := analysis.TailRatio > 0.05
	fmt.Fprintf(stdout, "echo: wet return present=%v (tail energy ratio: %.4f)\n", echoPresent, analysis.TailRatio)
	fmt.Fprintf(stdout, "tail: render includes the shared reverb and echo decay tail (energy: %.6f)\n", analysis.TailEnergy)
	return nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found")
}

func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
