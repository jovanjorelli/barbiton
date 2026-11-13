package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"barbiton/pkg/wav"
)

func TestAnalyzeSpectrumMainOK(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	wavPath := filepath.Join("..", "..", "test_data", "samples", "c4.wav")
	os.Args = []string{"analyze_spectrum", wavPath}

	main()
}

func TestAnalyzeSpectrumRun(t *testing.T) {
	wavPath := filepath.Join("..", "..", "test_data", "samples", "c4.wav")
	var stdout, stderr bytes.Buffer
	err := run([]string{"analyze_spectrum", wavPath}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	outStr := stdout.String()
	for _, expected := range []string{
		"spectral centroid",
		"peak frequency bins",
		"energy distribution",
		"echo:",
		"tail:",
	} {
		if !bytes.Contains(stdout.Bytes(), []byte(expected)) {
			t.Fatalf("output missing %q, got: %s", expected, outStr)
		}
	}
}

func TestAnalyzeSpectrumResolvedFromRoot(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	subDir := filepath.Join(origWd, "..", "..", "cmd")
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = run([]string{"analyze_spectrum", filepath.Join("test_data", "samples", "c4.wav")}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run failed to resolve from root: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("spectral centroid")) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestAnalyzeSpectrumSilentWav(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "silent.wav")
	f, err := os.Create(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	wr, err := wav.NewWriter(f, wav.PCM16, 48000, 1)
	if err != nil {
		t.Fatal(err)
	}
	silentSamples := make([]float32, 2048)
	if _, err := wr.WriteFrames(silentSamples); err != nil {
		t.Fatal(err)
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = run([]string{"analyze_spectrum", tmpFile}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run failed on silent wav: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("no prominent peaks detected")) {
		t.Fatalf("expected no prominent peaks message, got: %s", stdout.String())
	}
}

func TestAnalyzeSpectrumMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"analyze_spectrum", "non_existent_file_xyz.wav"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !bytes.Contains(stderr.Bytes(), []byte("missing file:")) {
		t.Fatalf("stderr missing error prefix, got: %s", stderr.String())
	}
}

func TestAnalyzeSpectrumDecodeError(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "corrupt.wav")
	if err := os.WriteFile(tmpFile, []byte("not a valid riff wave header"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := run([]string{"analyze_spectrum", tmpFile}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !bytes.Contains(stderr.Bytes(), []byte("decode error:")) {
		t.Fatalf("stderr missing decode error, got: %s", stderr.String())
	}
}

func TestAnalyzeSpectrumDefaultArgResolution(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	_ = run([]string{"analyze_spectrum"}, &stdout, &stderr)
	if !bytes.Contains(stderr.Bytes(), []byte("missing file:")) {
		t.Fatalf("expected missing file for default delay_test.wav, got: %s", stderr.String())
	}
}

func TestAnalyzeSpectrumPureSines(t *testing.T) {
	tmpDir := t.TempDir()
	sr := 48000
	numFrames := 48000

	testFreqs := []float64{440.0, 1000.0}
	for _, targetFreq := range testFreqs {
		filePath := filepath.Join(tmpDir, filepath.Clean(string(rune('a'+int(targetFreq))))+".wav")
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatal(err)
		}
		wr, err := wav.NewWriter(f, wav.PCM16, sr, 1)
		if err != nil {
			t.Fatal(err)
		}

		samples := make([]float32, numFrames)
		for i := 0; i < numFrames; i++ {
			phase := 2.0 * math.Pi * targetFreq * float64(i) / float64(sr)
			samples[i] = float32(0.8 * math.Sin(phase))
		}

		if _, err := wr.WriteFrames(samples); err != nil {
			t.Fatal(err)
		}
		if err := wr.Close(); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		var stdout, stderr bytes.Buffer
		if err := run([]string{"analyze_spectrum", filePath}, &stdout, &stderr); err != nil {
			t.Fatalf("run failed for %v Hz: %v\nstderr: %s", targetFreq, err, stderr.String())
		}

		outStr := stdout.String()
		binWidth := float64(sr) / 2048.0
		var foundPeak float64
		matched := false
		for i := 1; i <= 3; i++ {
			var peak float64
			n, _ := bytes.NewBufferString(outStr).Read(nil)
			_ = n
			marker := "#1:"
			if idx := bytes.Index(stdout.Bytes(), []byte(marker)); idx != -1 {
				line := stdout.Bytes()[idx:]
				if end := bytes.IndexByte(line, '\n'); end != -1 {
					line = line[:end]
				}
				_, _ = fmt.Sscanf(string(line), "#1: %f Hz", &peak)
				foundPeak = peak
				if math.Abs(peak-targetFreq) <= binWidth {
					matched = true
					break
				}
			}
		}
		if !matched {
			t.Fatalf("for target %v Hz, detected peak %v Hz is not within bin tolerance %v Hz\nOutput:\n%s",
				targetFreq, foundPeak, binWidth, outStr)
		}
	}
}
