package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func findRepoFrom(dir string) (string, error) {
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
	return "", fmt.Errorf("repository root containing go.mod not found")
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return findRepoFrom(dir)
}

func runCleanup(root string, out io.Writer) error {
	for _, sub := range []string{"test_data", filepath.Join("test_data", "examples"), filepath.Join("test_data", "samples")} {
		p := filepath.Join(root, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			return err
		}
	}

	for _, badDir := range []string{"bin", "demo", "scripts"} {
		p := filepath.Join(root, badDir)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			if err := os.RemoveAll(p); err != nil {
				return err
			}
			if out != nil {
				fmt.Fprintf(out, "Removed legacy %s/\n", badDir)
			}
		}
	}

	legacyDirs := map[string]string{
		"examples": filepath.Join(root, "test_data", "examples"),
		"samples":  filepath.Join(root, "test_data", "samples"),
	}
	for srcName, dstDir := range legacyDirs {
		srcPath := filepath.Join(root, srcName)
		if info, err := os.Stat(srcPath); err == nil && info.IsDir() {
			entries, err := os.ReadDir(srcPath)
			if err == nil {
				for _, e := range entries {
					_ = os.Rename(filepath.Join(srcPath, e.Name()), filepath.Join(dstDir, e.Name()))
				}
			}
			_ = os.RemoveAll(srcPath)
			if out != nil {
				fmt.Fprintf(out, "Migrated %s/ -> test_data/%s/\n", srcName, srcName)
			}
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	strayExts := map[string]bool{
		".wav":  true,
		".mid":  true,
		".midi": true,
		".json": true,
		".sfz":  true,
	}

	moved := 0
	targetDir := filepath.Join(root, "test_data")
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if strayExts[ext] {
			src := filepath.Join(root, name)
			dst := filepath.Join(targetDir, name)
			if err := os.Rename(src, dst); err != nil {
				return err
			}
			if out != nil {
				fmt.Fprintf(out, "Moved %s -> test_data/\n", name)
			}
			moved++
		}
	}

	if moved == 0 && out != nil {
		fmt.Fprintln(out, "Root already clean: no stray artifacts.")
	}

	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".bak" || ext == ".tmp" || ext == ".swp" || ext == ".prof" || ext == ".out" || strings.HasSuffix(name, "~") || (strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#")) {
			if err := os.Remove(p); err != nil {
				return err
			}
			if out != nil {
				fmt.Fprintf(out, "Removed junk %s\n", name)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	gitIgnorePath := filepath.Join(root, ".gitignore")
	info, err := os.Stat(gitIgnorePath)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf(".gitignore is a directory")
		}
	} else if os.IsNotExist(err) {
		defaultIgnore := "*.exe\n/barbiton\n*.wav\n*.mid\n*.midi\n!test_data/*.mid\n!test_data/*.midi\n*.prof\n*.out\n"
		if err := os.WriteFile(gitIgnorePath, []byte(defaultIgnore), 0644); err != nil {
			return err
		}
		if out != nil {
			fmt.Fprintln(out, "Created default .gitignore")
		}
	} else {
		return err
	}

	return nil
}

func run(out, errOut io.Writer) error {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(errOut, "cleanup error: %v\n", err)
		return err
	}
	if err := runCleanup(root, out); err != nil {
		fmt.Fprintf(errOut, "cleanup error: %v\n", err)
		return err
	}
	return nil
}

func main() {
	if err := run(os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
