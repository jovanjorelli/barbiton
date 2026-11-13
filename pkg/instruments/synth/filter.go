package synth

import "math"

type FilterType int

const (
	Filter1Pole FilterType = iota
	Filter12dBSVF
	Filter24dBCascade
	Filter24dBLadder
	FilterSVF12    = Filter12dBSVF
	FilterSVF24    = Filter24dBCascade
	FilterLadder24 = Filter24dBLadder
	Filter24dB     = Filter24dBCascade
)

func filterParam(v float32) FilterType {
	if math.IsNaN(float64(v)) {
		return Filter1Pole
	}
	switch int(math.Round(float64(v))) {
	case 1:
		return Filter12dBSVF
	case 2:
		return Filter24dBCascade
	case 3:
		return Filter24dBLadder
	default:
		return Filter1Pole
	}
}

type svfCoeffs struct {
	g float64
	k float64
	d float64
}

func calcSVFCoeffs(fc, res, sampleRate float64) svfCoeffs {
	w := math.Pi * fc / sampleRate
	if math.IsNaN(w) || w < 1e-5 {
		w = 1e-5
	} else if w > math.Pi*0.49 {
		w = math.Pi * 0.49
	}
	g := math.Tan(w)
	r := res
	if math.IsNaN(r) || r < 0 {
		r = 0
	} else if r > 1 {
		r = 1
	}
	k := math.Sqrt(2.0)*(1.0-r) + 0.05*r
	d := 1.0 + k*g + g*g
	return svfCoeffs{g: g, k: k, d: d}
}

func processSVF12(x float64, c svfCoeffs, s1, s2 *float64) float64 {
	hp := (x - (c.k+c.g)*(*s1) - *s2) / c.d
	v1 := c.g * hp
	bp := v1 + *s1
	bpSat := math.Tanh(bp)
	*s1 = v1 + bpSat
	v2 := c.g * bpSat
	lp := v2 + *s2
	*s2 = v2 + lp
	return lp
}

func processSVF24(x float64, c svfCoeffs, s1, s2, s3, s4 *float64) float64 {
	lp1 := processSVF12(x, c, s1, s2)
	return processSVF12(lp1, c, s3, s4)
}

type ladderCoeffs struct {
	G         float64
	onePlusG  float64
	resAmount float64
	denom     float64
}

func calcLadderCoeffs(fc, res, sampleRate float64) ladderCoeffs {
	w := math.Pi * fc / sampleRate
	if math.IsNaN(w) || w < 1e-5 {
		w = 1e-5
	} else if w > math.Pi*0.49 {
		w = math.Pi * 0.49
	}
	g := math.Tan(w)
	onePlusG := 1.0 + g
	G := g / onePlusG
	r := res
	if math.IsNaN(r) || r < 0 {
		r = 0
	} else if r > 1 {
		r = 1
	}
	resAmount := 4.0 * r * 0.99
	g4 := G * G * G * G
	denom := 1.0 + resAmount*g4
	return ladderCoeffs{G: G, onePlusG: onePlusG, resAmount: resAmount, denom: denom}
}

func processLadder24(x float64, c ladderCoeffs, s1, s2, s3, s4 *float64) float64 {
	S1 := *s1 / c.onePlusG
	S2 := *s2 / c.onePlusG
	S3 := *s3 / c.onePlusG
	S4 := *s4 / c.onePlusG
	G := c.G
	G2 := G * G
	G3 := G2 * G
	G4 := G3 * G
	y4Est := (G4*x + G3*S1 + G2*S2 + G*S3 + S4) / c.denom
	u := math.Tanh(x - c.resAmount*y4Est)
	v1 := (u - *s1) * G
	y1 := v1 + *s1
	*s1 = y1 + v1
	v2 := (y1 - *s2) * G
	y2 := v2 + *s2
	*s2 = y2 + v2
	v3 := (y2 - *s3) * G
	y3 := v3 + *s3
	*s3 = y3 + v3
	v4 := (y3 - *s4) * G
	y4 := v4 + *s4
	*s4 = y4 + v4
	return y4
}
