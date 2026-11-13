package dsp

import (
	"errors"
	"io"
	"math"
	"os"

	"barbiton/pkg/wav"
)

const (
	ConvPartitionSize = 128
	ConvFFTSize       = ConvPartitionSize * 2
	MaxConvPartitions = 512
	MaxPreDelaySec    = 0.5
)

type ConvolutionConfig struct {
	DecayRatio float32
	PreDelayMs float32
	DryWet     float32
	LowCutHz   float32
	HighCutHz  float32
}

func DefaultConvolutionConfig() ConvolutionConfig {
	return ConvolutionConfig{
		DecayRatio: 1.0,
		PreDelayMs: 0.0,
		DryWet:     0.3,
		LowCutHz:   20.0,
		HighCutHz:  18000.0,
	}
}

type ConvolutionReverb struct {
	sampleRate float64
	cfg        ConvolutionConfig

	rawIRL []float32
	rawIRR []float32

	partitions int
	hl         [][]complex128
	hr         [][]complex128
	histL      [][]complex128
	histR      [][]complex128
	histHead   int

	accumL []complex128
	accumR []complex128
	xBufL  []complex128
	xBufR  []complex128

	overlapL []float32
	overlapR []float32

	inBufL   [ConvPartitionSize]float32
	inBufR   [ConvPartitionSize]float32
	inPos    int
	outBufL  [ConvPartitionSize]float32
	outBufR  [ConvPartitionSize]float32
	outPos   int
	hasBlock bool

	preBufL  []float32
	preBufR  []float32
	preMask  int
	preWrite int
	preDelay int

	lowCutL  *Biquad
	lowCutR  *Biquad
	highCutL *Biquad
	highCutR *Biquad
}

func NewConvolutionReverb(sampleRate float64, cfg ConvolutionConfig) *ConvolutionReverb {
	if sampleRate <= 0 || math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) {
		sampleRate = 48000
	}
	preCap := 1
	for preCap <= int(MaxPreDelaySec*sampleRate) {
		preCap <<= 1
	}

	cr := &ConvolutionReverb{
		sampleRate: sampleRate,
		cfg:        cfg,
		accumL:     make([]complex128, ConvFFTSize),
		accumR:     make([]complex128, ConvFFTSize),
		xBufL:      make([]complex128, ConvFFTSize),
		xBufR:      make([]complex128, ConvFFTSize),
		overlapL:   make([]float32, ConvPartitionSize),
		overlapR:   make([]float32, ConvPartitionSize),
		preBufL:    make([]float32, preCap),
		preBufR:    make([]float32, preCap),
		preMask:    preCap - 1,
		lowCutL:    NewBiquad(Highpass, 20.0, 0.707, sampleRate),
		lowCutR:    NewBiquad(Highpass, 20.0, 0.707, sampleRate),
		highCutL:   NewBiquad(Lowpass, 18000.0, 0.707, sampleRate),
		highCutR:   NewBiquad(Lowpass, 18000.0, 0.707, sampleRate),
	}
	cr.SetConfig(cfg)
	return cr
}

func (cr *ConvolutionReverb) SetConfig(cfg ConvolutionConfig) {
	if cfg.DecayRatio <= 0.05 {
		cfg.DecayRatio = 0.05
	}
	if cfg.DecayRatio > 10.0 {
		cfg.DecayRatio = 10.0
	}
	if cfg.DryWet < 0 {
		cfg.DryWet = 0
	} else if cfg.DryWet > 1 {
		cfg.DryWet = 1
	}
	if cfg.PreDelayMs < 0 {
		cfg.PreDelayMs = 0
	} else if cfg.PreDelayMs > MaxPreDelaySec*1000.0 {
		cfg.PreDelayMs = MaxPreDelaySec * 1000.0
	}

	cr.cfg = cfg
	cr.preDelay = int(math.Round(float64(cfg.PreDelayMs) * 0.001 * cr.sampleRate))

	if cfg.LowCutHz > 10.0 && cfg.LowCutHz < float32(cr.sampleRate*0.45) {
		cr.lowCutL.SetFrequency(float64(cfg.LowCutHz))
		cr.lowCutR.SetFrequency(float64(cfg.LowCutHz))
	}
	if cfg.HighCutHz > 100.0 && cfg.HighCutHz < float32(cr.sampleRate*0.48) {
		cr.highCutL.SetFrequency(float64(cfg.HighCutHz))
		cr.highCutR.SetFrequency(float64(cfg.HighCutHz))
	}

	if len(cr.rawIRL) > 0 {
		cr.rebuildPartitions()
	}
}

