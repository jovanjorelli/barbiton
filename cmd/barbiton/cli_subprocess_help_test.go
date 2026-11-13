package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBarbitonBinary(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := dir
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("go.mod not found")
		}
		root = parent
	}
	out := filepath.Join(t.TempDir(), "barbiton")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/barbiton")
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	return out
}

func TestCLIHelpFlagTable(t *testing.T) {
	bin := buildBarbitonBinary(t)
	cmd := exec.Command(bin, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--help exit: %v\n%s", err, out)
	}

	for _, want := range []string{"-score", "-output", "-sample-rate", "-batch-dir"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"\n  -version", "\n  -load ", "\n  -out "} {
		if strings.Contains(string(out), unwanted) {
			t.Fatalf("help unexpectedly contains %q:\n%s", unwanted, out)
		}
	}
}

func TestCLIUnknownFlag(t *testing.T) {
	bin := buildBarbitonBinary(t)
	cmd := exec.Command(bin, "--bogus-flag")
	out := cmd.Run()
	if out == nil {
		t.Fatal("unknown flag accepted with exit 0")
	}
	if exit, ok := out.(*exec.ExitError); !ok || exit.ExitCode() == 0 {
		t.Fatalf("unknown flag exit = %v, want non-zero", out)
	}

	cmd2 := exec.Command(bin, "--bogus-flag")
	b, _ := cmd2.CombinedOutput()
	if !strings.Contains(string(b), "not defined") {
		t.Fatalf("stderr not readable:\n%s", b)
	}
}

func TestCLIPrunedFlagsRejected(t *testing.T) {
	bin := buildBarbitonBinary(t)
	for _, f := range []string{"--version", "--load=some.json", "--out=some.wav"} {
		cmd := exec.Command(bin, f)
		b, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("%s accepted with exit 0", f)
		}
		if !strings.Contains(string(b), "flag provided but not defined") {
			t.Fatalf("%s stderr missing 'flag provided but not defined':\n%s", f, b)
		}
	}
}

func TestCLIMissingInputFile(t *testing.T) {
	bin := buildBarbitonBinary(t)
	missing := filepath.Join(t.TempDir(), "nope.json")
	cmd := exec.Command(bin, "--score="+missing, "--output="+filepath.Join(t.TempDir(), "o.wav"))
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("missing input accepted with exit 0")
	}
	if !strings.Contains(string(b), "nope.json") {
		t.Fatalf("stderr missing path:\n%s", b)
	}
}
