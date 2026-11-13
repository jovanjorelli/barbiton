package synth

import (
	"testing"
)

func TestFinalRetuneBranches(t *testing.T) {
	s := New(48000)
	s.cutoff = 5
	s.resonance = -2
	s.retuneFilter()
	if s.filterAlpha <= 0 {
		t.Fatal("low cutoff killed the coefficient")
	}
	s.cutoff = 1e12
	s.resonance = 7
	s.retuneFilter()
	if !(s.filterAlpha > 0) || s.filterAlpha > 1 {
		t.Fatalf("alpha = %v, want (0,1]", s.filterAlpha)
	}
	if s.filterRes != maxRes {
		t.Fatalf("res = %v, want %v", s.filterRes, maxRes)
	}

	s.NoteOn(60, 0.8)
	if got := s.ActiveVoices(); got != 1 {
		t.Fatalf("voices = %d", got)
	}
}
