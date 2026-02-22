package fec

import (
	"fmt"

	"github.com/klauspost/reedsolomon"
)

// RSEncoder wraps Reed-Solomon encoding/decoding.
// Uses RS(255,223) - 223 data shards, 32 parity shards.
type RSEncoder struct {
	enc        reedsolomon.Encoder
	dataShards int
	parShards  int
}

const (
	DefaultDataShards   = 223
	DefaultParityShards = 32
)

// NewRSEncoder creates a new Reed-Solomon encoder.
func NewRSEncoder() (*RSEncoder, error) {
	return NewRSEncoderCustom(DefaultDataShards, DefaultParityShards)
}

// NewRSEncoderCustom creates a Reed-Solomon encoder with custom shard counts.
func NewRSEncoderCustom(dataShards, parityShards int) (*RSEncoder, error) {
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("create reed-solomon encoder: %w", err)
	}
	return &RSEncoder{
		enc:        enc,
		dataShards: dataShards,
		parShards:  parityShards,
	}, nil
}

// Encode adds Reed-Solomon parity to the data.
// Input: raw data bytes
// Output: data + parity bytes
func (rs *RSEncoder) Encode(data []byte) ([]byte, error) {
	totalShards := rs.dataShards + rs.parShards

	// Split data into shards
	shards, err := rs.splitData(data)
	if err != nil {
		return nil, err
	}

	// Encode parity
	err = rs.enc.Encode(shards)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	// Concatenate all shards
	result := make([]byte, 0, totalShards*len(shards[0]))
	for _, shard := range shards {
		result = append(result, shard...)
	}

	return result, nil
}

// Decode recovers the original data from encoded data (with possible errors).
// Input: encoded data (data + parity), with possible corrupted bytes (set to 0)
// Output: recovered original data
func (rs *RSEncoder) Decode(encoded []byte) ([]byte, error) {
	shards, err := rs.splitEncoded(encoded)
	if err != nil {
		return nil, err
	}

	// Reconstruct
	err = rs.enc.Reconstruct(shards)
	if err != nil {
		return nil, fmt.Errorf("reconstruct: %w", err)
	}

	// Verify
	ok, err := rs.enc.Verify(shards)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("verification failed: data may be corrupted beyond repair")
	}

	// Extract data shards
	var result []byte
	for i := 0; i < rs.dataShards; i++ {
		result = append(result, shards[i]...)
	}

	return result, nil
}

// EncodeBlock encodes a single block of data.
// Simpler interface: takes up to dataShards bytes and returns encoded block.
func (rs *RSEncoder) EncodeBlock(data []byte) ([]byte, error) {
	if len(data) > rs.dataShards {
		return nil, fmt.Errorf("data too large: %d > %d", len(data), rs.dataShards)
	}

	// Pad data if needed
	padded := make([]byte, rs.dataShards)
	copy(padded, data)

	// Create shards (1 byte per shard for simplicity)
	totalShards := rs.dataShards + rs.parShards
	shards := make([][]byte, totalShards)
	for i := 0; i < rs.dataShards; i++ {
		shards[i] = []byte{padded[i]}
	}
	for i := rs.dataShards; i < totalShards; i++ {
		shards[i] = make([]byte, 1)
	}

	err := rs.enc.Encode(shards)
	if err != nil {
		return nil, fmt.Errorf("encode block: %w", err)
	}

	result := make([]byte, totalShards)
	for i, s := range shards {
		result[i] = s[0]
	}
	return result, nil
}

// DecodeBlock decodes a single encoded block.
func (rs *RSEncoder) DecodeBlock(block []byte, erasures []int) ([]byte, error) {
	totalShards := rs.dataShards + rs.parShards
	if len(block) != totalShards {
		return nil, fmt.Errorf("invalid block size: %d != %d", len(block), totalShards)
	}

	shards := make([][]byte, totalShards)
	for i := 0; i < totalShards; i++ {
		shards[i] = []byte{block[i]}
	}

	// Mark erasures
	for _, idx := range erasures {
		if idx < totalShards {
			shards[idx] = nil
		}
	}

	err := rs.enc.Reconstruct(shards)
	if err != nil {
		return nil, fmt.Errorf("reconstruct block: %w", err)
	}

	result := make([]byte, rs.dataShards)
	for i := 0; i < rs.dataShards; i++ {
		if shards[i] != nil {
			result[i] = shards[i][0]
		}
	}
	return result, nil
}

func (rs *RSEncoder) splitData(data []byte) ([][]byte, error) {
	totalShards := rs.dataShards + rs.parShards
	shardSize := (len(data) + rs.dataShards - 1) / rs.dataShards

	shards := make([][]byte, totalShards)
	for i := 0; i < rs.dataShards; i++ {
		shards[i] = make([]byte, shardSize)
		start := i * shardSize
		end := start + shardSize
		if start < len(data) {
			if end > len(data) {
				end = len(data)
			}
			copy(shards[i], data[start:end])
		}
	}
	for i := rs.dataShards; i < totalShards; i++ {
		shards[i] = make([]byte, shardSize)
	}

	return shards, nil
}

func (rs *RSEncoder) splitEncoded(encoded []byte) ([][]byte, error) {
	totalShards := rs.dataShards + rs.parShards
	shardSize := len(encoded) / totalShards
	if len(encoded)%totalShards != 0 {
		return nil, fmt.Errorf("encoded data size %d not divisible by %d shards", len(encoded), totalShards)
	}

	shards := make([][]byte, totalShards)
	for i := 0; i < totalShards; i++ {
		shards[i] = make([]byte, shardSize)
		copy(shards[i], encoded[i*shardSize:(i+1)*shardSize])
	}
	return shards, nil
}

// DataShards returns the number of data shards.
func (rs *RSEncoder) DataShards() int { return rs.dataShards }

// ParityShards returns the number of parity shards.
func (rs *RSEncoder) ParityShards() int { return rs.parShards }
