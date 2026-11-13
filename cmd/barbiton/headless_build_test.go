package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHeadlessBuildGate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow build gate in short mode")
	}

	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to locate project root: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "ui")); !os.IsNotExist(err) {
		t.Fatal("forbidden ui/ directory exists")
	}

	err = filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) == ".go" && !strings.HasSuffix(p, "_test.go") {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			forbidden := "gio" + "ui.org"
			if bytes.Contains(data, []byte(forbidden)) {
				t.Fatalf("file %s imports forbidden %s", p, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("source scan failed: %v", err)
	}

	tmpDir := t.TempDir()
	linuxBin := filepath.Join(tmpDir, "barbiton")

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", linuxBin, "./cmd/barbiton")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("linux build failed: %v\noutput: %s", err, string(out))
	}

	fi, err := os.Stat(linuxBin)
	if err != nil {
		t.Fatalf("linux binary not created: %v", err)
	}
	if fi.Size() > 15728640 {
		t.Fatalf("linux binary size %d exceeds 15 MB limit", fi.Size())
	}

	if nmPath, err := exec.LookPath("nm"); err == nil {
		nmCmd := exec.Command(nmPath, linuxBin)
		if nmOut, err := nmCmd.CombinedOutput(); err == nil {
			if strings.Contains(string(nmOut), "libc.so") {
				t.Fatal("linux binary contains libc.so symbols")
			}
		}
	}

	if lddPath, err := exec.LookPath("ldd"); err == nil {
		lddCmd := exec.Command(lddPath, linuxBin)
		lddOut, _ := lddCmd.CombinedOutput()
		forbidden := regexp.MustCompile("libX11|libwayland|libgio|libgtk|libQt")
		if forbidden.Match(lddOut) {
			t.Fatalf("linux binary dynamically links GUI libraries: %s", string(lddOut))
		}
	}

	windowsBin := filepath.Join(tmpDir, "barbiton.exe")
	winCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", windowsBin, "./cmd/barbiton")
	winCmd.Dir = root
	winCmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=windows", "GOARCH=amd64")
	if out, err := winCmd.CombinedOutput(); err != nil {
		t.Fatalf("windows cross-compile failed: %v\noutput: %s", err, string(out))
	}

	winFi, err := os.Stat(windowsBin)
	if err != nil || winFi.Size() == 0 {
		t.Fatalf("windows binary missing or empty: %v", err)
	}
}

func findProjectRoot() (string, error) {
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
	return "", os.ErrNotExist
}
