package modem

import (
	"fmt"
	"math"
	"math/cmplx"
)

// OFDM parameters
const (
	FFTSize         = 512
	CPLen           = 64
	SymbolLen       = FFTSize + CPLen // 576 samples
	SampleRate      = 44100
	SubcarrierStart = 12  // ~1032 Hz (avoid DC and low freq)
	SubcarrierEnd   = 232 // ~19992 Hz
	NumPilots       = 16
)

// NumDataSubcarriers returns the number of data subcarriers.
func NumDataSubcarriers() int {
	count := 0
	for i := SubcarrierStart; i <= SubcarrierEnd; i++ {
		if !IsPilot(i) {
			count++
		}
	}
	return count
}

// BitsPerSymbol returns the total data bits per OFDM symbol for a given modulation.
func BitsPerOFDMSymbol(mod Modulation) int {
	return NumDataSubcarriers() * mod.BitsPerSymbol()
}

// Modulator handles OFDM modulation (bits → audio samples).
type Modulator struct {
	constellation *Constellation
	mod           Modulation
	fftSize       int
	cpLen         int
}

// NewModulator creates an OFDM modulator.
func NewModulator(mod Modulation) *Modulator {
	return &Modulator{
		constellation: NewConstellation(mod),
		mod:           mod,
		fftSize:       FFTSize,
		cpLen:         CPLen,
	}
}

// Modulate converts data bits into OFDM audio samples.
// bits: slice of 0/1 bytes, length must be multiple of BitsPerOFDMSymbol.
// Returns float64 audio samples.
func (m *Modulator) Modulate(bits []byte) ([]float64, error) {
	bitsPerSymbol := BitsPerOFDMSymbol(m.mod)
	if len(bits)%bitsPerSymbol != 0 {
		return nil, fmt.Errorf("bit count %d is not multiple of %d", len(bits), bitsPerSymbol)
	}

	numSymbols := len(bits) / bitsPerSymbol
	samples := make([]float64, 0, numSymbols*SymbolLen)

	for i := 0; i < numSymbols; i++ {
		symbolBits := bits[i*bitsPerSymbol : (i+1)*bitsPerSymbol]
		symbolSamples := m.modulateSymbol(symbolBits)
		samples = append(samples, symbolSamples...)
	}

	return samples, nil
}

// ModulateSingle modulates a single OFDM symbol from bits.
func (m *Modulator) ModulateSingle(bits []byte) []float64 {
	return m.modulateSymbol(bits)
}

func (m *Modulator) modulateSymbol(bits []byte) []float64 {
	// Map bits to constellation symbols
	dataSymbols := m.constellation.MapBits(bits)

	// Insert data and pilot symbols into subcarrier spectrum
	spectrum := InsertPilots(dataSymbols, m.fftSize, SubcarrierStart, SubcarrierEnd)

	// Apply Hermitian symmetry for real-valued output
	applyHermitianSymmetry(spectrum)

	// IFFT to time domain
	timeDomain := RealIFFT(spectrum)

	// Add cyclic prefix
	withCP := addCyclicPrefix(timeDomain, m.cpLen)

	// Normalize amplitude to prevent clipping
	normalizeAmplitude(withCP)

	return withCP
}

// Demodulator handles OFDM demodulation (audio samples → bits).
type Demodulator struct {
	constellation *Constellation
	mod           Modulation
	equalizer     *Equalizer
	fftSize       int
	cpLen         int
}

// NewDemodulator creates an OFDM demodulator.
func NewDemodulator(mod Modulation) *Demodulator {
	return &Demodulator{
		constellation: NewConstellation(mod),
		mod:           mod,
		equalizer:     NewEqualizer(FFTSize, SubcarrierStart, SubcarrierEnd),
		fftSize:       FFTSize,
		cpLen:         CPLen,
	}
}

// SetChannelEstimate sets the channel estimate for equalization.
func (d *Demodulator) SetChannelEstimate(received, known []complex128) {
	d.equalizer.EstimateChannel(received, known)
}

// Demodulate converts OFDM audio samples back to data bits.
func (d *Demodulator) Demodulate(samples []float64) ([]byte, error) {
	numSymbols := len(samples) / SymbolLen
	if numSymbols == 0 {
		return nil, fmt.Errorf("insufficient samples: %d < %d", len(samples), SymbolLen)
	}

	var allBits []byte
	for i := 0; i < numSymbols; i++ {
		symbolSamples := samples[i*SymbolLen : (i+1)*SymbolLen]
		bits := d.demodulateSymbol(symbolSamples)
		allBits = append(allBits, bits...)
	}

	return allBits, nil
}

// DemodulateSingle demodulates a single OFDM symbol.
func (d *Demodulator) DemodulateSingle(samples []float64) []byte {
	return d.demodulateSymbol(samples)
}

func (d *Demodulator) demodulateSymbol(samples []float64) []byte {
	// Remove cyclic prefix
	withoutCP := removeCyclicPrefix(samples, d.cpLen)

	// FFT to frequency domain
	cx := make([]complex128, len(withoutCP))
	for i, v := range withoutCP {
		cx[i] = complex(v, 0)
	}
	spectrum := FFT(cx)

	// Equalize
	equalized := d.equalizer.Equalize(spectrum)

	// Extract and correct phase using pilots
	receivedPilots := ExtractPilots(equalized)
	phaseOffset := EstimatePhaseOffset(receivedPilots)

	// Extract data symbols
	dataSymbols := ExtractData(equalized, SubcarrierStart, SubcarrierEnd)

	// Apply phase correction
	corrected := CorrectPhase(dataSymbols, phaseOffset)

	// Demap to bits
	bits := d.constellation.DemapSymbols(corrected)

	return bits
}