func (cr *ConvolutionReverb) Config() ConvolutionConfig {
	return cr.cfg
}

func (cr *ConvolutionReverb) LoadIRFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return cr.LoadIR(f)
}

func (cr *ConvolutionReverb) LoadIR(r io.ReadSeeker) error {
	info, samples, err := wav.Decode(r)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return errors.New("empty wav impulse response")
	}

	channels := info.Channels
	if channels < 1 {
		channels = 1
	}
	frames := len(samples) / channels
	if frames == 0 {
		return errors.New("zero frames in impulse response")
	}

	irL := make([]float32, frames)
	irR := make([]float32, frames)

	if channels == 1 {
		for i := 0; i < frames; i++ {
			v := samples[i]
			irL[i] = v
			irR[i] = v
		}
	} else {
		for i := 0; i < frames; i++ {
			irL[i] = samples[channels*i]
			irR[i] = samples[channels*i+1]
		}
	}

	if info.SampleRate > 0 && float64(info.SampleRate) != cr.sampleRate {
		irL = resampleLinear(irL, float64(info.SampleRate), cr.sampleRate)
		irR = resampleLinear(irR, float64(info.SampleRate), cr.sampleRate)
	}

	cr.SetImpulseResponse(irL, irR)
	return nil
}

func (cr *ConvolutionReverb) SetImpulseResponse(irL, irR []float32) {
	cr.rawIRL = make([]float32, len(irL))
	copy(cr.rawIRL, irL)
	cr.rawIRR = make([]float32, len(irR))
	copy(cr.rawIRR, irR)
	cr.rebuildPartitions()
	cr.Reset()
}

func (cr *ConvolutionReverb) rebuildPartitions() {
	irLen := len(cr.rawIRL)
	if len(cr.rawIRR) > irLen {
		irLen = len(cr.rawIRR)
	}
	if irLen == 0 {
		cr.partitions = 0
		return
	}

	pCount := (irLen + ConvPartitionSize - 1) / ConvPartitionSize
	if pCount > MaxConvPartitions {
		pCount = MaxConvPartitions
	}
	if pCount < 1 {
		pCount = 1
	}

	cr.partitions = pCount
	if len(cr.hl) < pCount {
		cr.hl = make([][]complex128, pCount)
		cr.hr = make([][]complex128, pCount)
		cr.histL = make([][]complex128, pCount)
		cr.histR = make([][]complex128, pCount)
		for p := 0; p < pCount; p++ {
			cr.hl[p] = make([]complex128, ConvFFTSize)
			cr.hr[p] = make([]complex128, ConvFFTSize)
			cr.histL[p] = make([]complex128, ConvFFTSize)
			cr.histR[p] = make([]complex128, ConvFFTSize)
		}
	}

	decay := float64(cr.cfg.DecayRatio)
	totalSamples := float64(pCount * ConvPartitionSize)
	tempBuf := make([]complex128, ConvFFTSize)

	var energy float64
	for p := 0; p < pCount; p++ {
		start := p * ConvPartitionSize
		for i := 0; i < ConvFFTSize; i++ {
			tempBuf[i] = 0
		}
		for i := 0; i < ConvPartitionSize; i++ {
			idx := start + i
			var v float64
			if idx < len(cr.rawIRL) {
				v = float64(cr.rawIRL[idx])
			}
			if decay != 1.0 {
				v *= math.Exp(-3.0 * float64(idx) / (totalSamples * decay))
			}
			tempBuf[i] = complex(v, 0)
			energy += v * v
		}
		FFT(tempBuf)
		copy(cr.hl[p], tempBuf)

		for i := 0; i < ConvFFTSize; i++ {
			tempBuf[i] = 0
		}
		for i := 0; i < ConvPartitionSize; i++ {
			idx := start + i
			var v float64
			if idx < len(cr.rawIRR) {
				v = float64(cr.rawIRR[idx])
			}
			if decay != 1.0 {
				v *= math.Exp(-3.0 * float64(idx) / (totalSamples * decay))
			}
			tempBuf[i] = complex(v, 0)
		}
		FFT(tempBuf)
		copy(cr.hr[p], tempBuf)
	}

	norm := 1.0
	if energy > 1e-12 {
		rms := math.Sqrt(energy / totalSamples)
		if rms > 0.3 {
			norm = 0.3 / rms
		}
	}
	if norm != 1.0 {
		for p := 0; p < pCount; p++ {
			for k := 0; k < ConvFFTSize; k++ {
				cr.hl[p][k] *= complex(norm, 0)
				cr.hr[p][k] *= complex(norm, 0)
			}
		}
	}
}

