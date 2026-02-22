package modem

import (
	"math"
	"math/cmplx"
	"math/rand"
)

// Schmidl-Cox preamble generation and detection for OFDM synchronization.

const (
	PreambleFFTSize = 512
	PreambleCPLen   = 64
	// Detection threshold for Schmidl-Cox metric (0 to 1)
	DetectionThreshold = 0.7
)

// PreambleGenerator generates Schmidl-Cox preambles.
type PreambleGenerator struct {
	fftSize int
	cpLen   int
	rng     *rand.Rand
}

// NewPreambleGenerator creates a new preamble generator.
func NewPreambleGenerator(seed int64) *PreambleGenerator {
	return &PreambleGenerator{
		fftSize: PreambleFFTSize,
		cpLen:   PreambleCPLen,
		rng:     rand.New(rand.NewSource(seed)),
	}
}

// GenerateSchmidlCox generates a Schmidl-Cox preamble consisting of two OFDM symbols.
// Symbol 1: Only even subcarriers carry PN sequence (time domain has two identical halves)
// Symbol 2: All subcarriers carry PN sequence (for fine frequency offset estimation)
func (pg *PreambleGenerator) GenerateSchmidlCox() (symbol1, symbol2 []float64) {
	// Symbol 1: Even subcarriers only → time-domain repetition
	spec1 := make([]complex128, pg.fftSize)
	pg.rng = rand.New(rand.NewSource(42)) // Fixed seed for reproducibility
	for k := SubcarrierStart; k <= SubcarrierEnd; k += 2 {
		// BPSK: +1 or -1
		if pg.rng.Intn(2) == 0 {
			spec1[k] = complex(1, 0)
		} else {
			spec1[k] = complex(-1, 0)
		}
	}
	// Hermitian symmetry for real output
	for k := 1; k < pg.fftSize/2; k++ {
		spec1[pg.fftSize-k] = cmplx.Conj(spec1[k])
	}
	spec1[0] = 0
	spec1[pg.fftSize/2] = 0

	td1 := RealIFFT(spec1)
	symbol1 = addCyclicPrefix(td1, pg.cpLen)
	normalizeAmplitude(symbol1)

	// Symbol 2: All subcarriers → unique pattern for fine estimation
	spec2 := make([]complex128, pg.fftSize)
	pg.rng = rand.New(rand.NewSource(43)) // Different seed
	for k := SubcarrierStart; k <= SubcarrierEnd; k++ {
		if pg.rng.Intn(2) == 0 {
			spec2[k] = complex(1, 0)
		} else {
			spec2[k] = complex(-1, 0)
		}
	}
	for k := 1; k < pg.fftSize/2; k++ {
		spec2[pg.fftSize-k] = cmplx.Conj(spec2[k])
	}
	spec2[0] = 0
	spec2[pg.fftSize/2] = 0

	td2 := RealIFFT(spec2)
	symbol2 = addCyclicPrefix(td2, pg.cpLen)
	normalizeAmplitude(symbol2)

	return
}

// GenerateChannelEstimation generates a known symbol for channel estimation.
// All data subcarriers carry known BPSK values.
func (pg *PreambleGenerator) GenerateChannelEstimation() ([]float64, []complex128) {
	spec := make([]complex128, pg.fftSize)
	knownSymbols := make([]complex128, pg.fftSize)

	pg.rng = rand.New(rand.NewSource(44))
	for k := SubcarrierStart; k <= SubcarrierEnd; k++ {
		if pg.rng.Intn(2) == 0 {
			spec[k] = complex(1, 0)
		} else {
			spec[k] = complex(-1, 0)
		}
		knownSymbols[k] = spec[k]
	}
	for k := 1; k < pg.fftSize/2; k++ {
		spec[pg.fftSize-k] = cmplx.Conj(spec[k])
	}
	spec[0] = 0
	spec[pg.fftSize/2] = 0

	td := RealIFFT(spec)
	samples := addCyclicPrefix(td, pg.cpLen)
	normalizeAmplitude(samples)

	return samples, knownSymbols
}

