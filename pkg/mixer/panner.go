package mixer

import "math"

func PanGains(pan float32) (left, right float32) {
	p := clampF(float64(pan), -1, 1)
	left = float32(math.Sqrt((1 - p) / 2))
	right = float32(math.Sqrt((1 + p) / 2))
	return left, right
}
