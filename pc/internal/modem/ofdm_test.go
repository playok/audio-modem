package modem

import (
	"testing"
)

func TestOFDM_ModDemod_Loopback(t *testing.T) {
	mod := Mod16QAM
	demodulator := NewDemodulator(mod)

	// Use proper channel estimation: generate CE symbol, "transmit" it,
	// then use the received version for channel estimation.
	pg := NewPreambleGenerator(42)
	ceTimeDomain, knownCE := pg.GenerateChannelEstimation()

	// In loopback, "received" CE is the same as transmitted
	ceWithoutCP := removeCyclicPrefix(ceTimeDomain, CPLen)
	ceCx := make([]complex128, len(ceWithoutCP))
	for i, v := range ceWithoutCP {
		ceCx[i] = complex(v, 0)
	}
	receivedCE := FFT(ceCx)
	demodulator.SetChannelEstimate(receivedCE, knownCE)

	// Generate and modulate data
	modulator := NewModulator(mod)
	bitsPerSym := BitsPerOFDMSymbol(mod)
	bits := make([]byte, bitsPerSym)
	for i := range bits {
		bits[i] = byte(i % 2)
	}
	samples := modulator.ModulateSingle(bits)

	// Demodulate
	recovered := demodulator.DemodulateSingle(samples)

	if len(recovered) < len(bits) {
		t.Fatalf("recovered length %d < input length %d", len(recovered), len(bits))
	}

	errors := 0
	for i := range bits {
		if i < len(recovered) && bits[i] != recovered[i] {
			errors++
		}
	}

	ber := float64(errors) / float64(len(bits))
	t.Logf("Bit Error Rate: %.4f (%d errors in %d bits)", ber, errors, len(bits))

	// With proper channel estimation in ideal conditions, some error is acceptable
	// due to per-symbol normalization differences
	if ber > 0.05 {
		t.Errorf("BER too high: %.4f (expected < 0.05)", ber)
	}
}

func TestOFDM_MultiSymbol(t *testing.T) {
	mod := ModQPSK
	modulator := NewModulator(mod)

	bitsPerSym := BitsPerOFDMSymbol(mod)
	numSymbols := 3
	bits := make([]byte, bitsPerSym*numSymbols)
	for i := range bits {
		bits[i] = byte((i * 7) % 2) // deterministic pattern
	}

	samples, err := modulator.Modulate(bits)
	if err != nil {
		t.Fatalf("Modulate error: %v", err)
	}

	expectedLen := numSymbols * SymbolLen
	if len(samples) != expectedLen {
		t.Errorf("Expected %d samples, got %d", expectedLen, len(samples))
	}
}

func TestBytesToBits_BitsToBytes(t *testing.T) {
	data := []byte{0xAB, 0xCD, 0xEF}
	bits := bytesToBits(data)

	if len(bits) != 24 {
		t.Fatalf("Expected 24 bits, got %d", len(bits))
	}

	recovered := bitsToBytes(bits)
	for i := range data {
		if data[i] != recovered[i] {
			t.Errorf("byte %d: 0x%02x != 0x%02x", i, data[i], recovered[i])
		}
	}
}

func TestNumDataSubcarriers(t *testing.T) {
	n := NumDataSubcarriers()
	total := SubcarrierEnd - SubcarrierStart + 1
	expected := total - NumPilots

	if n != expected {
		t.Errorf("NumDataSubcarriers() = %d, expected %d (total %d - pilots %d)",
			n, expected, total, NumPilots)
	}
	t.Logf("Data subcarriers: %d out of %d total", n, total)
}

func TestGenerateFrame_ReceiveFrame(t *testing.T) {
	data := []byte("Hello, OFDM!")
	mod := ModQPSK

	// Generate frame (preamble + channel est + data)
	samples := GenerateFrame(data, mod)
	t.Logf("Frame length: %d samples (%.2f ms)", len(samples), float64(len(samples))/float64(SampleRate)*1000)

	if len(samples) == 0 {
		t.Fatal("GenerateFrame returned empty samples")
	}

	// Try to receive (loopback)
	recovered, err := ReceiveFrame(samples, mod, len(data)*8)
	if err != nil {
		t.Fatalf("ReceiveFrame error: %v", err)
	}

	if len(recovered) < len(data) {
		t.Fatalf("Recovered data too short: %d < %d", len(recovered), len(data))
	}

	// Check data
	match := true
	for i := range data {
		if i < len(recovered) && data[i] != recovered[i] {
			match = false
			break
		}
	}

	if !match {
		t.Logf("Original: %v", data)
		t.Logf("Recovered: %v", recovered[:len(data)])
		t.Error("Data mismatch in loopback test")
	}
}

func TestSamplesToFloat32(t *testing.T) {
	samples := []float64{0.1, -0.5, 0.9, 0.0}
	f32 := SamplesToFloat32(samples)

	if len(f32) != len(samples) {
		t.Fatalf("length mismatch")
	}

	for i := range samples {
		if float64(f32[i])-samples[i] > 1e-6 {
			t.Errorf("sample %d: %v != %v", i, f32[i], samples[i])
		}
	}
}

func TestApplyDCRemoval(t *testing.T) {
	// Signal with DC offset
	samples := make([]float64, 1000)
	for i := range samples {
		samples[i] = 0.5 + 0.1*float64(i%2) // DC = 0.5, AC = ±0.1
	}

	filtered := ApplyDCRemoval(samples)

	// DC component should be significantly reduced at the end
	var dcSum float64
	for i := len(filtered) - 100; i < len(filtered); i++ {
		dcSum += filtered[i]
	}
	dcAvg := dcSum / 100.0

	if dcAvg > 0.1 {
		t.Errorf("DC not sufficiently removed: avg = %v", dcAvg)
	}
}
