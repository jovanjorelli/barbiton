package score

import (
	"math"
	"testing"
)

func TestAutomationInterpolateLinear(t *testing.T) {
	at := AutomationTrack{
		{Time: 1.0, Value: 10.0},
		{Time: 3.0, Value: 30.0, Curve: "linear"},
	}

	if v := at.Interpolate(0.0); v != 10.0 {
		t.Fatalf("before first point: got %g, want 10.0", v)
	}
	if v := at.Interpolate(1.0); v != 10.0 {
		t.Fatalf("at first point: got %g, want 10.0", v)
	}
	if v := at.Interpolate(2.0); math.Abs(v-20.0) > 1e-6 {
		t.Fatalf("at midpoint: got %g, want 20.0", v)
	}
	if v := at.Interpolate(3.0); v != 30.0 {
		t.Fatalf("at end point: got %g, want 30.0", v)
	}
	if v := at.Interpolate(4.0); v != 30.0 {
		t.Fatalf("after end point: got %g, want 30.0", v)
	}
}

func TestAutomationInterpolateStep(t *testing.T) {
	at := AutomationTrack{
		{Time: 0.0, Value: 5.0},
		{Time: 2.0, Value: 25.0, Curve: "step"},
	}
	if v := at.Interpolate(1.0); v != 5.0 {
		t.Fatalf("step before transition: got %g, want 5.0", v)
	}
	if v := at.Interpolate(2.0); v != 25.0 {
		t.Fatalf("step at transition: got %g, want 25.0", v)
	}
}

func TestAutomationInterpolateExponential(t *testing.T) {
	at := AutomationTrack{
		{Time: 0.0, Value: 100.0},
		{Time: 2.0, Value: 400.0, Curve: "exponential"},
	}
	vMid := at.Interpolate(1.0)
	if math.Abs(vMid-200.0) > 1e-3 {
		t.Fatalf("geometric midpoint: got %g, want 200.0", vMid)
	}

	atZero := AutomationTrack{
		{Time: 1.0, Value: 0.0},
		{Time: 1.25, Value: 8.0, Curve: "exponential"},
	}
	v0 := atZero.Interpolate(1.0)
	v1 := atZero.Interpolate(1.25)
	vm := atZero.Interpolate(1.125)
	if v0 != 0.0 || v1 != 8.0 {
		t.Fatalf("endpoints mismatch: v0=%g, v1=%g", v0, v1)
	}
	if vm <= 0.0 || vm >= 8.0 {
		t.Fatalf("midpoint out of bounds: %g", vm)
	}
}

func TestAutomationInterpolateBezier(t *testing.T) {
	at := AutomationTrack{
		{Time: 0.0, Value: 200.0},
		{Time: 1.5, Value: 4500.0, Curve: "bezier", Tension: 0.5},
	}
	v0 := at.Interpolate(0.0)
	v1 := at.Interpolate(1.5)
	vm := at.Interpolate(0.75)
	if v0 != 200.0 || v1 != 4500.0 {
		t.Fatalf("bezier endpoints: v0=%g, v1=%g", v0, v1)
	}
	if vm <= 200.0 || vm >= 4500.0 {
		t.Fatalf("bezier midpoint out of bounds: %g", vm)
	}
}

func TestAutomationMapValueAt(t *testing.T) {
	var am AutomationMap
	if v := am.ValueAt("cutoff", 1.0, 1000.0); v != 1000.0 {
		t.Fatalf("nil map fallback: got %g, want 1000", v)
	}

	am = AutomationMap{
		"cutoff": {
			{Time: 0.0, Value: 200.0},
			{Time: 1.5, Value: 4500.0, Curve: "bezier", Tension: 0.5},
		},
	}
	if !am.Has("cutoff") {
		t.Fatal("expected Has(cutoff) == true")
	}
	if am.Has("resonance") {
		t.Fatal("expected Has(resonance) == false")
	}
	if v := am.ValueAt("cutoff", 0.0, 0); v != 200.0 {
		t.Fatalf("ValueAt: got %g, want 200.0", v)
	}
}

func TestAutomationJSONScoreRoundtrip(t *testing.T) {
	raw := `{
		"metadata": {"title": "Test", "bpm": 120, "time_signature": "4/4"},
		"tracks": [
			{
				"id": "lead",
				"instrument": {"type": "synth"},
				"pan": 0,
				"volume": 0.8,
				"automation": {
					"cutoff": [{"time": 0.0, "value": 200.0}, {"time": 1.5, "value": 4500.0, "curve": "bezier", "tension": 0.5}],
					"fm_depth": [{"time": 1.0, "value": 0.0}, {"time": 1.25, "value": 8.0, "curve": "exponential"}],
					"lfo_rate": [{"time": 2.0, "value": 2.0}, {"time": 2.5, "value": 16.0, "curve": "linear"}],
					"pitch_bend": [{"time": 0.0, "value": 0.0}, {"time": 0.2, "value": -12.0}],
					"distortion_drive": [{"time": 0.0, "value": 1.0}, {"time": 1.0, "value": 10.0}]
				},
				"distortion": {"mode": "tube", "drive": 3.0, "mix": 0.7},
				"ott": {"depth": 0.8, "time_scale": 1.2, "band_gains": [1.0, 0.0, -1.0]},
				"notes": [{"time": 0.0, "duration": 2.0, "pitch": 60, "velocity": 0.9}]
			}
		]
	}`

	s, err := ParseJSON([]byte(raw))
	if err != nil {
		t.Fatalf("failed to parse score with automation: %v", err)
	}
	if len(s.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(s.Tracks))
	}
	tr := s.Tracks[0]
	if !tr.Automation.Has("cutoff") || !tr.Automation.Has("fm_depth") || !tr.Automation.Has("pitch_bend") {
		t.Fatal("missing automation parameters in parsed score")
	}
	if tr.Distortion == nil || tr.Distortion.Mode != "tube" || tr.Distortion.Drive != 3.0 {
		t.Fatalf("distortion parsed incorrectly: %+v", tr.Distortion)
	}
	if tr.OTT == nil || tr.OTT.Depth != 0.8 || tr.OTT.TimeScale != 1.2 {
		t.Fatalf("ott parsed incorrectly: %+v", tr.OTT)
	}

	clone := s.Clone()
	if clone.Tracks[0].Automation["cutoff"][0].Value != 200.0 {
		t.Fatal("clone failed for automation")
	}
}

func TestAutomationValidation(t *testing.T) {
	bad := AutomationMap{
		"test": {
			{Time: -1.0, Value: 10.0},
		},
	}
	if errs := ValidateAutomationMap(bad); len(errs) == 0 {
		t.Fatal("expected error for negative time")
	}

	badCurve := AutomationMap{
		"test": {
			{Time: 1.0, Value: 10.0, Curve: "hyperbolic"},
		},
	}
	if errs := ValidateAutomationMap(badCurve); len(errs) == 0 {
		t.Fatal("expected error for invalid curve")
	}
}
