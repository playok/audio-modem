package modem

import (
	"math"
)

// Modulation represents a QAM modulation scheme.
type Modulation int

const (
	ModQPSK  Modulation = 2 // 2 bits per symbol
	Mod16QAM Modulation = 4 // 4 bits per symbol
	Mod64QAM Modulation = 6 // 6 bits per symbol
)

// BitsPerSymbol returns the number of bits per constellation symbol.
func (m Modulation) BitsPerSymbol() int {
	return int(m)
}

// String returns the modulation name.
func (m Modulation) String() string {
	switch m {
	case ModQPSK:
		return "QPSK"
	case Mod16QAM:
		return "16-QAM"
	case Mod64QAM:
		return "64-QAM"
	default:
		return "Unknown"
	}
}

// Constellation holds QAM constellation points.
type Constellation struct {
	Mod    Modulation
	points []complex128
	scale  float64 // normalization factor for unit average power
}

// NewConstellation creates a new constellation for the given modulation.
func NewConstellation(mod Modulation) *Constellation {
	c := &Constellation{Mod: mod}
	switch mod {
	case ModQPSK:
		c.generateQPSK()
	case Mod16QAM:
		c.generateQAM(4) // 4x4
	case Mod64QAM:
		c.generateQAM(8) // 8x8
	default:
		c.generateQPSK()
	}
	c.normalize()
	return c
}

func (c *Constellation) generateQPSK() {
	// Gray-coded QPSK: 00, 01, 11, 10
	c.points = []complex128{
		complex(1, 1),   // 00
		complex(-1, 1),  // 01
		complex(-1, -1), // 11
		complex(1, -1),  // 10
	}
}

func (c *Constellation) generateQAM(order int) {
	// Generate square QAM constellation with Gray coding
	size := order * order
	c.points = make([]complex128, size)

	for i := 0; i < size; i++ {
		row := i / order
		col := i % order

		// Gray code mapping
		grayRow := row ^ (row >> 1)
		grayCol := col ^ (col >> 1)

		// Map to symmetric constellation
		x := float64(2*grayCol-order+1) // odd values: -3, -1, 1, 3 for 4-QAM
		y := float64(2*grayRow-order+1)

		c.points[i] = complex(x, y)
	}
}

func (c *Constellation) normalize() {
	// Calculate average power
	var avgPower float64
	for _, p := range c.points {
		avgPower += real(p)*real(p) + imag(p)*imag(p)
	}
	avgPower /= float64(len(c.points))

	// Normalize to unit average power
	c.scale = 1.0 / math.Sqrt(avgPower)
	for i := range c.points {
		c.points[i] = complex(real(c.points[i])*c.scale, imag(c.points[i])*c.scale)
	}
}

// Map maps bits to a constellation point.
func (c *Constellation) Map(bits []byte) complex128 {
	idx := bitsToIndex(bits)
	if idx >= len(c.points) {
		idx = 0
	}
	return c.points[idx]
}

// Demap finds the closest constellation point and returns the bits.
func (c *Constellation) Demap(symbol complex128) []byte {
	minDist := math.MaxFloat64
	minIdx := 0

	for i, p := range c.points {
		d := real(symbol-p)*real(symbol-p) + imag(symbol-p)*imag(symbol-p)
		if d < minDist {
			minDist = d
			minIdx = i
		}
	}

	return indexToBits(minIdx, c.Mod.BitsPerSymbol())
}

// MapBits maps a bit slice to constellation symbols.
// bits are packed as bytes (0 or 1 each).
func (c *Constellation) MapBits(bits []byte) []complex128 {
	bps := c.Mod.BitsPerSymbol()
	numSymbols := len(bits) / bps
	symbols := make([]complex128, numSymbols)

	for i := 0; i < numSymbols; i++ {
		symbols[i] = c.Map(bits[i*bps : (i+1)*bps])
	}
	return symbols
}

// DemapSymbols demaps constellation symbols back to bits.
func (c *Constellation) DemapSymbols(symbols []complex128) []byte {
	bps := c.Mod.BitsPerSymbol()
	bits := make([]byte, 0, len(symbols)*bps)

	for _, s := range symbols {
		bits = append(bits, c.Demap(s)...)
	}
	return bits
}

func bitsToIndex(bits []byte) int {
	idx := 0
	for _, b := range bits {
		idx = (idx << 1) | int(b&1)
	}
	return idx
}

func indexToBits(idx, numBits int) []byte {
	bits := make([]byte, numBits)
	for i := numBits - 1; i >= 0; i-- {
		bits[i] = byte(idx & 1)
		idx >>= 1
	}
	return bits
}
