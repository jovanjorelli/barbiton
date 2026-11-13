package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestASTZeroComments(t *testing.T) {
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	fset := token.NewFileSet()
	var scanned int

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
		if filepath.Ext(p) != ".go" {
			return nil
		}

		scanned++
		f, parseErr := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if parseErr != nil {
			t.Errorf("failed to parse %s: %v", p, parseErr)
			return nil
		}

		if len(f.Comments) > 0 {
			for _, cg := range f.Comments {
				pos := fset.Position(cg.Pos())
				t.Errorf("%s:%d: comment found: %s", p, pos.Line, cg.Text())
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}

	if scanned == 0 {
		t.Fatal("no Go files scanned")
	}
}
