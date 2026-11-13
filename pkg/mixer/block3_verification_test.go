package mixer

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"

	"barbiton/pkg/dsp"
)

func TestBlock3MixerThresholdAdaptiveBypass(t *testing.T) {
	for numTracks := 1; numTracks <= 12; numTracks++ {
		seq := New(48000, numTracks, 256)
		par := New(48000, numTracks, 256)
		par.SetParallel(true)

		for i := 0; i < numTracks; i++ {
			lvl := float32(0.05 * float64(i+1))
			pan := float32(-1.0 + 2.0*float64(i)/float64(max(1, numTracks-1)))
			seq.AddTrack(&dcInstrument{level: lvl}, pan, 0.8)
			par.AddTrack(&dcInstrument{level: lvl}, pan, 0.8)
			seq.SetSend(i, 0.2)
			par.SetSend(i, 0.2)
		}

		for b := 0; b < 4; b++ {
			seq.Process(256)
			par.Process(256)

			sMaster := seq.Master()
			pMaster := par.Master()
			if len(sMaster) != len(pMaster) {
				t.Fatalf("tracks %d block %d len mismatch: %d vs %d", numTracks, b, len(sMaster), len(pMaster))
			}
			for j := range sMaster {
				if math.Abs(float64(sMaster[j]-pMaster[j])) > 1e-6 {
					t.Fatalf("tracks %d block %d sample %d mismatch: seq %v vs par %v", numTracks, b, j, sMaster[j], pMaster[j])
				}
			}
		}
	}
}

func TestBlock3MixerSingleThreadedBypass(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(oldProcs)

	m := New(48000, 8, 256)
	m.SetParallel(true)
	for i := 0; i < 8; i++ {
		m.AddTrack(&dcInstrument{level: 0.1}, 0, 0.8)
	}
	m.Process(256)
	if m.Err() != nil {
		t.Fatalf("unexpected error on single threaded bypass: %v", m.Err())
	}
}

func TestBlock3MixerZeroAllocUnderLoad(t *testing.T) {
	m := New(48000, 16, 256)
	m.SetParallel(true)
	m.SetReverbWet(0.2)
	for i := 0; i < 16; i++ {
		m.AddTrack(&dcInstrument{level: 0.05}, float32(i-8)/8.0, 0.7)
		m.SetSend(i, 0.3)
	}
	m.Process(256)
	allocs := testing.AllocsPerRun(50, func() {
		m.Process(256)
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocations under parallel 16-track load, got %v", allocs)
	}
}

func TestBlock3MixerUnifiedBufferAccumulation(t *testing.T) {
	m := New(48000, 4, 128)
	m.SetReverbWet(0.3)
	i1 := m.AddTrack(&dcInstrument{level: 0.4}, -0.5, 0.7)
	m.SetSend(i1, 0.4)
	m.SetTrackDelay(i1, dsp.DelayConfig{DelaySeconds: 0.02, Feedback: 0.3, Wet: 0.5}, 120)

	i2 := m.AddTrack(&dcInstrument{level: -0.3}, 0.5, 0.6)
	m.SetSend(i2, 0.2)

	m.AddTrack(&dcInstrument{level: 0.2}, 0.0, 0.8)

	m.Process(128)
	out := m.Master()
	if len(out) != 256 {
		t.Fatalf("expected 256 samples, got %d", len(out))
	}
	for _, v := range out {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatal("accumulateTrack produced NaN/Inf")
		}
	}
}

func TestBlock3MixerPanicsAcrossThresholds(t *testing.T) {
	for _, numTracks := range []int{2, 8, 12} {
		m := New(48000, numTracks, 128)
		m.SetParallel(true)
		for i := 0; i < numTracks; i++ {
			if i == 0 || i == numTracks-1 {
				m.AddTrack(&multiPanicInstrument{msg: fmt.Sprintf("crash_%d", i)}, 0, 1.0)
			} else {
				m.AddTrack(&dcInstrument{level: 0.1}, 0, 0.8)
			}
		}
		m.Process(128)
		err := m.Err()
		if err == nil {
			t.Fatalf("expected panics recorded for %d tracks, got nil", numTracks)
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "crash_0") {
			t.Fatalf("missing crash_0 in error message: %s", errMsg)
		}
		lastCrash := fmt.Sprintf("crash_%d", numTracks-1)
		if !strings.Contains(errMsg, lastCrash) {
			t.Fatalf("missing %s in error message: %s", lastCrash, errMsg)
		}
	}
}
