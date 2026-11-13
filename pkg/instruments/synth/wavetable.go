package synth

import (
	"math"
	"strings"
)

const (
	WavetableFrameSize = 2048
	NumWavetableFrames = 16
	NumReferenceTables = 4
)

type Wavetable [NumWavetableFrames][WavetableFrameSize]float32

var (
	wavetableNames = [NumReferenceTables]string{
		"saw_to_square",
		"metallic_growl",
		"vocal_formant_talk",
		"aggressive_modern",
	}
	referenceTables [NumReferenceTables]Wavetable
)

func init() {
	generateSawToSquare(&referenceTables[0])
	generateMetallicGrowl(&referenceTables[1])
	generateVocalFormantTalk(&referenceTables[2])
	generateAggressiveModern(&referenceTables[3])
}

func WavetableNames() []string {
	names := make([]string, NumReferenceTables)
	copy(names, wavetableNames[:])
	return names
}

func WavetableIndex(name string) int {
	norm := strings.ToLower(strings.TrimSpace(name))
	for i, n := range wavetableNames {
		if norm == n {
			return i
		}
	}
	return 0
}

func ReferenceWavetable(index int) *Wavetable {
	if index < 0 || index >= NumReferenceTables {
		index = 0
	}
	return &referenceTables[index]
}

func GetWavetable(name string) *Wavetable {
	return ReferenceWavetable(WavetableIndex(name))
}

func hermiteCubic(y0, y1, y2, y3, frac float64) float64 {
	c0 := y1
	c1 := 0.5 * (y2 - y0)
	c2 := y0 - 2.5*y1 + 2.0*y2 - 0.5*y3
	c3 := 0.5*(y3-y0) + 1.5*(y1-y2)
	return ((c3*frac+c2)*frac+c1)*frac + c0
}

func SampleWavetableHermite(wt *Wavetable, pos, phase float64) float64 {
	if wt == nil {
		return 0
	}

	phi := phase - math.Floor(phase)
	if phi < 0 {
		phi += 1.0
	}

	samplePos := phi * float64(WavetableFrameSize)
	sIdx := int(samplePos)
	sFrac := samplePos - float64(sIdx)
	m := WavetableFrameSize - 1

	i0 := (sIdx - 1) & m
	i1 := sIdx & m
	i2 := (sIdx + 1) & m
	i3 := (sIdx + 2) & m

	p := pos
	if p < 0 {
		p = 0
	} else if p > 1 {
		p = 1
	}

	framePos := p * float64(NumWavetableFrames-1)
	fIdx := int(framePos)
	fFrac := framePos - float64(fIdx)
	if fIdx >= NumWavetableFrames-1 {
		fIdx = NumWavetableFrames - 2
		fFrac = 1.0
	}

	k0 := fIdx - 1
	if k0 < 0 {
		k0 = 0
	}
	k1 := fIdx
	k2 := fIdx + 1
	if k2 >= NumWavetableFrames {
		k2 = NumWavetableFrames - 1
	}
	k3 := fIdx + 2
	if k3 >= NumWavetableFrames {
		k3 = NumWavetableFrames - 1
	}

	y0 := hermiteCubic(float64(wt[k0][i0]), float64(wt[k0][i1]), float64(wt[k0][i2]), float64(wt[k0][i3]), sFrac)
	y1 := hermiteCubic(float64(wt[k1][i0]), float64(wt[k1][i1]), float64(wt[k1][i2]), float64(wt[k1][i3]), sFrac)
	y2 := hermiteCubic(float64(wt[k2][i0]), float64(wt[k2][i1]), float64(wt[k2][i2]), float64(wt[k2][i3]), sFrac)
	y3 := hermiteCubic(float64(wt[k3][i0]), float64(wt[k3][i1]), float64(wt[k3][i2]), float64(wt[k3][i3]), sFrac)

	return hermiteCubic(y0, y1, y2, y3, fFrac)
}

