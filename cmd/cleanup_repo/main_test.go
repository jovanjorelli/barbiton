package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupRepo(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}

	strayWav := filepath.Join(tmpDir, "stray.wav")
	if err := os.WriteFile(strayWav, []byte("fake wav"), 0644); err != nil {
		t.Fatal(err)
	}
	strayJson := filepath.Join(tmpDir, "stray.json")
	if err := os.WriteFile(strayJson, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	legacyBin := filepath.Join(tmpDir, "bin")
	_ = os.MkdirAll(legacyBin, 0755)
	legacyExamples := filepath.Join(tmpDir, "examples")
	_ = os.MkdirAll(legacyExamples, 0755)
	_ = os.WriteFile(filepath.Join(legacyExamples, "ex.json"), []byte("{}"), 0644)
	legacySamples := filepath.Join(tmpDir, "samples")
	_ = os.MkdirAll(legacySamples, 0755)
	_ = os.WriteFile(filepath.Join(legacySamples, "s.wav"), []byte("data"), 0644)

	var buf bytes.Buffer
	if err := runCleanup(tmpDir, &buf); err != nil {
		t.Fatalf("runCleanup failed: %v", err)
	}

	for _, sub := range []string{"test_data", filepath.Join("test_data", "examples"), filepath.Join("test_data", "samples")} {
		info, err := os.Stat(filepath.Join(tmpDir, sub))
		if err != nil || !info.IsDir() {
			t.Fatalf("directory %s not created", sub)
		}
	}

	for _, legacy := range []string{"bin", "examples", "samples"} {
		if _, err := os.Stat(filepath.Join(tmpDir, legacy)); !os.IsNotExist(err) {
			t.Fatalf("legacy directory %s still exists at root", legacy)
		}
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "test_data", "examples", "ex.json")); err != nil {
		t.Fatalf("ex.json not migrated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "test_data", "samples", "s.wav")); err != nil {
		t.Fatalf("s.wav not migrated: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "test_data", "stray.wav")); err != nil {
		t.Fatalf("stray.wav not moved to test_data: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "test_data", "stray.json")); err != nil {
		t.Fatalf("stray.json not moved to test_data: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".gitignore")); err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}

	buf.Reset()
	if err := runCleanup(tmpDir, &buf); err != nil {
		t.Fatalf("second runCleanup failed: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Root already clean")) {
		t.Fatalf("expected already clean message, got: %s", buf.String())
	}

	subDir := filepath.Join(tmpDir, "pkg", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"file.bak", "file.tmp", "file.swp", "file~", "#lock#", "profile.prof", "coverage.out"} {
		if err := os.WriteFile(filepath.Join(subDir, name), []byte("junk"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "index.tmp"), []byte("git-internal"), 0644); err != nil {
		t.Fatal(err)
	}

	buf.Reset()
	if err := runCleanup(tmpDir, &buf); err != nil {
		t.Fatalf("cleanup with junk failed: %v", err)
	}

	for _, name := range []string{"file.bak", "file.tmp", "file.swp", "file~", "#lock#", "profile.prof", "coverage.out"} {
		if _, err := os.Stat(filepath.Join(subDir, name)); !os.IsNotExist(err) {
			t.Fatalf("junk %s was not removed", name)
		}
	}
	if _, err := os.Stat(filepath.Join(gitDir, "index.tmp")); err != nil {
		t.Fatalf("file inside .git was unexpectedly modified: %v", err)
	}
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod not found at detected root: %s", root)
	}

	_, err = findRepoFrom("/")
	if err == nil {
		t.Fatal("expected error finding repo from root /")
	}
}

func TestCleanupRepoRun(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run(&out, &errOut); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	_ = os.Chdir("/")
	errOut.Reset()
	if err := run(&out, &errOut); err == nil {
		t.Fatal("expected error running from /")
	}
}

func TestCleanupRepoMain(t *testing.T) {
	main()
}

func TestCleanupRepoErrors(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(tmpFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err := runCleanup(filepath.Join(tmpFile, "invalid"), &buf)
	if err == nil {
		t.Fatal("expected error when root is inside regular file")
	}

	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, ".gitignore"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := runCleanup(tmpDir, &buf); err == nil {
		t.Fatal("expected error when .gitignore cannot be written")
	}

	strayWav := filepath.Join(tmpDir, "bad.wav")
	if err := os.WriteFile(strayWav, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(filepath.Join(tmpDir, "test_data"))
	if err := os.WriteFile(filepath.Join(tmpDir, "test_data"), []byte("not-a-dir"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := runCleanup(tmpDir, &buf); err == nil {
		t.Fatal("expected error when test_data cannot be created as directory")
	}

	renameDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(renameDir, "stray.wav"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(renameDir, "test_data", "stray.wav"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := runCleanup(renameDir, &buf); err == nil {
		t.Fatal("expected error when rename target is a directory")
	}

	noWriteDir := t.TempDir()
	for _, sub := range []string{"test_data", filepath.Join("test_data", "examples"), filepath.Join("test_data", "samples")} {
		_ = os.MkdirAll(filepath.Join(noWriteDir, sub), 0755)
	}
	_ = os.Chmod(noWriteDir, 0555)
	defer func() { _ = os.Chmod(noWriteDir, 0755) }()
	if err := runCleanup(noWriteDir, &buf); err == nil {
		t.Fatal("expected error writing .gitignore in read-only dir")
	}

	noReadDir := t.TempDir()
	for _, sub := range []string{"test_data", filepath.Join("test_data", "examples"), filepath.Join("test_data", "samples")} {
		_ = os.MkdirAll(filepath.Join(noReadDir, sub), 0755)
	}
	_ = os.Chmod(noReadDir, 0311)
	defer func() { _ = os.Chmod(noReadDir, 0755) }()
	if err := runCleanup(noReadDir, &buf); err == nil {
		t.Fatal("expected error reading directory in non-readable dir")
	}

	loopDir := t.TempDir()
	loopPath := filepath.Join(loopDir, ".gitignore")
	if err := os.Symlink(loopPath, loopPath); err == nil {
		if err := runCleanup(loopDir, &buf); err == nil {
			t.Fatal("expected error on cyclic .gitignore symlink")
		}
	}

	runFailDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runFailDir, "go.mod"), []byte("module fail"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runFailDir, "test_data"), []byte("notadir"), 0644); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	_ = os.Chdir(runFailDir)
	var runOut, runErr bytes.Buffer
	if err := run(&runOut, &runErr); err == nil {
		t.Fatal("expected run to fail when runCleanup encounters invalid test_data path")
	}

	noRemoveDir := t.TempDir()
	for _, sub := range []string{"test_data", filepath.Join("test_data", "examples"), filepath.Join("test_data", "samples")} {
		_ = os.MkdirAll(filepath.Join(noRemoveDir, sub), 0755)
	}
	_ = os.WriteFile(filepath.Join(noRemoveDir, ".gitignore"), []byte("*.exe\n"), 0644)
	lockedSub := filepath.Join(noRemoveDir, "locked")
	_ = os.MkdirAll(lockedSub, 0755)
	_ = os.WriteFile(filepath.Join(lockedSub, "junk.bak"), []byte("data"), 0644)
	_ = os.Chmod(lockedSub, 0555)
	defer func() { _ = os.Chmod(lockedSub, 0755) }()
	if err := runCleanup(noRemoveDir, &buf); err == nil {
		t.Fatal("expected error removing junk from read-only directory")
	}
}
