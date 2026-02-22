package fec

import (
	"encoding/binary"
	"hash/crc32"
)

// CRC32 computes CRC-32 checksum using IEEE polynomial.
func CRC32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// AppendCRC32 appends 4-byte CRC-32 to the data.
func AppendCRC32(data []byte) []byte {
	checksum := CRC32(data)
	result := make([]byte, len(data)+4)
	copy(result, data)
	binary.BigEndian.PutUint32(result[len(data):], checksum)
	return result
}

// VerifyCRC32 verifies the CRC-32 at the end of the data.
// Returns the data without CRC and whether verification passed.
func VerifyCRC32(dataWithCRC []byte) ([]byte, bool) {
	if len(dataWithCRC) < 4 {
		return nil, false
	}

	data := dataWithCRC[:len(dataWithCRC)-4]
	expected := binary.BigEndian.Uint32(dataWithCRC[len(dataWithCRC)-4:])
	actual := CRC32(data)

	return data, actual == expected
}

// CRC32Bytes returns the CRC-32 as a 4-byte slice.
func CRC32Bytes(data []byte) []byte {
	checksum := CRC32(data)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, checksum)
	return buf
}