func removeCyclicPrefix(samples []float64, cpLen int) []float64 {
	if len(samples) <= cpLen {
		return samples
	}
	return samples[cpLen:]
}

func applyHermitianSymmetry(spectrum []complex128) {
	n := len(spectrum)
	for k := 1; k < n/2; k++ {
		spectrum[n-k] = cmplx.Conj(spectrum[k])
	}
	spectrum[0] = 0
	spectrum[n/2] = complex(real(spectrum[n/2]), 0)
}

// GenerateFrame generates a complete transmittable frame:
// [Preamble1][Preamble2][ChannelEst][Data symbols...]
func GenerateFrame(data []byte, mod Modulation) []float64 {
	modulator := NewModulator(mod)

	// Generate preamble
	pg := NewPreambleGenerator(42)
	preamble1, preamble2 := pg.GenerateSchmidlCox()
	channelEst, _ := pg.GenerateChannelEstimation()

	// Convert data bytes to bits
	bits := bytesToBits(data)

	// Pad to multiple of BitsPerOFDMSymbol
	bitsPerSym := BitsPerOFDMSymbol(mod)
	if rem := len(bits) % bitsPerSym; rem != 0 {
		padding := make([]byte, bitsPerSym-rem)
		bits = append(bits, padding...)
	}

	// Modulate data
	dataSamples, _ := modulator.Modulate(bits)

	// Combine: preamble + channel estimation + data
	var frame []float64
	frame = append(frame, preamble1...)
	frame = append(frame, preamble2...)
	frame = append(frame, channelEst...)
	frame = append(frame, dataSamples...)

	return frame
}

// ReceiveFrame detects and demodulates a frame from received samples.
func ReceiveFrame(samples []float64, mod Modulation, expectedBits int) ([]byte, error) {
	detector := NewPreambleDetector()

	// Detect preamble
	startIdx := detector.Detect(samples)
	if startIdx < 0 {
		return nil, fmt.Errorf("preamble not detected")
	}

	// Skip preamble (2 symbols) and move to channel estimation symbol
	channelEstStart := startIdx + 2*SymbolLen
	if channelEstStart+SymbolLen > len(samples) {
		return nil, fmt.Errorf("insufficient samples for channel estimation")
	}

	// Extract channel estimation symbol and estimate channel
	ceSymbol := samples[channelEstStart : channelEstStart+SymbolLen]
	ceSamples := removeCyclicPrefix(ceSymbol, CPLen)
	cx := make([]complex128, len(ceSamples))
	for i, v := range ceSamples {
		cx[i] = complex(v, 0)
	}
	receivedCE := FFT(cx)

	pg := NewPreambleGenerator(42)
	_, knownCE := pg.GenerateChannelEstimation()

	demod := NewDemodulator(mod)
	demod.SetChannelEstimate(receivedCE, knownCE)

	// Extract data symbols
	dataStart := channelEstStart + SymbolLen
	if dataStart >= len(samples) {
		return nil, fmt.Errorf("no data samples after channel estimation")
	}

	dataSamples := samples[dataStart:]
	bits, err := demod.Demodulate(dataSamples)
	if err != nil {
		return nil, fmt.Errorf("demodulation: %w", err)
	}

	// Convert bits to bytes
	if expectedBits > 0 && expectedBits < len(bits) {
		bits = bits[:expectedBits]
	}
	data := bitsToBytes(bits)

	return data, nil
}

func bytesToBits(data []byte) []byte {
	bits := make([]byte, len(data)*8)
	for i, b := range data {
		for j := 7; j >= 0; j-- {
			bits[i*8+(7-j)] = (b >> uint(j)) & 1
		}
	}
	return bits
}

func bitsToBytes(bits []byte) []byte {
	numBytes := len(bits) / 8
	data := make([]byte, numBytes)
	for i := 0; i < numBytes; i++ {
		var b byte
		for j := 0; j < 8; j++ {
			b = (b << 1) | (bits[i*8+j] & 1)
		}
		data[i] = b
	}
	return data
}

// SamplesToFloat32 converts float64 samples to float32 for audio output.
func SamplesToFloat32(samples []float64) []float32 {
	out := make([]float32, len(samples))
	for i, s := range samples {
		out[i] = float32(s)
	}
	return out
}

// Float32ToSamples converts float32 audio input to float64 for processing.
func Float32ToSamples(samples []float32) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = float64(s)
	}
	return out
}

// ApplyDCRemoval removes DC offset from samples using a high-pass filter.
func ApplyDCRemoval(samples []float64) []float64 {
	if len(samples) == 0 {
		return samples
	}

	// Simple DC removal: subtract running average
	alpha := 0.999 // high-pass filter coefficient
	out := make([]float64, len(samples))
	dc := samples[0]
	for i, s := range samples {
		dc = alpha*dc + (1-alpha)*s
		out[i] = s - dc
	}
	return out
}

// ApplyAGC applies Automatic Gain Control to normalize signal level.
func ApplyAGC(samples []float64, targetRMS float64) []float64 {
	if len(samples) == 0 {
		return samples
	}

	// Calculate current RMS
	var sumSq float64
	for _, s := range samples {
		sumSq += s * s
	}
	rms := math.Sqrt(sumSq / float64(len(samples)))

	if rms < 1e-10 {
		return samples
	}

	gain := targetRMS / rms
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = s * gain
	}
	return out
}
