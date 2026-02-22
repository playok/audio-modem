package modem

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestFFT_IFFT_RoundTrip(t *testing.T) {
	// Test that IFFT(FFT(x)) == x
	n := 512
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(float64(i)/float64(n), 0)
	}

	y := FFT(x)
	z := IFFT(y)

	for i := range x {
		if cmplx.Abs(x[i]-z[i]) > 1e-10 {
			t.Errorf("IFFT(FFT(x))[%d] = %v, want %v", i, z[i], x[i])
		}
	}
}

func TestFFT_KnownValues(t *testing.T) {
	// FFT of [1, 1, 1, 1] should be [4, 0, 0, 0]
	x := []complex128{1, 1, 1, 1}
	y := FFT(x)

	if cmplx.Abs(y[0]-4) > 1e-10 {
		t.Errorf("FFT([1,1,1,1])[0] = %v, want 4", y[0])
	}
	for i := 1; i < 4; i++ {
		if cmplx.Abs(y[i]) > 1e-10 {
			t.Errorf("FFT([1,1,1,1])[%d] = %v, want 0", i, y[i])
		}
	}
}

func TestFFT_Parseval(t *testing.T) {
	// Parseval's theorem: sum|x|^2 == sum|X|^2 / N
	n := 256
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(math.Sin(2*math.Pi*float64(i)/float64(n)), 0)
	}

	y := FFT(x)

	var sumX, sumY float64
	for i := range x {
		sumX += real(x[i])*real(x[i]) + imag(x[i])*imag(x[i])
		sumY += real(y[i])*real(y[i]) + imag(y[i])*imag(y[i])
	}
	sumY /= float64(n)

	if math.Abs(sumX-sumY) > 1e-6 {
		t.Errorf("Parseval's theorem violated: sumX=%v, sumY/N=%v", sumX, sumY)
	}
}

func TestRealFFT_Sine(t *testing.T) {
	n := 512
	freq := 10.0 // 10 cycles
	x := make([]float64, n)
	for i := range x {
		x[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(n))
	}

	y := RealFFT(x)

	// Peak should be at index 10 (and 512-10)
	maxMag := 0.0
	maxIdx := 0
	for i := 1; i < n/2; i++ {
		mag := cmplx.Abs(y[i])
		if mag > maxMag {
			maxMag = mag
			maxIdx = i
		}
	}

	if maxIdx != int(freq) {
		t.Errorf("Peak at index %d, expected %d", maxIdx, int(freq))
	}
}
