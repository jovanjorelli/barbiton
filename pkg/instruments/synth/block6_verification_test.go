package synth

import (
	"math"
	"testing"

	"barbiton/pkg/dsp"
)

func TestBlock6Filter24dBSlope(t *testing.T) {
	sr := 48000.0
	fc := 1000.0

	calcSineGain := func(ft FilterType, freq float64) float64 {
		frames := 2048
		cSVF := calcSVFCoeffs(fc, 0, sr)
		cLad := calcLadderCoeffs(fc, 0, sr)

		var s1, s2, s3, s4 float64
		var sumSq float64
		for i := 0; i < frames; i++ {
			in := math.Sin(2 * math.Pi * freq * float64(i) / sr)
			var y float64
			switch ft {
			case Filter1Pole:
				alpha := 1.0 - math.Exp(-2.0*math.Pi*fc/sr)
				s1 += alpha * (in - s1)
				y = s1
			case Filter12dBSVF:
				y = processSVF12(in, cSVF, &s1, &s2)
			case Filter24dBCascade:
				y = processSVF24(in, cSVF, &s1, &s2, &s3, &s4)
			case Filter24dBLadder:
				y = processLadder24(in, cLad, &s1, &s2, &s3, &s4)
			}
			if i >= 512 {
				sumSq += y * y
			}
		}
		return math.Sqrt(sumSq / float64(frames-512))
	}

	gainPass1 := calcSineGain(Filter1Pole, 100)
	gainOct1 := calcSineGain(Filter1Pole, 2000)
	db1 := 20 * math.Log10(gainOct1/gainPass1)

	gainPass12 := calcSineGain(Filter12dBSVF, 100)
	gainOct12 := calcSineGain(Filter12dBSVF, 2000)
	db12 := 20 * math.Log10(gainOct12/gainPass12)

	gainPass24C := calcSineGain(Filter24dBCascade, 100)
	gainOct24C := calcSineGain(Filter24dBCascade, 2000)
	db24C := 20 * math.Log10(gainOct24C/gainPass24C)

	gainPass24L := calcSineGain(Filter24dBLadder, 100)
	gainOct24L := calcSineGain(Filter24dBLadder, 2000)
	db24L := 20 * math.Log10(gainOct24L/gainPass24L)

	if db1 > -5.0 || db1 < -9.0 {
		t.Fatalf("Filter1Pole at 2kHz = %v dB, want ~-6 to -7 dB", db1)
	}
	if db12 > -10.0 || db12 < -16.0 {
		t.Fatalf("Filter12dBSVF at 2kHz = %v dB, want ~-12 dB", db12)
	}
	if db24C > -20.0 || db24C < -30.0 {
		t.Fatalf("Filter24dBCascade at 2kHz = %v dB, want ~-24 dB", db24C)
	}
	if db24L > -20.0 || db24L < -30.0 {
		t.Fatalf("Filter24dBLadder at 2kHz = %v dB, want ~-24 dB", db24L)
	}
}

func TestBlock6ResonanceStability(t *testing.T) {
	filterTypes := []FilterType{Filter1Pole, Filter12dBSVF, Filter24dBCascade, Filter24dBLadder}
	cutoffs := []float32{50, 500, 2000, 10000, 18000}
	for _, ft := range filterTypes {
		for _, fc := range cutoffs {
			s := New(48000)
			s.SetParameters(map[string]float32{
				"filter_type": float32(ft),
				"cutoff":      fc,
				"resonance":   1.0,
				"attack":      0.001,
				"sustain":     1.0,
			})
			buf := dsp.NewStereoBuffer(48000)
			buf.SetLen(48000)
			s.NoteOn(60, 1.0)
			s.Process(buf, 1.0/48000)

			maxPeak := float32(0)
			for i := range buf.Left {
				if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
					t.Fatalf("filter type %v cutoff %v produced NaN/Inf at frame %d", ft, fc, i)
				}
				if a := float32(math.Abs(float64(buf.Left[i]))); a > maxPeak {
					maxPeak = a
				}
			}
			if maxPeak > 3.5 {
				t.Fatalf("filter type %v cutoff %v exploded: peak = %v", ft, fc, maxPeak)
			}
		}
	}
}

