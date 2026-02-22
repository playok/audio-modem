package modem

import (
	"math"
	"math/cmplx"
)

// FFT computes the Discrete Fourier Transform using Cooley-Tukey radix-2.
// Input length must be a power of 2.
func FFT(x []complex128) []complex128 {
	n := len(x)
	if n <= 1 {
		out := make([]complex128, n)
		copy(out, x)
		return out
	}
	if n&(n-1) != 0 {
		panic("FFT: length must be a power of 2")
	}

	out := make([]complex128, n)
	copy(out, x)
	bitReverse(out)
	fftIterative(out, false)
	return out
}

// IFFT computes the Inverse Discrete Fourier Transform.
func IFFT(x []complex128) []complex128 {
	n := len(x)
	if n <= 1 {
		out := make([]complex128, n)
		copy(out, x)
		return out
	}

	out := make([]complex128, n)
	copy(out, x)
	bitReverse(out)
	fftIterative(out, true)

	// Scale by 1/N
	scale := 1.0 / float64(n)
	for i := range out {
		out[i] *= complex(scale, 0)
	}
	return out
}

func fftIterative(x []complex128, inverse bool) {
	n := len(x)
	for size := 2; size <= n; size <<= 1 {
		halfSize := size >> 1
		sign := -1.0
		if inverse {
			sign = 1.0
		}
		wn := cmplx.Exp(complex(0, sign*2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for j := 0; j < halfSize; j++ {
				u := x[start+j]
				v := w * x[start+j+halfSize]
				x[start+j] = u + v
				x[start+j+halfSize] = u - v
				w *= wn
			}
		}
	}
}

func bitReverse(x []complex128) {
	n := len(x)
	bits := 0
	for tmp := n; tmp > 1; tmp >>= 1 {
		bits++
	}
	for i := 0; i < n; i++ {
		j := reverseBits(i, bits)
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
}

func reverseBits(x, bits int) int {
	result := 0
	for i := 0; i < bits; i++ {
		result = (result << 1) | (x & 1)
		x >>= 1
	}
	return result
}

// RealFFT performs FFT on real-valued input.
func RealFFT(x []float64) []complex128 {
	n := len(x)
	cx := make([]complex128, n)
	for i, v := range x {
		cx[i] = complex(v, 0)
	}
	return FFT(cx)
}

// RealIFFT performs IFFT and returns only the real part.
func RealIFFT(x []complex128) []float64 {
	result := IFFT(x)
	out := make([]float64, len(result))
	for i, v := range result {
		out[i] = real(v)
	}
	return out
}
