package protocol

import (
	"testing"
)

func TestFrame_EncodeDecode(t *testing.T) {
	tests := []struct {
		name  string
		frame *Frame
	}{
		{
			name:  "DATA frame",
			frame: NewDataFrame(42, []byte("Hello, World!")),
		},
		{
			name:  "ACK frame",
			frame: NewACKFrame(42),
		},
		{
			name:  "NACK frame",
			frame: NewNACKFrame(7),
		},
		{
			name:  "PING frame",
			frame: NewPingFrame(),
		},
		{
			name:  "PONG frame",
			frame: NewPongFrame(),
		},
		{
			name:  "CONTROL frame",
			frame: NewControlFrame([]byte{0x01, 0x02}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.frame.Encode()
			decoded, err := DecodeFrame(encoded)
			if err != nil {
				t.Fatalf("Decode error: %v", err)
			}

			if decoded.Type != tt.frame.Type {
				t.Errorf("Type: 0x%02x != 0x%02x", decoded.Type, tt.frame.Type)
			}
			if decoded.SeqNum != tt.frame.SeqNum {
				t.Errorf("SeqNum: %d != %d", decoded.SeqNum, tt.frame.SeqNum)
			}
			if decoded.PayloadLen != tt.frame.PayloadLen {
				t.Errorf("PayloadLen: %d != %d", decoded.PayloadLen, tt.frame.PayloadLen)
			}
			if tt.frame.PayloadLen > 0 {
				for i := 0; i < int(tt.frame.PayloadLen); i++ {
					if decoded.Payload[i] != tt.frame.Payload[i] {
						t.Errorf("Payload[%d]: 0x%02x != 0x%02x", i, decoded.Payload[i], tt.frame.Payload[i])
					}
				}
			}
		})
	}
}

func TestFrame_CRCDetectsCorruption(t *testing.T) {
	frame := NewDataFrame(1, []byte("Integrity test"))
	encoded := frame.Encode()

	// Corrupt one byte
	encoded[5] ^= 0xFF

	_, err := DecodeFrame(encoded)
	if err == nil {
		t.Error("Expected CRC error for corrupted frame")
	}
}

func TestFrame_TooShort(t *testing.T) {
	_, err := DecodeFrame([]byte{0x01, 0x02})
	if err == nil {
		t.Error("Expected error for short frame")
	}
}

func TestFileMeta_EncodeDecode(t *testing.T) {
	meta := &FileMetadata{
		Filename: "test_file.txt",
		Size:     12345,
		MD5Hash:  "d41d8cd98f00b204e9800998ecf8427e",
	}

	encoded := EncodeFileMeta(meta)
	decoded, err := DecodeFileMeta(encoded)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if decoded.Filename != meta.Filename {
		t.Errorf("Filename: %s != %s", decoded.Filename, meta.Filename)
	}
	if decoded.Size != meta.Size {
		t.Errorf("Size: %d != %d", decoded.Size, meta.Size)
	}
	if decoded.MD5Hash != meta.MD5Hash {
		t.Errorf("MD5: %s != %s", decoded.MD5Hash, meta.MD5Hash)
	}
}
