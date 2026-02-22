package modem

import (
	"testing"
)

func TestPreambleGeneration(t *testing.T) {
	pg := NewPreambleGenerator(42)
	sym1, sym2 := pg.GenerateSchmidlCox()

	expectedLen := FFTSize + CPLen
	if len(sym1) != expectedLen {
		t.Errorf("Symbol 1 length: %d, expected %d", len(sym1), expectedLen)
	}
	if len(sym2) != expectedLen {
		t.Errorf("Symbol 2 length: %d, expected %d", len(sym2), expectedLen)
	}

	// Check amplitude is reasonable (normalized)
	for i, s := range sym1 {
		if s > 1.0 || s < -1.0 {
			t.Errorf("Symbol 1 sample %d out of range: %v", i, s)
			break
		}
	}
}

func TestPreambleDetection(t *testing.T) {
	pg := NewPreambleGenerator(42)
	sym1, sym2 := pg.GenerateSchmidlCox()

	// Create signal: silence + preamble + some data
	silence := make([]float64, 1000)
	var signal []float64
	signal = append(signal, silence...)
	signal = append(signal, sym1...)
	signal = append(signal, sym2...)
	// Add some dummy data after preamble
	dummy := make([]float64, 2000)
	signal = append(signal, dummy...)

	detector := NewPreambleDetector()
	idx := detector.Detect(signal)

	if idx < 0 {
		t.Fatal("Preamble not detected")
	}

	// The Schmidl-Cox metric peaks within the preamble region.
	// The exact peak position depends on the CP and autocorrelation window.
	// It should be within the preamble area [1000, 1000 + 2*SymbolLen].
	preambleStart := 1000
	preambleEnd := preambleStart + 2*SymbolLen
	if idx < preambleStart || idx > preambleEnd {
		t.Errorf("Preamble detected at %d, expected within [%d, %d]", idx, preambleStart, preambleEnd)
	}
	t.Logf("Preamble detected at index %d (preamble region: [%d, %d])", idx, preambleStart, preambleEnd)
}

func TestChannelEstimation(t *testing.T) {
	pg := NewPreambleGenerator(42)
	samples, known := pg.GenerateChannelEstimation()

	if len(samples) != FFTSize+CPLen {
		t.Errorf("Channel estimation symbol length: %d, expected %d", len(samples), FFTSize+CPLen)
	}

	// Verify known symbols are non-zero in the data band
	nonZero := 0
	for k := SubcarrierStart; k <= SubcarrierEnd; k++ {
		if known[k] != 0 {
			nonZero++
		}
	}

	if nonZero == 0 {
		t.Error("No non-zero known symbols in channel estimation")
	}
	t.Logf("Channel estimation: %d non-zero known symbols", nonZero)
}
