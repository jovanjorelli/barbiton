package dsp

import (
	"math"
)

func NextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func bitReverse(x []complex128) {
	n := len(x)
	j := 0
	for i := 0; i < n-1; i++ {
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
		k := n / 2
		for k > 0 && k <= j {
			j -= k
			k /= 2
		}
		j += k
	}
}

func FFT(x []complex128) {
	n := len(x)
	if n <= 1 || (n&(n-1)) != 0 {
		return
	}
	bitReverse(x)
	for length := 2; length <= n; length <<= 1 {
		half := length / 2
		angle := -2.0 * math.Pi / float64(length)
		wStep := complex(math.Cos(angle), math.Sin(angle))
		for i := 0; i < n; i += length {
			w := complex(1.0, 0.0)
			for j := 0; j < half; j++ {
				u := x[i+j]
				v := x[i+j+half] * w
				x[i+j] = u + v
				x[i+j+half] = u - v
				w *= wStep
			}
		}
	}
}

func IFFT(x []complex128) {
	n := len(x)
	if n <= 1 || (n&(n-1)) != 0 {
		return
	}
	for i := range x {
		x[i] = complex(real(x[i]), -imag(x[i]))
	}
	FFT(x)
	factor := 1.0 / float64(n)
	for i := range x {
		x[i] = complex(real(x[i])*factor, -imag(x[i])*factor)
	}
}

func HannWindow(n int) []float64 {
	w := make([]float64, n)
	if n == 1 {
		w[0] = 1.0
		return w
	}
	for i := 0; i < n; i++ {
		w[i] = 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}
