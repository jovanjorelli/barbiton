package main

import (
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func renderVia(t *testing.T, bin string, scoreFlag, score, outFlag, out string) {
	t.Helper()
	cmd := exec.Command(bin, scoreFlag+"="+score, outFlag+"="+out, "--reverb=none")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s/%s: %v\n%s", scoreFlag, outFlag, err, b)
	}
}

func sha256File(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		t.Fatalf("output missing/empty: %v", err)
	}
	return sha256.Sum256(data)
}

func TestCLIShorthandScoreAlias(t *testing.T) {
	bin := buildBarbitonBinary(t)
	dir := t.TempDir()
	score := writeMinimalScoreFile(t, dir, "s.json")
	canonical := filepath.Join(dir, "canonical.wav")
	shorthand := filepath.Join(dir, "shorthand.wav")
	renderVia(t, bin, "--score", score, "--output", canonical)
	renderVia(t, bin, "-s", score, "--output", shorthand)
	if sha256File(t, canonical) != sha256File(t, shorthand) {
		t.Fatal("-s output differs from --score output")
	}
}

func TestCLIShorthandOutAlias(t *testing.T) {
	bin := buildBarbitonBinary(t)
	dir := t.TempDir()
	score := writeMinimalScoreFile(t, dir, "s.json")
	canonical := filepath.Join(dir, "canonical.wav")
	shorthand := filepath.Join(dir, "shorthand.wav")
	renderVia(t, bin, "--score", score, "--output", canonical)
	renderVia(t, bin, "--score", score, "-o", shorthand)
	if sha256File(t, canonical) != sha256File(t, shorthand) {
		t.Fatal("-o output differs from --output output")
	}
}