// PreambleDetector detects Schmidl-Cox preambles in the received signal.
type PreambleDetector struct {
	fftSize   int
	cpLen     int
	threshold float64
}

// NewPreambleDetector creates a new preamble detector.
func NewPreambleDetector() *PreambleDetector {
	return &PreambleDetector{
		fftSize:   PreambleFFTSize,
		cpLen:     PreambleCPLen,
		threshold: DetectionThreshold,
	}
}

// Detect finds the preamble start position in the received signal.
// Returns the sample index of the preamble start, or -1 if not found.
func (pd *PreambleDetector) Detect(signal []float64) int {
	halfLen := pd.fftSize / 2
	symbolLen := pd.fftSize + pd.cpLen

	if len(signal) < symbolLen+halfLen {
		return -1
	}

	bestMetric := 0.0
	bestIdx := -1

	// Sliding window Schmidl-Cox metric
	for d := 0; d < len(signal)-symbolLen; d++ {
		// P(d) = sum of x[d+m] * conj(x[d+m+N/2]) for m=0..N/2-1
		var pReal, pImag float64
		var rr float64

		for m := 0; m < halfLen; m++ {
			if d+m+halfLen >= len(signal) {
				break
			}
			a := signal[d+m]
			b := signal[d+m+halfLen]
			pReal += a * b
			pImag += 0 // real signals, imaginary is 0
			rr += b * b
		}

		pMag := pReal*pReal + pImag*pImag
		if rr > 0 {
			metric := pMag / (rr * rr)
			if metric > bestMetric {
				bestMetric = metric
				bestIdx = d
			}
		}
	}

	if bestMetric > pd.threshold {
		return bestIdx
	}
	return -1
}

// DetectWithMetrics returns the detection metric at each sample position.
// Useful for debugging and visualization.
func (pd *PreambleDetector) DetectWithMetrics(signal []float64) (int, []float64) {
	halfLen := pd.fftSize / 2
	symbolLen := pd.fftSize + pd.cpLen
	metricsLen := len(signal) - symbolLen
	if metricsLen <= 0 {
		return -1, nil
	}

	metrics := make([]float64, metricsLen)
	bestMetric := 0.0
	bestIdx := -1

	for d := 0; d < metricsLen; d++ {
		var pReal float64
		var rr float64

		for m := 0; m < halfLen; m++ {
			if d+m+halfLen >= len(signal) {
				break
			}
			a := signal[d+m]
			b := signal[d+m+halfLen]
			pReal += a * b
			rr += b * b
		}

		pMag := pReal * pReal
		if rr > 0 {
			metrics[d] = pMag / (rr * rr)
		}

		if metrics[d] > bestMetric {
			bestMetric = metrics[d]
			bestIdx = d
		}
	}

	if bestMetric > pd.threshold {
		return bestIdx, metrics
	}
	return -1, metrics
}

// EstimateFrequencyOffset estimates the fractional frequency offset from the preamble.
func EstimateFrequencyOffset(signal []float64, startIdx int, fftSize int) float64 {
	halfLen := fftSize / 2

	if startIdx+fftSize+halfLen > len(signal) {
		return 0
	}

	// Compute correlation angle
	var pReal, pImag float64
	for m := 0; m < halfLen; m++ {
		a := signal[startIdx+m]
		b := signal[startIdx+m+halfLen]
		pReal += a * b
	}
	_ = pImag

	// Fractional frequency offset = angle(P) / (π)
	angle := math.Atan2(pImag, pReal)
	return angle / math.Pi
}

func addCyclicPrefix(samples []float64, cpLen int) []float64 {
	n := len(samples)
	result := make([]float64, cpLen+n)
	// Copy last cpLen samples to the beginning
	copy(result, samples[n-cpLen:])
	copy(result[cpLen:], samples)
	return result
}

func normalizeAmplitude(samples []float64) {
	maxAbs := 0.0
	for _, s := range samples {
		if abs := math.Abs(s); abs > maxAbs {
			maxAbs = abs
		}
	}
	if maxAbs > 0 {
		scale := 0.8 / maxAbs // Leave some headroom
		for i := range samples {
			samples[i] *= scale
		}
	}
}
