package midi

import (
	"testing"
)

func TestGMProgramMappingFamilies(t *testing.T) {
	for prog := 0; prog <= 7; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "sampler" {
			t.Fatalf("program %d (pianos): expected sampler, got %s", prog, eng)
		}
	}
	for prog := 8; prog <= 15; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "sampler" {
			t.Fatalf("program %d (chromatic): expected sampler, got %s", prog, eng)
		}
	}
	for prog := 16; prog <= 23; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "synth" {
			t.Fatalf("program %d (organ): expected synth, got %s", prog, eng)
		}
	}
	for prog := 24; prog <= 31; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "synth" {
			t.Fatalf("program %d (guitar): expected synth, got %s", prog, eng)
		}
	}
	for prog := 32; prog <= 39; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "synth" {
			t.Fatalf("program %d (bass): expected synth, got %s", prog, eng)
		}
	}
	for prog := 40; prog <= 55; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "synth" {
			t.Fatalf("program %d (strings/ensemble): expected synth, got %s", prog, eng)
		}
	}
	for prog := 56; prog <= 63; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "synth" {
			t.Fatalf("program %d (brass): expected synth, got %s", prog, eng)
		}
	}
	for prog := 64; prog <= 79; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "ocarina" {
			t.Fatalf("program %d (reeds/pipes): expected ocarina, got %s", prog, eng)
		}
	}
	for prog := 80; prog <= 87; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "synth" {
			t.Fatalf("program %d (leads): expected synth, got %s", prog, eng)
		}
	}
	for prog := 88; prog <= 103; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "synth" {
			t.Fatalf("program %d (pads/fx): expected synth, got %s", prog, eng)
		}
	}
	for prog := 112; prog <= 119; prog++ {
		if eng := GMEngineForProgram(prog, false); eng != "sampler" {
			t.Fatalf("program %d (percussive): expected sampler, got %s", prog, eng)
		}
	}

	for prog := 0; prog <= 127; prog++ {
		if eng := GMEngineForProgram(prog, true); eng != "sampler" {
			t.Fatalf("program %d on percussion channel: expected sampler, got %s", prog, eng)
		}
	}
}

func TestMapGMInstrumentDefs(t *testing.T) {
	instPiano := mapGMInstrument(0, false)
	if instPiano.Type != "sampler" || instPiano.File == "" {
		t.Fatalf("piano instrument mismatch: %+v", instPiano)
	}

	instLead := mapGMInstrument(80, false)
	if instLead.Type != "synth" {
		t.Fatalf("synth lead instrument mismatch: %+v", instLead)
	}

	instFlute := mapGMInstrument(73, false)
	if instFlute.Type != "ocarina" {
		t.Fatalf("flute instrument mismatch: %+v", instFlute)
	}

	instDrums := mapGMInstrument(0, true)
	if instDrums.Type != "sampler" || instDrums.File == "" {
		t.Fatalf("drum instrument mismatch: %+v", instDrums)
	}
}
