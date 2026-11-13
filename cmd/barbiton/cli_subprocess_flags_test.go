package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	r2BinOnce sync.Once
	r2BinPath string
	r2BinRoot string
	r2BinErr  error
)

func r2Binary(t *testing.T) string {
	t.Helper()
	r2BinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "barbiton-r2bin")
		if err != nil {
			r2BinErr = err
			return
		}
		root, err := os.Getwd()
		if err != nil {
			r2BinErr = err
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				r2BinErr = err
				return
			}
			root = parent
		}
		r2BinPath = filepath.Join(dir, "barbiton")
		r2BinRoot = root
		cmd := exec.Command("go", "build", "-o", r2BinPath, "./cmd/barbiton")
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			r2BinErr = err
			_ = b
			return
		}
	})
	if r2BinErr != nil {
		t.Fatalf("build shared binary: %v", r2BinErr)
	}
	return r2BinPath
}

func r2Render(t *testing.T, extra ...string) string {
	t.Helper()
	bin := r2Binary(t)
	out := filepath.Join(t.TempDir(), "out.wav")
	args := append([]string{"--score=test_data/examples/simple_score.json", "--output=" + out}, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Dir = r2BinRoot
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render %v: %v\n%s", extra, err, b)
	}
	raw, err := os.ReadFile(out)
	if err != nil || len(raw) <= 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		t.Fatalf("render %v produced invalid wav (%d bytes, err %v)", extra, len(raw), err)
	}
	return out
}

func TestR2SubSaturationClean(t *testing.T) { r2Render(t, "--saturation=clean") }

func TestR2SubReverbDamping(t *testing.T) { r2Render(t, "--reverb-damping=0.0") }

func TestR2SubNoiseGate(t *testing.T) { r2Render(t, "--noise-gate=-60") }

func TestR2SubLimiter(t *testing.T) { r2Render(t, "--limiter") }

func TestR2SubMonoBass(t *testing.T) { r2Render(t, "--mono-bass=100") }

func TestR2SubParallel(t *testing.T) { r2Render(t, "--parallel") }

func TestR2SubBadSaturation(t *testing.T) { r2Reject(t, "--saturation=bogus", "saturation") }

func TestR2SubBadMonoBass(t *testing.T) { r2Reject(t, "--mono-bass=bogus", "mono-bass") }

func TestR2SubBadNoiseGate(t *testing.T) { r2Reject(t, "--noise-gate=5", "noise-gate") }

func r2Reject(t *testing.T, flag, want string) {
	t.Helper()
	bin := r2Binary(t)
	out := filepath.Join(t.TempDir(), "out.wav")
	cmd := exec.Command(bin, "--score=test_data/examples/simple_score.json", "--output="+out, flag)
	cmd.Dir = r2BinRoot
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("%s accepted with exit 0", flag)
	}
	if !strings.Contains(string(b), want) {
		t.Fatalf("%s stderr missing %q:\n%s", flag, want, b)
	}
}
