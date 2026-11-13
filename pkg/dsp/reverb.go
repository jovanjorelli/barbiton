package dsp

import (
	"math"
	"math/bits"
	"strings"
)

type Reverb struct {
	sampleRate float64
	params     ReverbParams

	lines    [8]fdnLine
	matrix   [8][8]float32
	feedback float32
	inGain   float32
	outScale float32

	allL [4]reverbAllpass
	allR [4]reverbAllpass

	preL, preR fdnLine

	widthCoef float32
}

var fdnBaseLengths = [8]int{1559, 1619, 1489, 1427, 1277, 1361, 1187, 1117}

var allpassBaseLengths = [4]int{605, 480, 371, 245}

const (
	fdnStereoSpread    = 23
	allpassGain        = 0.5
	maxPredelaySeconds = 0.1
	reverbInGain       = 0.30
	reverbOutScale     = 0.45
	maxRoomStretch     = 1.15
)

type ReverbParams struct {
	RoomSize float32
	Damping  float32
	Wet      float32
	Dry      float32
	Width    float32
	PreDelay float32
}

func PresetParams(name string) (ReverbParams, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cathedral":
		return ReverbParams{RoomSize: 0.95, Damping: 0.22, Wet: 0.45, Dry: 0.55, Width: 0.95, PreDelay: 0.06}, true
	case "hall":
		return ReverbParams{RoomSize: 0.85, Damping: 0.30, Wet: 0.40, Dry: 0.60, Width: 0.90, PreDelay: 0.035}, true
	case "chapel":
		return ReverbParams{RoomSize: 0.78, Damping: 0.26, Wet: 0.40, Dry: 0.60, Width: 0.85, PreDelay: 0.025}, true
	case "room":
		return ReverbParams{RoomSize: 0.55, Damping: 0.40, Wet: 0.35, Dry: 0.65, Width: 0.75, PreDelay: 0.008}, true
	case "plate":
		return ReverbParams{RoomSize: 0.72, Damping: 0.55, Wet: 0.45, Dry: 0.55, Width: 1.0, PreDelay: 0.003}, true
	}
	return ReverbParams{}, false
}

type fdnLine struct {
	buf    []float32
	mask   int
	idx    int
	length int
	dampY  float32
}

func newFDNLine(maxLen int) fdnLine {
	if maxLen < 1 {
		maxLen = 1
	}
	capacity := 1
	for capacity <= maxLen {
		capacity <<= 1
	}
	return fdnLine{buf: make([]float32, capacity), mask: capacity - 1}
}

func (l *fdnLine) read() float32 {
	return l.buf[(l.idx-l.length)&l.mask]
}

func (l *fdnLine) write(v float32) {
	l.buf[l.idx] = v
	l.idx = (l.idx + 1) & l.mask
}

func (l *fdnLine) reset() {
	clear(l.buf)
	l.idx = 0
	l.dampY = 0
}

type reverbAllpass struct {
	line  fdnLine
	gain  float32
	gain2 float32
}

func (a *reverbAllpass) process(x float32) float32 {
	if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
		x = 0
	}
	w := a.line.read()
	if math.IsNaN(float64(w)) || math.IsInf(float64(w), 0) {
		w = 0
	}
	y := w - a.gain*x
	toWrite := a.gain2*x + a.gain*w
	if math.IsNaN(float64(toWrite)) || math.IsInf(float64(toWrite), 0) {
		toWrite = 0
	}
	a.line.write(toWrite)
	if math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) {
		y = 0
	}
	return y
}

func NewReverb(sampleRate float64, params ReverbParams) *Reverb {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	rateScale := sampleRate / 48000
	r := &Reverb{
		sampleRate: sampleRate,
		inGain:     reverbInGain,
		outScale:   reverbOutScale,
	}
	spread := int(math.Ceil(float64(fdnStereoSpread) * rateScale))
	for i, base := range fdnBaseLengths {
		maxLen := int(math.Ceil(float64(base)*maxRoomStretch*rateScale)) + spread
		r.lines[i] = newFDNLine(maxLen)
	}
	for i, base := range allpassBaseLengths {
		r.allL[i] = reverbAllpass{
			line:  newFDNLine(int(math.Ceil(float64(base) * rateScale))),
			gain:  allpassGain,
			gain2: 1 - allpassGain*allpassGain,
		}
		r.allR[i] = reverbAllpass{
			line:  newFDNLine(int(math.Ceil(float64(base)*rateScale)) + spread),
			gain:  allpassGain,
			gain2: 1 - allpassGain*allpassGain,
		}
	}
	preMax := int(math.Ceil(maxPredelaySeconds * sampleRate))
	r.preL = newFDNLine(preMax)
	r.preR = newFDNLine(preMax)

	const invSqrt8 = 0.35355339059327376
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			if bits.OnesCount(uint(i&j))%2 == 1 {
				r.matrix[i][j] = -invSqrt8
			} else {
				r.matrix[i][j] = invSqrt8
			}
		}
	}

	r.SetParams(params)
	return r
}

