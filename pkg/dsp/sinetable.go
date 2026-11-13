package dsp

import "math"

const sineTableBits = 11
const sineTableSize = 1 << sineTableBits

var sineTable [sineTableSize + 1]float32

func init() {
	for i := 0; i <= sineTableSize; i++ {
		sineTable[i] = float32(math.Sin(twoPi * float64(i) / sineTableSize))
	}
}

func SineFast(phase float64) float32 {
	p := phase - math.Floor(phase)
	x := p * sineTableSize
	i := int(x)
	f := float32(x - float64(i))
	a := sineTable[i]
	return a + (sineTable[i+1]-a)*f
}
