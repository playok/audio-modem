package protocol

import (
	"encoding/binary"
	"fmt"

	"github.com/jeongseonghan/audio-modem/internal/fec"
)

// Frame types
const (
	TypeData     byte = 0x01
	TypeACK      byte = 0x02
	TypeNACK     byte = 0x03
	TypeControl  byte = 0x04
	TypeFileMeta byte = 0x05
	TypeFileEnd  byte = 0x06
	TypePing     byte = 0x07
	TypePong     byte = 0x08
)

// Frame size limits
const (
	HeaderSize     = 4
	MaxPayloadSize = 1024
	CRCSize        = 4
)

// Frame represents a protocol frame.
// Format: [Type(1B)][SeqNum(1B)][PayloadLen(2B)][Payload][CRC-32(4B)]
type Frame struct {
	Type       byte
	SeqNum     byte
	PayloadLen uint16
	Payload    []byte
}

// TypeName returns a human-readable name for the frame type.
func (f *Frame) TypeName() string {
	switch f.Type {
	case TypeData:
		return "DATA"
	case TypeACK:
		return "ACK"
	case TypeNACK:
		return "NACK"
	case TypeControl:
		return "CONTROL"
	case TypeFileMeta:
		return "FILE_META"
	case TypeFileEnd:
		return "FILE_END"
	case TypePing:
		return "PING"
	case TypePong:
		return "PONG"
	default:
		return fmt.Sprintf("UNKNOWN(0x%02x)", f.Type)
	}
}

// NewDataFrame creates a new DATA frame.
func NewDataFrame(seqNum byte, payload []byte) *Frame {
	return &Frame{
		Type:       TypeData,
		SeqNum:     seqNum,
		PayloadLen: uint16(len(payload)),
		Payload:    payload,
	}
}

// NewACKFrame creates a new ACK frame.
func NewACKFrame(seqNum byte) *Frame {
	return &Frame{
		Type:       TypeACK,
		SeqNum:     seqNum,
		PayloadLen: 0,
		Payload:    nil,
	}
}

// NewNACKFrame creates a new NACK frame.
func NewNACKFrame(seqNum byte) *Frame {
	return &Frame{
		Type:       TypeNACK,
		SeqNum:     seqNum,
		PayloadLen: 0,
		Payload:    nil,
	}
}

// NewControlFrame creates a new CONTROL frame.
func NewControlFrame(payload []byte) *Frame {
	return &Frame{
		Type:       TypeControl,
		SeqNum:     0,
		PayloadLen: uint16(len(payload)),
		Payload:    payload,
	}
}

// NewPingFrame creates a new PING frame.
func NewPingFrame() *Frame {
	return &Frame{
		Type:       TypePing,
		SeqNum:     0,
		PayloadLen: 0,
		Payload:    nil,
	}
}

// NewPongFrame creates a new PONG frame.
func NewPongFrame() *Frame {
	return &Frame{
		Type:       TypePong,
		SeqNum:     0,
		PayloadLen: 0,
		Payload:    nil,
	}
}

// Encode serializes the frame to bytes with CRC-32.
func (f *Frame) Encode() []byte {
	totalLen := HeaderSize + int(f.PayloadLen) + CRCSize
	buf := make([]byte, totalLen)

	// Header
	buf[0] = f.Type
	buf[1] = f.SeqNum
	binary.BigEndian.PutUint16(buf[2:4], f.PayloadLen)

	// Payload
	if f.PayloadLen > 0 {
		copy(buf[HeaderSize:], f.Payload[:f.PayloadLen])
	}

	// CRC-32 over header + payload
	dataForCRC := buf[:HeaderSize+int(f.PayloadLen)]
	checksum := fec.CRC32(dataForCRC)
	binary.BigEndian.PutUint32(buf[totalLen-CRCSize:], checksum)

	return buf
}

// DecodeFrame deserializes bytes into a Frame, verifying CRC-32.
func DecodeFrame(data []byte) (*Frame, error) {
	if len(data) < HeaderSize+CRCSize {
		return nil, fmt.Errorf("frame too short: %d bytes", len(data))
	}

	f := &Frame{
		Type:       data[0],
		SeqNum:     data[1],
		PayloadLen: binary.BigEndian.Uint16(data[2:4]),
	}

	expectedLen := HeaderSize + int(f.PayloadLen) + CRCSize
	if len(data) < expectedLen {
		return nil, fmt.Errorf("frame truncated: have %d, need %d", len(data), expectedLen)
	}

	// Verify CRC
	dataForCRC := data[:HeaderSize+int(f.PayloadLen)]
	expectedCRC := binary.BigEndian.Uint32(data[expectedLen-CRCSize : expectedLen])
	actualCRC := fec.CRC32(dataForCRC)

	if expectedCRC != actualCRC {
		return nil, fmt.Errorf("CRC mismatch: expected 0x%08x, got 0x%08x", expectedCRC, actualCRC)
	}

	// Extract payload
	if f.PayloadLen > 0 {
		f.Payload = make([]byte, f.PayloadLen)
		copy(f.Payload, data[HeaderSize:HeaderSize+int(f.PayloadLen)])
	}

	return f, nil
}

// FrameToBytes converts a frame to bytes suitable for OFDM modulation.
// Includes RS encoding for forward error correction.
func FrameToBytes(f *Frame, rsEncoder *fec.RSEncoder) ([]byte, error) {
	raw := f.Encode()

	if rsEncoder != nil {
		encoded, err := rsEncoder.Encode(raw)
		if err != nil {
			return nil, fmt.Errorf("RS encode: %w", err)
		}
		return encoded, nil
	}

	return raw, nil
}

// BytesToFrame decodes bytes from OFDM demodulation back to a frame.
// Includes RS decoding for error correction.
func BytesToFrame(data []byte, rsDecoder *fec.RSEncoder) (*Frame, error) {
	var decoded []byte

	if rsDecoder != nil {
		var err error
		decoded, err = rsDecoder.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("RS decode: %w", err)
		}
	} else {
		decoded = data
	}

	return DecodeFrame(decoded)
}