func (cr *ConvolutionReverb) Reset() {
	cr.histHead = 0
	for p := 0; p < cr.partitions; p++ {
		clear(cr.histL[p])
		clear(cr.histR[p])
	}
	clear(cr.overlapL)
	clear(cr.overlapR)
	clear(cr.inBufL[:])
	clear(cr.inBufR[:])
	clear(cr.outBufL[:])
	clear(cr.outBufR[:])
	clear(cr.preBufL)
	clear(cr.preBufR)
	cr.inPos = 0
	cr.outPos = 0
	cr.hasBlock = false
	cr.preWrite = 0
	cr.lowCutL.Reset()
	cr.lowCutR.Reset()
	cr.highCutL.Reset()
	cr.highCutR.Reset()
}

func (cr *ConvolutionReverb) processSubBlock() {
	_ = cr.xBufL[ConvFFTSize-1]
	_ = cr.xBufR[ConvFFTSize-1]
	_ = cr.inBufL[ConvPartitionSize-1]
	_ = cr.inBufR[ConvPartitionSize-1]

	for i := 0; i < ConvPartitionSize; i++ {
		cr.xBufL[i] = complex(float64(cr.inBufL[i]), 0)
		cr.xBufR[i] = complex(float64(cr.inBufR[i]), 0)
		cr.xBufL[i+ConvPartitionSize] = 0
		cr.xBufR[i+ConvPartitionSize] = 0
	}

	FFT(cr.xBufL)
	FFT(cr.xBufR)

	head := cr.histHead
	copy(cr.histL[head], cr.xBufL)
	copy(cr.histR[head], cr.xBufR)

	clear(cr.accumL)
	clear(cr.accumR)

	P := cr.partitions
	for p := 0; p < P; p++ {
		idx := (head - p + P) % P
		curXL := cr.histL[idx]
		curXR := cr.histR[idx]
		curHL := cr.hl[p]
		curHR := cr.hr[p]
		for k := 0; k < ConvFFTSize; k++ {
			cr.accumL[k] += curXL[k] * curHL[k]
			cr.accumR[k] += curXR[k] * curHR[k]
		}
	}

	IFFT(cr.accumL)
	IFFT(cr.accumR)

	_ = cr.outBufL[ConvPartitionSize-1]
	_ = cr.outBufR[ConvPartitionSize-1]
	_ = cr.overlapL[ConvPartitionSize-1]
	_ = cr.overlapR[ConvPartitionSize-1]

	for i := 0; i < ConvPartitionSize; i++ {
		rL := float32(real(cr.accumL[i])) + cr.overlapL[i]
		rR := float32(real(cr.accumR[i])) + cr.overlapR[i]

		if math.IsNaN(float64(rL)) || math.IsInf(float64(rL), 0) {
			rL = 0
		}
		if math.IsNaN(float64(rR)) || math.IsInf(float64(rR), 0) {
			rR = 0
		}
		if rL > -1e-25 && rL < 1e-25 {
			rL = 0
		}
		if rR > -1e-25 && rR < 1e-25 {
			rR = 0
		}

		cr.outBufL[i] = rL
		cr.outBufR[i] = rR

		cr.overlapL[i] = float32(real(cr.accumL[i+ConvPartitionSize]))
		cr.overlapR[i] = float32(real(cr.accumR[i+ConvPartitionSize]))
	}

	cr.histHead = (head + 1) % P
	cr.inPos = 0
	cr.outPos = 0
	cr.hasBlock = true
}

