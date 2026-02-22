package fec

import (
	"testing"
)

func TestCRC32_Basic(t *testing.T) {
	data := []byte("Hello, World!")
	checksum := CRC32(data)

	if checksum == 0 {
		t.Error("CRC32 should not be 0 for non-empty data")
	}

	// Same data should produce same CRC
	checksum2 := CRC32(data)
	if checksum != checksum2 {
		t.Errorf("CRC32 not deterministic: %x != %x", checksum, checksum2)
	}

	// Different data should produce different CRC
	data2 := []byte("Hello, World?")
	checksum3 := CRC32(data2)
	if checksum == checksum3 {
		t.Error("Different data produced same CRC32")
	}
}

func TestCRC32_AppendVerify(t *testing.T) {
	data := []byte("Test data for CRC verification")

	withCRC := AppendCRC32(data)
	if len(withCRC) != len(data)+4 {
		t.Fatalf("Expected length %d, got %d", len(data)+4, len(withCRC))
	}

	recovered, valid := VerifyCRC32(withCRC)
	if !valid {
		t.Error("CRC verification failed for valid data")
	}

	if string(recovered) != string(data) {
		t.Error("Recovered data mismatch")
	}

	// Corrupt data and verify detection
	withCRC[5] ^= 0xFF
	_, valid = VerifyCRC32(withCRC)
	if valid {
		t.Error("CRC verification should fail for corrupted data")
	}
}

func TestRSEncoder_EncodeBlock(t *testing.T) {
	rs, err := NewRSEncoder()
	if err != nil {
		t.Fatalf("Failed to create RS encoder: %v", err)
	}

	data := make([]byte, 200)
	for i := range data {
		data[i] = byte(i)
	}

	encoded, err := rs.EncodeBlock(data)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	expectedLen := DefaultDataShards + DefaultParityShards
	if len(encoded) != expectedLen {
		t.Errorf("Encoded length: %d, expected %d", len(encoded), expectedLen)
	}
}

func TestRSEncoder_EncodeDecode(t *testing.T) {
	rs, err := NewRSEncoder()
	if err != nil {
		t.Fatalf("Failed to create RS encoder: %v", err)
	}

	data := []byte("This is test data for Reed-Solomon encoding and decoding verification. " +
		"The data should survive encoding and decoding without errors.")

	encoded, err := rs.Encode(data)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	decoded, err := rs.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	// Decoded should contain the original data (padded to shard size)
	for i := range data {
		if i < len(decoded) && data[i] != decoded[i] {
			t.Errorf("Byte %d mismatch: 0x%02x != 0x%02x", i, data[i], decoded[i])
		}
	}
}

func TestRSEncoder_ErrorCorrection(t *testing.T) {
	rs, err := NewRSEncoderCustom(10, 4) // Smaller for testing: 10 data, 4 parity
	if err != nil {
		t.Fatalf("Failed to create RS encoder: %v", err)
	}

	data := []byte("Hello RS!!")  // Exactly 10 bytes

	encoded, err := rs.EncodeBlock(data)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	// Introduce errors (up to 2 erasures can be corrected with 4 parity shards)
	corrupted := make([]byte, len(encoded))
	copy(corrupted, encoded)

	// Mark 2 positions as erasures
	erasures := []int{2, 5}
	for _, idx := range erasures {
		corrupted[idx] = 0
	}

	decoded, err := rs.DecodeBlock(corrupted, erasures)
	if err != nil {
		t.Fatalf("Decode error with erasures: %v", err)
	}

	for i := range data {
		if decoded[i] != data[i] {
			t.Errorf("Byte %d: 0x%02x != 0x%02x", i, decoded[i], data[i])
		}
	}
}
