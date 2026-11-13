package score

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExamplesAndTestDataValidity(t *testing.T) {
	dirs := []string{
		filepath.Join("..", "..", "test_data", "examples"),
		filepath.Join("..", "..", "test_data"),
	}

	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Fatalf("failed to read dir %s: %v", d, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			p := filepath.Join(d, e.Name())
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("failed to read %s: %v", p, err)
			}
			sc, err := ParseJSON(data)
			if err != nil {
				t.Fatalf("ParseJSON failed for %s: %v", p, err)
			}
			if err := Validate(sc); err != nil {
				t.Fatalf("Validate failed for %s: %v", p, err)
			}
		}
	}
}
