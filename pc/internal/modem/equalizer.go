package modem

import (
	"math"
	"math/cmplx"
)

// Equalizer performs channel estimation and equalization for OFDM.
type Equalizer struct {
	fftSize     int
	channelResp []complex128 // H(k) channel frequency response
	startIdx    int
	endIdx      int
}

// NewEqualizer creates a new equalizer.
func NewEqualizer(fftSize, startIdx, endIdx int) *Equalizer {
	return &Equalizer{
		fftSize:     fftSize,
		channelResp: make([]complex128, fftSize),
		startIdx:    startIdx,
		endIdx:      endIdx,
	}
}

// EstimateChannel estimates the channel response from a known training symbol.
// received: FFT of the received training symbol
// known: the known transmitted training symbol in frequency domain
func (eq *Equalizer) EstimateChannel(received, known []complex128) {
	eq.channelResp = make([]complex128, eq.fftSize)

	for k := eq.startIdx; k <= eq.endIdx; k++ {
		if k >= len(received) || k >= len(known) {
			continue
		}
		if known[k] != 0 {
			// H(k) = Y(k) / X(k)
			eq.channelResp[k] = received[k] / known[k]
		}
	}

	// Interpolate channel response for unused subcarriers
	eq.interpolateChannel()
}

// interpolateChannel fills gaps in the channel estimate via linear interpolation.
func (eq *Equalizer) interpolateChannel() {
	// Find non-zero channel estimates
	type point struct {
		idx int
		val complex128
	}
	var points []point
	for k := eq.startIdx; k <= eq.endIdx; k++ {
		if eq.channelResp[k] != 0 {
			points = append(points, point{k, eq.channelResp[k]})
		}
	}

	if len(points) < 2 {
		return
	}

	// Linear interpolation between known points
	for i := 0; i < len(points)-1; i++ {
		k1, k2 := points[i].idx, points[i+1].idx
		v1, v2 := points[i].val, points[i+1].val

		for k := k1 + 1; k < k2; k++ {
			if eq.channelResp[k] == 0 {
				t := float64(k-k1) / float64(k2-k1)
				realPart := real(v1)*(1-t) + real(v2)*t
				imagPart := imag(v1)*(1-t) + imag(v2)*t
				eq.channelResp[k] = complex(realPart, imagPart)
			}
		}
	}
}

// Equalize performs zero-forcing equalization on received frequency-domain data.
func (eq *Equalizer) Equalize(receivedSpectrum []complex128) []complex128 {
	equalized := make([]complex128, len(receivedSpectrum))
	copy(equalized, receivedSpectrum)

	for k := eq.startIdx; k <= eq.endIdx; k++ {
		if k >= len(equalized) {
			break
		}
		h := eq.channelResp[k]
		if cmplx.Abs(h) > 1e-10 {
			equalized[k] = receivedSpectrum[k] / h
		}
	}

	return equalized
}

// EqualizeMMSE performs MMSE equalization (more robust in noisy conditions).
func (eq *Equalizer) EqualizeMMSE(receivedSpectrum []complex128, noisePower float64) []complex128 {
	equalized := make([]complex128, len(receivedSpectrum))
	copy(equalized, receivedSpectrum)

	for k := eq.startIdx; k <= eq.endIdx; k++ {
		if k >= len(equalized) {
			break
		}
		h := eq.channelResp[k]
		hConj := cmplx.Conj(h)
		hPow := real(h)*real(h) + imag(h)*imag(h)

		if hPow+noisePower > 1e-10 {
			// W(k) = H*(k) / (|H(k)|² + σ²)
			w := hConj / complex(hPow+noisePower, 0)
			equalized[k] = receivedSpectrum[k] * w
		}
	}

	return equalized
}

// GetChannelResponse returns the estimated channel response.
func (eq *Equalizer) GetChannelResponse() []complex128 {
	out := make([]complex128, len(eq.channelResp))
	copy(out, eq.channelResp)
	return out
}

// GetSNREstimate estimates the SNR per subcarrier from pilot symbols.
func (eq *Equalizer) GetSNREstimate(receivedPilots, expectedPilots []complex128) []float64 {
	snr := make([]float64, len(receivedPilots))
	for i := range receivedPilots {
		if i >= len(expectedPilots) {
			break
		}
		signal := cmplx.Abs(expectedPilots[i])
		noise := cmplx.Abs(receivedPilots[i] - expectedPilots[i])
		if noise > 1e-10 {
			snr[i] = 20 * logBase10(signal/noise) // dB
		} else {
			snr[i] = 60 // very high SNR
		}
	}
	return snr
}

func logBase10(x float64) float64 {
	if x <= 0 {
		return -100
	}
	return math.Log10(x)
}
