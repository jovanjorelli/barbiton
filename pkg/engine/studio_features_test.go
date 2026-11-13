package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"barbiton/pkg/score"
	"barbiton/pkg/wav"
)

func TestEngineSidechainDuckingEndToEnd(t *testing.T) {
	jsonScore := `{
		"metadata": {
			"title": "Sidechain Test",
			"bpm": 120,
			"time_signature": "4/4"
		},
		"tracks": [
			{
				"id": "kick",
				"name": "Kick",
				"instrument": {
					"type": "synth",
					"parameters": {
						"cutoff": 200,
						"attack": 0.001,
						"decay": 0.1,
						"sustain": 0,
						"release": 0.05
					}
				},
				"pan": 0,
				"volume": 1,
				"notes": [
					{"time": 0.0, "duration": 0.1, "pitch": 36, "velocity": 1.0}
				]
			},
			{
				"id": "bass",
				"name": "Bass",
				"instrument": {
					"type": "synth",
					"parameters": {
						"cutoff": 1000,
						"attack": 0.001,
						"decay": 0.5,
						"sustain": 0.8,
						"release": 0.1
					}
				},
				"pan": 0,
				"volume": 1,
				"sidechain_source": "kick",
				"sidechain_ducking_db": -24,
				"sidechain_attack_ms": 2,
				"sidechain_release_ms": 80,
				"notes": [
					{"time": 0.0, "duration": 0.5, "pitch": 48, "velocity": 0.9}
				]
			}
		]
	}`

	var s score.Score
	if err := json.Unmarshal([]byte(jsonScore), &s); err != nil {
		t.Fatalf("unmarshal score: %v", err)
	}

	eng, err := New(&s, 48000, 256)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	duckedFound := false
	eng.ProcessFrames(48000, func(master []float32) {
		for _, v := range master {
			if v > 0 {
				duckedFound = true
				break
			}
		}
	})
	if !duckedFound {
		t.Fatalf("expected audio rendered")
	}
}

func TestEngineHumanizationScoreParams(t *testing.T) {
	jsonScore := `{
		"metadata": {
			"title": "Humanize Score Test",
			"bpm": 120,
			"time_signature": "4/4",
			"humanize_timing_ms": 15,
			"humanize_velocity": 0.1
		},
		"tracks": [
			{
				"id": "t1",
				"name": "Track 1",
				"instrument": {"type": "synth"},
				"pan": 0,
				"volume": 0.8,
				"notes": [
					{"time": 0.0, "duration": 0.25, "pitch": 60, "velocity": 0.8},
					{"time": 0.5, "duration": 0.25, "pitch": 62, "velocity": 0.8},
					{"time": 1.0, "duration": 0.25, "pitch": 64, "velocity": 0.8}
				]
			}
		]
	}`

	var s score.Score
	if err := json.Unmarshal([]byte(jsonScore), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	origTime := s.Tracks[0].Notes[1].Time
	origVel := s.Tracks[0].Notes[1].Velocity

	eng, err := New(&s, 48000, 256)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	engScore := eng.Score()
	modTime := engScore.Tracks[0].Notes[1].Time
	modVel := engScore.Tracks[0].Notes[1].Velocity

	if modTime == origTime && modVel == origVel {
		t.Fatalf("expected humanized jitter in time or velocity")
	}
}

func TestEngineMultibandAndOversampling(t *testing.T) {
	jsonScore := `{
		"metadata": {
			"title": "Mastering Test",
			"bpm": 120,
			"time_signature": "4/4",
			"multiband": true,
			"oversampling": true
		},
		"tracks": [
			{
				"id": "t1",
				"name": "Track 1",
				"instrument": {"type": "synth"},
				"pan": 0,
				"volume": 0.9,
				"notes": [
					{"time": 0.0, "duration": 0.2, "pitch": 60, "velocity": 1.0}
				]
			}
		]
	}`

	var s score.Score
	if err := json.Unmarshal([]byte(jsonScore), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	eng, err := New(&s, 48000, 256)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	if !eng.Mixer().MultibandEnabled() {
		t.Fatalf("expected multiband enabled")
	}
	if !eng.Mixer().OversamplingEnabled() {
		t.Fatalf("expected oversampling enabled")
	}

	eng.ProcessBlock()
}

func TestEngineConvolutionReverbIR(t *testing.T) {
	tmpDir := t.TempDir()
	irPath := filepath.Join(tmpDir, "test_ir.wav")
	f, err := os.Create(irPath)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wav.NewWriter(f, wav.PCM16, 48000, 1)
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	irSamples := make([]float32, 256)
	irSamples[0] = 0.8
	irSamples[100] = 0.4
	if _, err := w.WriteFrames(irSamples); err != nil {
		w.Close()
		f.Close()
		t.Fatal(err)
	}
	w.Close()
	f.Close()

	jsonScore := `{
		"metadata": {
			"title": "Convolution Test",
			"bpm": 120,
			"time_signature": "4/4",
			"convolution_ir": "` + filepath.ToSlash(irPath) + `",
			"convolution_dry_wet": 0.5
		},
		"tracks": [
			{
				"id": "t1",
				"name": "Track 1",
				"instrument": {"type": "synth"},
				"pan": 0,
				"volume": 0.8,
				"notes": [
					{"time": 0.0, "duration": 0.1, "pitch": 60, "velocity": 0.8}
				]
			}
		]
	}`

	var s score.Score
	if err := json.Unmarshal([]byte(jsonScore), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	eng, err := New(&s, 48000, 256)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	if !eng.Mixer().ConvolutionEnabled() {
		t.Fatalf("expected convolution enabled on mixer")
	}

	eng.ProcessBlock()
}