func TestBlock6FilterEnvelopeResponse(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"filter_type":      float32(Filter24dBCascade),
		"cutoff":           100,
		"resonance":        0.1,
		"filter_attack":    0.01,
		"filter_decay":     0.05,
		"filter_sustain":   0.2,
		"filter_release":   0.05,
		"filter_env_depth": 5.0,
		"attack":           0.001,
		"sustain":          1.0,
	})

	s.NoteOn(60, 1.0)
	if s.voices[0].filtEnv.Stage() != dsp.StageAttack {
		t.Fatalf("expected filter env stage Attack, got %v", s.voices[0].filtEnv.Stage())
	}

	buf := dsp.NewStereoBuffer(480)
	buf.SetLen(480)
	s.Process(buf, 1.0/48000)

	bufDecay := dsp.NewStereoBuffer(4800)
	bufDecay.SetLen(4800)
	s.Process(bufDecay, 1.0/48000)

	s.NoteOff(60)
	if s.voices[0].filtEnv.Stage() != dsp.StageRelease {
		t.Fatalf("expected filter env stage Release, got %v", s.voices[0].filtEnv.Stage())
	}

	sFast := New(48000)
	sFast.SetParameters(map[string]float32{
		"cutoff":           1000,
		"filter_env_depth": 0,
	})
	bufFast := dsp.NewStereoBuffer(256)
	bufFast.SetLen(256)
	sFast.NoteOn(60, 1.0)
	sFast.Process(bufFast, 1.0/48000)

	sStatic := New(48000)
	sStatic.SetParameters(map[string]float32{
		"cutoff": 1000,
	})
	bufStatic := dsp.NewStereoBuffer(256)
	bufStatic.SetLen(256)
	sStatic.NoteOn(60, 1.0)
	sStatic.Process(bufStatic, 1.0/48000)

	for i := range bufFast.Left {
		if bufFast.Left[i] != bufStatic.Left[i] {
			t.Fatalf("fast path depth==0 differed from static at frame %d", i)
		}
	}
}

func TestBlock6DynamicEnvAllFilterTypes(t *testing.T) {
	types := []FilterType{Filter1Pole, Filter12dBSVF, Filter24dBCascade, Filter24dBLadder}
	for _, ft := range types {
		s := New(48000)
		s.SetParameters(map[string]float32{
			"filter_type":      float32(ft),
			"cutoff":           500,
			"filter_env_depth": 4.0,
			"filter_attack":    0.01,
			"filter_decay":     0.02,
			"filter_sustain":   0.5,
			"filter_release":   0.02,
		})
		buf := dsp.NewStereoBuffer(512)
		buf.SetLen(512)
		s.NoteOn(60, 0.8)
		s.Process(buf, 1.0/48000)
		for i := range buf.Left {
			if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
				t.Fatalf("dynamic env filter type %v produced NaN/Inf", ft)
			}
		}

		sNeg := New(48000)
		sNeg.SetParameters(map[string]float32{
			"filter_type":      float32(ft),
			"cutoff":           200,
			"filter_env_depth": -8.0,
		})
		sNeg.NoteOn(60, 0.8)
		sNeg.Process(buf, 1.0/48000)
		for i := range buf.Left {
			if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
				t.Fatalf("negative dynamic env filter type %v produced NaN/Inf", ft)
			}
		}
	}
}