func normalizeFrame(frame *[WavetableFrameSize]float32) {
	var maxVal float32
	for i := 0; i < WavetableFrameSize; i++ {
		v := frame[i]
		if v < 0 {
			v = -v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal > 1e-6 {
		scale := float32(0.95) / maxVal
		for i := 0; i < WavetableFrameSize; i++ {
			frame[i] *= scale
		}
	}
}

func generateSawToSquare(wt *Wavetable) {
	for k := 0; k < NumWavetableFrames; k++ {
		alpha := float64(k) / float64(NumWavetableFrames-1)
		for i := 0; i < WavetableFrameSize; i++ {
			theta := 2.0 * math.Pi * float64(i) / float64(WavetableFrameSize)
			var saw, sq float64
			for h := 1; h <= 48; h++ {
				fh := float64(h)
				sign := 1.0
				if h%2 == 0 {
					sign = -1.0
				}
				saw += (sign / fh) * math.Sin(fh*theta)
			}
			saw *= 2.0 / math.Pi

			for m := 0; m < 24; m++ {
				fm := float64(2*m + 1)
				sq += (1.0 / fm) * math.Sin(fm*theta)
			}
			sq *= 4.0 / math.Pi

			wt[k][i] = float32((1.0-alpha)*saw + alpha*sq)
		}
		normalizeFrame(&wt[k])
	}
}

func generateMetallicGrowl(wt *Wavetable) {
	for k := 0; k < NumWavetableFrames; k++ {
		alpha := float64(k) / float64(NumWavetableFrames-1)
		peakHarmonic := 18.0 - 15.0*alpha
		for i := 0; i < WavetableFrameSize; i++ {
			theta := 2.0 * math.Pi * float64(i) / float64(WavetableFrameSize)
			var sum float64
			for h := 1; h <= 48; h++ {
				fh := float64(h)
				diff := fh - peakHarmonic
				bell := math.Exp(-(diff * diff) / 10.0)
				lowWeight := (1.0 / math.Pow(fh, 0.7)) * alpha * 1.5
				amp := bell + lowWeight + 0.15/(1.0+0.1*fh)
				phaseShift := fh * (1.0 - alpha) * 0.4
				sum += amp * math.Sin(fh*theta+phaseShift)
			}
			wt[k][i] = float32(sum)
		}
		normalizeFrame(&wt[k])
	}
}

func generateVocalFormantTalk(wt *Wavetable) {
	vowelF1 := [5]float64{6.0, 4.0, 2.0, 5.0, 2.5}
	vowelF2 := [5]float64{9.0, 15.0, 19.0, 7.0, 7.0}
	vowelF3 := [5]float64{20.0, 20.0, 25.0, 20.0, 18.0}

	for k := 0; k < NumWavetableFrames; k++ {
		alpha := float64(k) / float64(NumWavetableFrames-1)
		scaled := alpha * 4.0
		idx := int(scaled)
		frac := scaled - float64(idx)
		if idx >= 4 {
			idx = 3
			frac = 1.0
		}

		f1 := (1.0-frac)*vowelF1[idx] + frac*vowelF1[idx+1]
		f2 := (1.0-frac)*vowelF2[idx] + frac*vowelF2[idx+1]
		f3 := (1.0-frac)*vowelF3[idx] + frac*vowelF3[idx+1]

		for i := 0; i < WavetableFrameSize; i++ {
			theta := 2.0 * math.Pi * float64(i) / float64(WavetableFrameSize)
			var sum float64
			for h := 1; h <= 48; h++ {
				fh := float64(h)
				d1 := fh - f1
				d2 := fh - f2
				d3 := fh - f3
				amp1 := 1.0 / (1.0 + 3.0*d1*d1)
				amp2 := 0.7 / (1.0 + 2.0*d2*d2)
				amp3 := 0.4 / (1.0 + 2.0*d3*d3)
				totalAmp := amp1 + amp2 + amp3
				sum += totalAmp * math.Sin(fh*theta)
			}
			wt[k][i] = float32(sum)
		}
		normalizeFrame(&wt[k])
	}
}

func generateAggressiveModern(wt *Wavetable) {
	for k := 0; k < NumWavetableFrames; k++ {
		alpha := float64(k) / float64(NumWavetableFrames-1)
		for i := 0; i < WavetableFrameSize; i++ {
			theta := 2.0 * math.Pi * float64(i) / float64(WavetableFrameSize)
			var sum float64
			for h := 1; h <= 48; h++ {
				fh := float64(h)
				c := (1.0 / math.Sqrt(fh)) * math.Cos(fh*alpha*math.Pi/6.0)
				d := math.Exp(-fh/16.0) * math.Sin(fh*(0.5+alpha))
				sum += (c + 0.8*d) * math.Sin(fh*theta)
			}
			shaped := math.Tanh(sum * (1.2 + alpha*1.8))
			wt[k][i] = float32(shaped)
		}
		normalizeFrame(&wt[k])
	}
}