func (cr *ConvolutionReverb) ProcessFrameWet(inL, inR float32) (float32, float32) {
	if cr.partitions == 0 {
		return 0, 0
	}

	if math.IsNaN(float64(inL)) || math.IsInf(float64(inL), 0) {
		inL = 0
	}
	if math.IsNaN(float64(inR)) || math.IsInf(float64(inR), 0) {
		inR = 0
	}

	cr.inBufL[cr.inPos] = inL
	cr.inBufR[cr.inPos] = inR
	cr.inPos++

	var rawWetL, rawWetR float32
	if cr.hasBlock && cr.outPos < ConvPartitionSize {
		rawWetL = cr.outBufL[cr.outPos]
		rawWetR = cr.outBufR[cr.outPos]
		cr.outPos++
	}

	if cr.inPos == ConvPartitionSize {
		cr.processSubBlock()
	}

	wL := rawWetL
	wR := rawWetR

	if cr.preDelay > 0 {
		w := cr.preWrite & cr.preMask
		r := (cr.preWrite - cr.preDelay) & cr.preMask
		cr.preBufL[w] = wL
		cr.preBufR[w] = wR
		wL = cr.preBufL[r]
		wR = cr.preBufR[r]
		cr.preWrite = (w + 1) & cr.preMask
	}

	wL = cr.lowCutL.ProcessSample(cr.highCutL.ProcessSample(wL))
	wR = cr.lowCutR.ProcessSample(cr.highCutR.ProcessSample(wR))

	if math.IsNaN(float64(wL)) || math.IsInf(float64(wL), 0) {
		wL = 0
	}
	if math.IsNaN(float64(wR)) || math.IsInf(float64(wR), 0) {
		wR = 0
	}

	return wL, wR
}

func (cr *ConvolutionReverb) ProcessFrame(inL, inR float32) (float32, float32) {
	wetL, wetR := cr.ProcessFrameWet(inL, inR)
	wet := cr.cfg.DryWet
	dry := 1.0 - wet
	return inL*dry + wetL*wet, inR*dry + wetR*wet
}

func (cr *ConvolutionReverb) Process(inL, inR, outL, outR []float32) {
	n := len(inL)
	if len(inR) < n {
		n = len(inR)
	}
	if len(outL) < n {
		n = len(outL)
	}
	if len(outR) < n {
		n = len(outR)
	}
	if n == 0 {
		return
	}

	_ = inL[n-1]
	_ = inR[n-1]
	_ = outL[n-1]
	_ = outR[n-1]

	for i := 0; i < n; i++ {
		oL, oR := cr.ProcessFrame(inL[i], inR[i])
		outL[i] = oL
		outR[i] = oR
	}
}

func (cr *ConvolutionReverb) ProcessInterleaved(in, out []float32) {
	frames := len(in) / 2
	outFrames := len(out) / 2
	if outFrames < frames {
		frames = outFrames
	}
	if frames == 0 {
		return
	}

	_ = in[2*frames-1]
	_ = out[2*frames-1]

	for i := 0; i < frames; i++ {
		oL, oR := cr.ProcessFrame(in[2*i], in[2*i+1])
		out[2*i] = oL
		out[2*i+1] = oR
	}
}

func resampleLinear(src []float32, srcRate, dstRate float64) []float32 {
	if len(src) == 0 {
		return nil
	}
	ratio := srcRate / dstRate
	outLen := int(math.Round(float64(len(src)) / ratio))
	if outLen < 1 {
		outLen = 1
	}
	dst := make([]float32, outLen)
	maxSrcIdx := len(src) - 1
	for i := 0; i < outLen; i++ {
		pos := float64(i) * ratio
		idx := int(pos)
		frac := float32(pos - float64(idx))
		if idx >= maxSrcIdx {
			dst[i] = src[maxSrcIdx]
		} else {
			dst[i] = src[idx]*(1.0-frac) + src[idx+1]*frac
		}
	}
	return dst
}