func TestBlock6WaveformsExpanded(t *testing.T) {
	if oscWave(WaveTriangle, 0.0) != 0.0 {
		t.Fatalf("triangle at 0 got %v, want 0", oscWave(WaveTriangle, 0.0))
	}
	if math.Abs(oscWave(WaveTriangle, 0.25)-1.0) > 1e-4 {
		t.Fatalf("triangle at 0.25 got %v, want 1", oscWave(WaveTriangle, 0.25))
	}
	if math.Abs(oscWave(WaveTriangle, 0.5)-0.0) > 1e-4 {
		t.Fatalf("triangle at 0.5 got %v, want 0", oscWave(WaveTriangle, 0.5))
	}
	if math.Abs(oscWave(WaveTriangle, 0.75)-(-1.0)) > 1e-4 {
		t.Fatalf("triangle at 0.75 got %v, want -1", oscWave(WaveTriangle, 0.75))
	}

	dt := 440.0 / 48000.0
	for p := 0.0; p < 1.0; p += 0.01 {
		v := oscWaveBlep(WaveTriangle, p, dt)
		if math.IsNaN(v) || math.IsInf(v, 0) || v < -1.1 || v > 1.1 {
			t.Fatalf("triangle BLEP out of bounds at phase %v: %v", p, v)
		}
	}

	s1 := New(48000)
	s1.SetParameters(map[string]float32{"waveform1": float32(WaveNoise), "waveform2": float32(WaveNoise)})
	s2 := New(48000)
	s2.SetParameters(map[string]float32{"waveform1": float32(WaveNoise), "waveform2": float32(WaveNoise)})

	buf1 := dsp.NewStereoBuffer(512)
	buf1.SetLen(512)
	buf2 := dsp.NewStereoBuffer(512)
	buf2.SetLen(512)

	s1.NoteOn(60, 1.0)
	s2.NoteOn(60, 1.0)
	s1.Process(buf1, 1.0/48000)
	s2.Process(buf2, 1.0/48000)

	for i := range buf1.Left {
		if buf1.Left[i] != buf2.Left[i] {
			t.Fatalf("noise not deterministic at frame %d: %v vs %v", i, buf1.Left[i], buf2.Left[i])
		}
	}

	sPink := New(48000)
	sPink.SetParameters(map[string]float32{"waveform1": float32(WavePinkNoise), "waveform2": float32(WavePinkNoise)})
	bufPink := dsp.NewStereoBuffer(1024)
	bufPink.SetLen(1024)
	sPink.NoteOn(60, 1.0)
	sPink.Process(bufPink, 1.0/48000)

	hasSignal := false
	for _, v := range bufPink.Left {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatal("pink noise produced NaN/Inf")
		}
		if math.Abs(float64(v)) > 0.001 {
			hasSignal = true
		}
	}
	if !hasSignal {
		t.Fatal("pink noise output was silent")
	}

	if waveParam(3) != WaveTriangle {
		t.Fatalf("waveParam(3) = %v, want WaveTriangle", waveParam(3))
	}
	if waveParam(4) != WaveNoise {
		t.Fatalf("waveParam(4) = %v, want WaveNoise", waveParam(4))
	}
	if waveParam(5) != WavePinkNoise {
		t.Fatalf("waveParam(5) = %v, want WavePinkNoise", waveParam(5))
	}
	if waveParam(10) != WaveSaw {
		t.Fatalf("waveParam(10) = %v, want WaveSaw", waveParam(10))
	}
}

func TestBlock6PWMContinuity(t *testing.T) {
	dt := 440.0 / 48000.0

	for duty := 0.05; duty <= 0.95; duty += 0.05 {
		prev := oscWaveBlepPWM(WaveSquare, 0, dt, duty)
		for phase := 0.001; phase < 1.0; phase += 0.001 {
			cur := oscWaveBlepPWM(WaveSquare, phase, dt, duty)
			if math.IsNaN(cur) || math.IsInf(cur, 0) {
				t.Fatalf("PWM produced NaN/Inf at duty %v phase %v", duty, phase)
			}
			prev = cur
		}
		_ = prev
	}

	s := New(48000)
	s.SetParameters(map[string]float32{
		"waveform1":    float32(WaveSquare),
		"waveform2":    float32(WaveSquare),
		"pulse_width":  0.25,
		"pulse_width1": 0.30,
		"pulse_width2": 0.70,
		"pwm":          0.40,
		"pw":           0.35,
	})

	if math.Abs(s.pulseWidth1-0.30) > 1e-4 {
		t.Fatalf("pulseWidth1 = %v, want 0.30", s.pulseWidth1)
	}
	if math.Abs(s.pulseWidth2-0.70) > 1e-4 {
		t.Fatalf("pulseWidth2 = %v, want 0.70", s.pulseWidth2)
	}

	sPWM := New(48000)
	sPWM.SetParameters(map[string]float32{"pwm": 0.40})
	if math.Abs(sPWM.pulseWidth1-0.40) > 1e-4 || math.Abs(sPWM.pulseWidth2-0.40) > 1e-4 {
		t.Fatalf("pwm = %v/%v, want 0.40", sPWM.pulseWidth1, sPWM.pulseWidth2)
	}

	sPW := New(48000)
	sPW.SetParameters(map[string]float32{"pw": 0.35})
	if math.Abs(sPW.pulseWidth1-0.35) > 1e-4 || math.Abs(sPW.pulseWidth2-0.35) > 1e-4 {
		t.Fatalf("pw = %v/%v, want 0.35", sPW.pulseWidth1, sPW.pulseWidth2)
	}

	buf := dsp.NewStereoBuffer(1024)
	buf.SetLen(1024)
	s.NoteOn(60, 1.0)
	s.Process(buf, 1.0/48000)

	for i := range buf.Left {
		if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
			t.Fatalf("synth PWM produced NaN/Inf at frame %d", i)
		}
	}
}

