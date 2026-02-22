package modem

import (
	"testing"
)

func TestQPSK_MapDemap(t *testing.T) {
	c := NewConstellation(ModQPSK)

	// Test all 4 QPSK points
	for i := 0; i < 4; i++ {
		bits := indexToBits(i, 2)
		symbol := c.Map(bits)
		recovered := c.Demap(symbol)

		for j := range bits {
			if bits[j] != recovered[j] {
				t.Errorf("QPSK point %d: bit %d mismatch: %d != %d", i, j, bits[j], recovered[j])
			}
		}
	}
}

func Test16QAM_MapDemap(t *testing.T) {
	c := NewConstellation(Mod16QAM)

	// Test all 16 points
	for i := 0; i < 16; i++ {
		bits := indexToBits(i, 4)
		symbol := c.Map(bits)
		recovered := c.Demap(symbol)

		for j := range bits {
			if bits[j] != recovered[j] {
				t.Errorf("16QAM point %d: bit %d mismatch: %d != %d", i, j, bits[j], recovered[j])
			}
		}
	}
}

func Test64QAM_MapDemap(t *testing.T) {
	c := NewConstellation(Mod64QAM)

	// Test all 64 points
	for i := 0; i < 64; i++ {
		bits := indexToBits(i, 6)
		symbol := c.Map(bits)
		recovered := c.Demap(symbol)

		for j := range bits {
			if bits[j] != recovered[j] {
				t.Errorf("64QAM point %d: bit %d mismatch: %d != %d", i, j, bits[j], recovered[j])
			}
		}
	}
}

func TestConstellation_MapBits_DemapSymbols(t *testing.T) {
	c := NewConstellation(Mod16QAM)

	bits := []byte{1, 0, 1, 1, 0, 0, 1, 0, 1, 1, 0, 0}
	symbols := c.MapBits(bits)
	recovered := c.DemapSymbols(symbols)

	if len(recovered) != len(bits) {
		t.Fatalf("length mismatch: %d != %d", len(recovered), len(bits))
	}

	for i := range bits {
		if bits[i] != recovered[i] {
			t.Errorf("bit %d: %d != %d", i, bits[i], recovered[i])
		}
	}
}

func TestBitsToIndex_IndexToBits(t *testing.T) {
	tests := []struct {
		idx     int
		numBits int
		bits    []byte
	}{
		{0, 2, []byte{0, 0}},
		{1, 2, []byte{0, 1}},
		{2, 2, []byte{1, 0}},
		{3, 2, []byte{1, 1}},
		{5, 4, []byte{0, 1, 0, 1}},
		{15, 4, []byte{1, 1, 1, 1}},
	}

	for _, tt := range tests {
		bits := indexToBits(tt.idx, tt.numBits)
		idx := bitsToIndex(bits)

		if idx != tt.idx {
			t.Errorf("roundtrip failed for idx=%d: got %d", tt.idx, idx)
		}
	}
}
