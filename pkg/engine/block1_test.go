package engine

import (
	"testing"

	"barbiton/pkg/score"
)

func TestBlock1WaltzMetricWeight(t *testing.T) {
	b1 := metricWeight(0, 3)
	b2 := metricWeight(1, 3)
	b3 := metricWeight(2, 3)

	if b1 != 1.0 {
		t.Fatalf("beat 1 want 1.0, got %v", b1)
	}
	if b2 != 0.85 {
		t.Fatalf("beat 2 want 0.85, got %v", b2)
	}
	if b3 > 0.75 {
		t.Fatalf("beat 3 in 3/4 waltz must be <= 0.75, got %v", b3)
	}
	if b3 >= b2 {
		t.Fatalf("beat 3 (%v) must be weaker than beat 2 (%v)", b3, b2)
	}
}

func TestBlock1PhraseBreathDurationShortening(t *testing.T) {
	tr := &score.Track{
		ID:       "solo",
		BreathMs: 100,
		Notes: []score.NoteEvent{
			{Time: 0.0, Duration: 1.0, Pitch: 60, PhraseID: 1},
			{Time: 1.0, Duration: 1.0, Pitch: 62, PhraseID: 2},
		},
	}
	times := breathTimes(tr, 0)
	durs := breathDurations(tr, 0)

	if times[0] != 0.0 || times[1] != 1.0 {
		t.Fatalf("note start times shifted: %v %v, want 0.0 and 1.0", times[0], times[1])
	}
	if mathAbs(durs[0]-0.9) > 1e-6 {
		t.Fatalf("first note duration not shortened to 0.9: got %v", durs[0])
	}
	if mathAbs(durs[1]-1.0) > 1e-6 {
		t.Fatalf("second note duration altered: got %v", durs[1])
	}
}

func TestBlock1PhraseBreathPreExistingRestUntouched(t *testing.T) {
	tr := &score.Track{
		ID:       "solo",
		BreathMs: 100,
		Notes: []score.NoteEvent{
			{Time: 0.0, Duration: 0.5, Pitch: 60, PhraseID: 1},
			{Time: 1.0, Duration: 1.0, Pitch: 62, PhraseID: 2},
		},
	}
	durs := breathDurations(tr, 0)
	if mathAbs(durs[0]-0.5) > 1e-6 {
		t.Fatalf("duration unexpectedly shortened when rest already existed: got %v", durs[0])
	}
}

func mathAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
