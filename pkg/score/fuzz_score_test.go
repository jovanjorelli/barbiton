package score

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzScoreJSON(f *testing.F) {
	f.Add(`{"metadata": {"title": "T", "bpm": 120, "time_signature": "4/4"}, "tracks": [{"id": "s", "instrument": {"type": "ocarina"}, "volume": 0.8, "notes": [{"time": 0, "duration": 0.5, "pitch": 69, "velocity": 0.8}]}]}`)
	f.Add(`{"metadata": {"title": "T", "bpm": 120, "time_signature": "4/4"}, "tracks": []}`)
	f.Add(``)
	f.Add(`{"metadata": `)
	f.Add("\x00\x01\x02\xff\xfe")
	if data, err := os.ReadFile(filepath.Join("..", "..", "test_data", "examples", "simple_score.json")); err == nil {
		f.Add(string(data))
	}
	f.Fuzz(func(t *testing.T, data string) {
		s, err := ParseJSON([]byte(data))
		if err != nil {
			return
		}
		if s == nil {
			t.Fatal("nil score with nil error")
		}
		_ = Validate(s)
		_ = s.TotalNotes()
		_ = s.Duration()
	})
}
