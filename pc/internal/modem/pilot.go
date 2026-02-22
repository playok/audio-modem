package modem

// Pilot subcarrier management for OFDM.
// Pilots are used for phase tracking and channel estimation.

// PilotPattern defines the pilot subcarrier indices within the data band.
// 16 pilots evenly spaced across 200+ data subcarriers.
var PilotPattern = []int{
	15, 29, 43, 57, 71, 85, 99, 113,
	127, 141, 155, 169, 183, 197, 211, 225,
}

// PilotValue is the known pilot symbol (BPSK +1).
var PilotValue = complex(1, 0)

// IsPilot returns true if the given subcarrier index is a pilot.
func IsPilot(subcarrierIdx int) bool {
	for _, p := range PilotPattern {
		if subcarrierIdx == p {
			return true
		}
	}
	return false
}

// DataSubcarriers returns the subcarrier indices used for data
// (excluding pilots) within the range [startIdx, endIdx].
func DataSubcarriers(startIdx, endIdx int) []int {
	var data []int
	for i := startIdx; i <= endIdx; i++ {
		if !IsPilot(i) {
			data = append(data, i)
		}
	}
	return data
}

// InsertPilots inserts pilot symbols into the subcarrier array.
func InsertPilots(dataSymbols []complex128, fftSize int, startIdx, endIdx int) []complex128 {
	spectrum := make([]complex128, fftSize)

	dataIdx := 0
	for i := startIdx; i <= endIdx; i++ {
		if IsPilot(i) {
			spectrum[i] = PilotValue
		} else if dataIdx < len(dataSymbols) {
			spectrum[i] = dataSymbols[dataIdx]
			dataIdx++
		}
	}

	return spectrum
}

// ExtractPilots extracts pilot values from the received spectrum.
func ExtractPilots(spectrum []complex128) []complex128 {
	pilots := make([]complex128, len(PilotPattern))
	for i, idx := range PilotPattern {
		if idx < len(spectrum) {
			pilots[i] = spectrum[idx]
		}
	}
	return pilots
}

// ExtractData extracts data symbols from the received spectrum (excluding pilots).
func ExtractData(spectrum []complex128, startIdx, endIdx int) []complex128 {
	var data []complex128
	for i := startIdx; i <= endIdx; i++ {
		if !IsPilot(i) && i < len(spectrum) {
			data = append(data, spectrum[i])
		}
	}
	return data
}

// EstimatePhaseOffset estimates the common phase error from pilot symbols.
func EstimatePhaseOffset(receivedPilots []complex128) float64 {
	var sumAngle float64
	count := 0
	for _, p := range receivedPilots {
		if p != 0 {
			// Pilot should be +1, so angle of received pilot = phase offset
			angle := imag(p) // small angle approximation: Im(p)/Re(p) ≈ angle
			if real(p) != 0 {
				angle = imag(p) / real(p)
			}
			sumAngle += angle
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sumAngle / float64(count)
}

// CorrectPhase applies common phase error correction to data symbols.
func CorrectPhase(symbols []complex128, phaseOffset float64) []complex128 {
	corrected := make([]complex128, len(symbols))
	correction := complex(1, -phaseOffset) // small angle: e^{-jθ} ≈ 1 - jθ
	normFactor := 1.0 / abs128(correction)
	correction *= complex(normFactor, 0)

	for i, s := range symbols {
		corrected[i] = s * correction
	}
	return corrected
}

func abs128(c complex128) float64 {
	r, im := real(c), imag(c)
	return r*r + im*im
}