func (r *Reverb) SetParams(p ReverbParams) {
	p.RoomSize = ClampF32(p.RoomSize, 0, 1)
	p.Damping = ClampF32(p.Damping, 0, 0.95)
	p.Wet = ClampF32(p.Wet, 0, 1)
	p.Dry = ClampF32(p.Dry, 0, 1)
	p.Width = ClampF32(p.Width, 0, 1)
	p.PreDelay = ClampF32(p.PreDelay, 0, maxPredelaySeconds)
	r.params = p

	rateScale := r.sampleRate / 48000
	stretch := float64(0.85 + 0.3*p.RoomSize)
	spread := int(math.Round(float64(fdnStereoSpread) * rateScale))
	for i, base := range fdnBaseLengths {
		n := int(math.Round(float64(base) * stretch * rateScale))
		if n < 1 {
			n = 1
		}
		if i >= 4 {
			n += spread
		}
		r.lines[i].length = n
	}
	for i, base := range allpassBaseLengths {
		n := int(math.Round(float64(base) * rateScale))
		if n < 1 {
			n = 1
		}
		r.allL[i].line.length = n
		r.allR[i].line.length = n + spread
	}
	preLen := int(math.Round(float64(p.PreDelay) * r.sampleRate))
	r.preL.length = preLen
	r.preR.length = preLen

	r.feedback = ClampF32(0.55+0.42*p.RoomSize, 0, 0.97)
	r.widthCoef = p.Width
}

func (r *Reverb) Params() ReverbParams { return r.params }

func (r *Reverb) SetDamping(d float32) {
	r.params.Damping = ClampF32(d, 0, 0.95)
}

func (r *Reverb) RT60Seconds() float64 {
	if r.feedback <= 0 || r.feedback >= 1 {
		return 0
	}
	mean := 0
	for i := range r.lines {
		mean += r.lines[i].length
	}
	tLoop := float64(mean) / 8 / r.sampleRate
	return 3 * tLoop / (-math.Log10(float64(r.feedback)))
}

func (r *Reverb) Reset() {
	for i := range r.lines {
		r.lines[i].reset()
	}
	for i := range r.allL {
		r.allL[i].line.reset()
		r.allR[i].line.reset()
	}
	r.preL.reset()
	r.preR.reset()
}

func (r *Reverb) ProcessFrameWet(inL, inR float32) (float32, float32) {
	if math.IsNaN(float64(inL)) || math.IsInf(float64(inL), 0) {
		inL = 0
	}
	if math.IsNaN(float64(inR)) || math.IsInf(float64(inR), 0) {
		inR = 0
	}
	xL, xR := inL, inR
	if r.preL.length > 0 {
		xL = r.preL.read()
		r.preL.write(inL)
		xR = r.preR.read()
		r.preR.write(inR)
	}

	var y, d [8]float32
	damp := r.dampCoef()
	for i := range r.lines {
		v := r.lines[i].read()
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			v = 0
		}
		y[i] = v
		ln := &r.lines[i]
		newDamp := y[i]*(1-damp) + ln.dampY*damp
		if math.IsNaN(float64(newDamp)) || math.IsInf(float64(newDamp), 0) || math.Abs(float64(newDamp)) < 1e-25 {
			newDamp = 0
		}
		ln.dampY = newDamp
		d[i] = newDamp
	}

	for i := range r.lines {
		mix := float32(0)
		row := &r.matrix[i]
		for j := 0; j < 8; j++ {
			mix += row[j] * d[j]
		}
		x := xL
		if i >= 4 {
			x = xR
		}
		toWrite := mix*r.feedback + x*r.inGain
		if math.IsNaN(float64(toWrite)) || math.IsInf(float64(toWrite), 0) {
			toWrite = 0
		}
		r.lines[i].write(toWrite)
	}

	wetL := (y[0] + y[1] + y[2] + y[3]) * r.outScale
	wetR := (y[4] + y[5] + y[6] + y[7]) * r.outScale

	for i := range r.allL {
		wetL = r.allL[i].process(wetL)
		if math.IsNaN(float64(wetL)) || math.IsInf(float64(wetL), 0) {
			wetL = 0
		}
	}
	for i := range r.allR {
		wetR = r.allR[i].process(wetR)
		if math.IsNaN(float64(wetR)) || math.IsInf(float64(wetR), 0) {
			wetR = 0
		}
	}

	mono := (wetL + wetR) * 0.5
	w := r.widthCoef
	return mono + (wetL-mono)*w, mono + (wetR-mono)*w
}

func (r *Reverb) dampCoef() float32 { return r.params.Damping }

func (r *Reverb) ProcessFrame(inL, inR float32) (float32, float32) {
	wL, wR := r.ProcessFrameWet(inL, inR)
	return inL*r.params.Dry + wL*r.params.Wet,
		inR*r.params.Dry + wR*r.params.Wet
}

func (r *Reverb) ProcessStereo(buf *StereoBuffer) {
	if buf == nil {
		return
	}
	left, right := buf.Left, buf.Right
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		left[i], right[i] = r.ProcessFrame(left[i], right[i])
	}
}