func TestBlock6PolyBLAMPAndCoeffEdgeCases(t *testing.T) {
	if polyBLAMP(0.1, -1) != 0 {
		t.Fatal("polyBLAMP(-1) != 0")
	}
	if polyBLAMP(0.1, 0.6) != 0 {
		t.Fatal("polyBLAMP(dt>=0.5) != 0")
	}
	if polyBLAMP(-0.1, 0.1) != 0 {
		t.Fatal("polyBLAMP(t<0) != 0")
	}
	if polyBLAMP(1.5, 0.1) != 0 {
		t.Fatal("polyBLAMP(t>1) != 0")
	}

	svfMin := calcSVFCoeffs(1, -1, 48000)
	if svfMin.g <= 0 {
		t.Fatal("calcSVFCoeffs min g <= 0")
	}
	svfMax := calcSVFCoeffs(100000, 2, 48000)
	if svfMax.g <= 0 {
		t.Fatal("calcSVFCoeffs max g <= 0")
	}

	ladMin := calcLadderCoeffs(1, -1, 48000)
	if ladMin.G <= 0 {
		t.Fatal("calcLadderCoeffs min G <= 0")
	}
	ladMax := calcLadderCoeffs(100000, 2, 48000)
	if ladMax.G <= 0 {
		t.Fatal("calcLadderCoeffs max G <= 0")
	}

	var zeroSeed uint64
	x := xorshift64Step(&zeroSeed)
	if math.IsNaN(x) {
		t.Fatal("xorshift64Step zero seed produced NaN")
	}

	wClampLow := oscWaveBlepPWM(WaveSquare, 0.1, 0.01, 0.001)
	if math.IsNaN(wClampLow) {
		t.Fatal("oscWaveBlepPWM clamp low produced NaN")
	}
	wClampHigh := oscWaveBlepPWM(WaveSquare, 0.1, 0.01, 0.999)
	if math.IsNaN(wClampHigh) {
		t.Fatal("oscWaveBlepPWM clamp high produced NaN")
	}
	wNaive := oscWaveBlepPWM(WaveSquare, 0.1, 0, 0.5)
	if wNaive != 1.0 {
		t.Fatalf("oscWaveBlepPWM dt<=0 got %v, want 1", wNaive)
	}

	if waveParam(float32(math.NaN())) != WaveSaw {
		t.Fatal("waveParam(NaN) != WaveSaw")
	}
	if filterParam(float32(math.NaN())) != Filter1Pole {
		t.Fatal("filterParam(NaN) != Filter1Pole")
	}
	svfNaN := calcSVFCoeffs(math.NaN(), math.NaN(), 48000)
	if math.IsNaN(svfNaN.g) || math.IsNaN(svfNaN.k) || math.IsNaN(svfNaN.d) {
		t.Fatal("calcSVFCoeffs NaN produced NaN coefficients")
	}
	ladNaN := calcLadderCoeffs(math.NaN(), math.NaN(), 48000)
	if math.IsNaN(ladNaN.G) || math.IsNaN(ladNaN.resAmount) || math.IsNaN(ladNaN.denom) {
		t.Fatal("calcLadderCoeffs NaN produced NaN coefficients")
	}
	wNaN := oscWaveBlepPWM(WaveSquare, 0.5, 0.01, math.NaN())
	if math.IsNaN(wNaN) || math.IsInf(wNaN, 0) {
		t.Fatal("oscWaveBlepPWM NaN duty produced NaN/Inf")
	}
}

