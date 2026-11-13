package mixer

import (
	"math"
	"testing"

	"barbiton/pkg/dsp"
)

func TestMixerTrackEffects(t *testing.T) {
	m := New(48000, 2, 256)
	idx := m.AddTrack(&dcInstrument{level: 0.5}, 0, 1.0)
	if idx < 0 {
		t.Fatal("failed to add track")
	}

	distCfg := dsp.DefaultDistortionConfig()
	distCfg.Mode = dsp.DistortionTube
	distCfg.Drive = 2.0
	distCfg.Mix = 1.0
	m.SetTrackDistortion(idx, distCfg)

	if m.TrackDistortion(idx) == nil {
		t.Fatal("expected track distortion to be set")
	}

	ottCfg := dsp.DefaultOTTConfig()
	ottCfg.Depth = 1.0
	m.SetTrackOTT(idx, ottCfg)

	if m.TrackOTT(idx) == nil {
		t.Fatal("expected track ott to be set")
	}

	m.SetOTTConfig(ottCfg)
	if !m.OTTEnabled() {
		t.Fatal("expected master OTT to be enabled")
	}

	m.Process(256)
	master := m.Master()
	var hasAudio bool
	for _, v := range master {
		if math.Abs(float64(v)) > 1e-4 {
			hasAudio = true
			break
		}
	}
	if !hasAudio {
		t.Fatal("expected audio output with track effects and master OTT")
	}

	allocs := testing.AllocsPerRun(100, func() {
		m.Process(256)
	})
	if allocs != 0 {
		t.Fatalf("Mixer.Process with track effects allocated %g objects, want 0", allocs)
	}
}

func TestMixerTurntableEffects(t *testing.T) {
	m := New(48000, 2, 256)
	idx := m.AddTrack(&dcInstrument{level: 0.5}, 0, 1.0)
	if idx < 0 {
		t.Fatal("failed to add track")
	}

	ttCfg := dsp.DefaultTurntableConfig()
	m.SetTrackTurntable(idx, ttCfg)
	if m.TrackTurntable(idx) == nil {
		t.Fatal("expected track turntable to be set")
	}

	m.SetTrackScratch(idx, -1.5)
	m.Process(256)

	m.SetTrackBeatRoll(idx, true, dsp.SubdivisionQuarter, 120.0)
	m.Process(256)

	m.SetTrackBrake(idx, 50.0, "linear")
	m.Process(256)

	m.SetTrackBackspin(idx, -2.5, 50.0)
	m.Process(256)

	m.SetTurntableConfig(ttCfg)
	if !m.TurntableEnabled() {
		t.Fatal("expected master turntable to be enabled")
	}

	m.SetMasterScratch(2.0)
	m.Process(256)

	m.SetMasterBeatRoll(true, dsp.SubdivisionSixteenth, 120.0)
	m.Process(256)

	m.SetMasterBrake(80.0, "exponential")
	m.Process(256)

	m.SetMasterBackspin(-3.0, 100.0)
	m.Process(256)

	m.SetTrackTurntableBypass(idx)
	if m.TrackTurntable(idx).Mode() != 0 {
		t.Fatal("expected track turntable mode 0 after bypass")
	}
	m.SetMasterTurntableBypass()
	if m.Turntable().Mode() != 0 {
		t.Fatal("expected master turntable mode 0 after bypass")
	}

	allocs := testing.AllocsPerRun(100, func() {
		m.Process(256)
	})
	if allocs != 0 {
		t.Fatalf("Mixer.Process with turntable allocated %g objects, want 0", allocs)
	}
}

func BenchmarkMixerTrackEffects(b *testing.B) {
	m := New(48000, 2, 1024)
	idx := m.AddTrack(&dcInstrument{level: 0.5}, 0, 1.0)
	m.SetTrackDistortion(idx, dsp.DefaultDistortionConfig())
	m.SetTrackOTT(idx, dsp.DefaultOTTConfig())
	m.SetOTTConfig(dsp.DefaultOTTConfig())
	m.Process(1024)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Process(1024)
	}
}