func TestBlock6PinkNoiseSpectralTilt(t *testing.T) {
	n := 96000
	var whiteSeed uint64 = 123456789
	var pinkSeed uint64 = 123456789
	var pinkState [7]float64

	var whiteHighEnergy, whiteLowEnergy float64
	var pinkHighEnergy, pinkLowEnergy float64

	lpAlpha := 1.0 - math.Exp(-2.0*math.Pi*1000.0/48000.0)
	var whiteLP, pinkLP float64

	for i := 0; i < n; i++ {
		w := xorshift64Step(&whiteSeed)
		whiteLP += lpAlpha * (w - whiteLP)
		whiteHP := w - whiteLP
		if i >= 4096 {
			whiteLowEnergy += whiteLP * whiteLP
			whiteHighEnergy += whiteHP * whiteHP
		}

		p := kelletPinkStep(&pinkState, xorshift64Step(&pinkSeed))
		pinkLP += lpAlpha * (p - pinkLP)
		pinkHP := p - pinkLP
		if i >= 4096 {
			pinkLowEnergy += pinkLP * pinkLP
			pinkHighEnergy += pinkHP * pinkHP
		}
	}

	whiteRatio := whiteLowEnergy / whiteHighEnergy
	pinkRatio := pinkLowEnergy / pinkHighEnergy

	if pinkRatio < whiteRatio*2.0 {
		t.Fatalf("pink noise spectral tilt ratio %v not > 2x white ratio %v", pinkRatio, whiteRatio)
	}
}

func TestBlock6BackwardCompatibilityBitExact(t *testing.T) {
	s := New(48000)
	buf := dsp.NewStereoBuffer(512)
	buf.SetLen(512)
	s.NoteOn(69, 0.8)
	s.Process(buf, 1.0/48000)

	sRef := New(48000)
	bufRef := dsp.NewStereoBuffer(512)
	bufRef.SetLen(512)
	sRef.NoteOn(69, 0.8)
	sRef.Process(bufRef, 1.0/48000)

	for i := range buf.Left {
		if buf.Left[i] != bufRef.Left[i] || buf.Right[i] != bufRef.Right[i] {
			t.Fatalf("mismatch at frame %d: got %v/%v, want %v/%v", i, buf.Left[i], buf.Right[i], bufRef.Left[i], bufRef.Right[i])
		}
	}
}

func TestBlock6ZeroAllocationsAllModes(t *testing.T) {
	buf := dsp.NewStereoBuffer(256)
	buf.SetLen(256)

	testConfigs := []map[string]float32{
		{"filter_type": float32(Filter1Pole)},
		{"filter_type": float32(Filter12dBSVF)},
		{"filter_type": float32(Filter24dBCascade)},
		{"filter_type": float32(Filter24dBLadder)},
		{"filter_type": float32(Filter24dBCascade), "filter_env_depth": 3.0},
		{"filter_type": float32(Filter24dBLadder), "filter_env_depth": -2.0},
		{"waveform1": float32(WaveTriangle), "waveform2": float32(WaveTriangle)},
		{"waveform1": float32(WaveNoise), "waveform2": float32(WaveNoise)},
		{"waveform1": float32(WavePinkNoise), "waveform2": float32(WavePinkNoise)},
		{"waveform1": float32(WaveSquare), "pulse_width": 0.3},
	}

	for idx, cfg := range testConfigs {
		s := New(48000)
		s.SetParameters(cfg)
		s.NoteOn(60, 0.9)
		s.Process(buf, 1.0/48000)

		allocs := testing.AllocsPerRun(50, func() {
			s.Process(buf, 1.0/48000)
		})
		if allocs != 0 {
			t.Fatalf("config %d allocated %v, want 0", idx, allocs)
		}
	}
}

func TestBlock6VoiceStealMidAttackResonancePWM(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{
		"waveform1":        float32(WaveSquare),
		"waveform2":        float32(WaveSquare),
		"filter_type":      float32(Filter24dBLadder),
		"cutoff":           1500,
		"resonance":        1.0,
		"filter_attack":    0.5,
		"filter_decay":     0.5,
		"filter_sustain":   0.3,
		"filter_release":   0.5,
		"filter_env_depth": 4.0,
		"attack":           0.5,
		"decay":            0.5,
		"sustain":          0.5,
		"release":          0.5,
		"pulse_width1":     0.05,
		"pulse_width2":     0.95,
	})

	buf := dsp.NewStereoBuffer(256)
	buf.SetLen(256)

	for p := 0; p < MaxVoices*2; p++ {
		s.NoteOn(40+p, 0.9)
		s.SetParameters(map[string]float32{
			"pulse_width1": 0.05 + float32(p)*0.02,
			"pulse_width2": 0.95 - float32(p)*0.02,
		})
		s.Process(buf, 1.0/48000)
		for i := range buf.Left {
			if math.IsNaN(float64(buf.Left[i])) || math.IsInf(float64(buf.Left[i]), 0) {
				t.Fatalf("voice stealing produced NaN/Inf at pitch %d frame %d", 40+p, i)
			}
		}
	}
}
