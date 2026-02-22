package protocol

import (
	"fmt"
	"log"
	"time"

	"github.com/jeongseonghan/audio-modem/internal/audio"
	"github.com/jeongseonghan/audio-modem/internal/fec"
	"github.com/jeongseonghan/audio-modem/internal/modem"
)

// SessionMode represents the operating mode.
type SessionMode int

const (
	ModeSend    SessionMode = iota
	ModeReceive
)

// SessionStatus represents the session state.
type SessionStatus int

const (
	StatusDisconnected SessionStatus = iota
	StatusConnecting
	StatusConnected
	StatusTransferring
	StatusCompleted
	StatusError
)

// String returns the status name.
func (s SessionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "disconnected"
	case StatusConnecting:
		return "connecting"
	case StatusConnected:
		return "connected"
	case StatusTransferring:
		return "transferring"
	case StatusCompleted:
		return "completed"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// SessionEvent is sent to listeners when session state changes.
type SessionEvent struct {
	Status   SessionStatus
	Message  string
	Progress float64 // 0.0 to 1.0
	Error    error
}

// Session manages an audio modem communication session.
type Session struct {
	audioIO     *audio.AudioIO
	modulator   *modem.Modulator
	demodulator *modem.Demodulator
	rsEncoder   *fec.RSEncoder
	transport   *Transport
	modulation  modem.Modulation

	status    SessionStatus
	eventChan chan SessionEvent

	// preamble generator for frame transmission
	preambleGen *modem.PreambleGenerator
}

// NewSession creates a new communication session.
func NewSession(mod modem.Modulation) (*Session, error) {
	rsEnc, err := fec.NewRSEncoder()
	if err != nil {
		return nil, fmt.Errorf("create RS encoder: %w", err)
	}

	s := &Session{
		audioIO:     audio.NewAudioIO(),
		modulator:   modem.NewModulator(mod),
		demodulator: modem.NewDemodulator(mod),
		rsEncoder:   rsEnc,
		modulation:  mod,
		eventChan:   make(chan SessionEvent, 100),
		preambleGen: modem.NewPreambleGenerator(42),
	}

	// Create transport with sender/receiver callbacks
	s.transport = NewTransport(s.sendFrame, s.receiveFrame)

	return s, nil
}

// Open initializes the audio I/O.
func (s *Session) Open() error {
	s.setStatus(StatusConnecting, "Opening audio devices...")

	if err := s.audioIO.OpenDuplex(); err != nil {
		s.setStatus(StatusError, fmt.Sprintf("Audio open failed: %v", err))
		return err
	}

	s.setStatus(StatusConnected, "Audio devices ready")
	return nil
}

// Close releases all resources.
func (s *Session) Close() error {
	s.setStatus(StatusDisconnected, "Session closed")
	return s.audioIO.Close()
}

// Events returns the event channel for monitoring session state.
func (s *Session) Events() <-chan SessionEvent {
	return s.eventChan
}

// Transport returns the transport layer for file transfer operations.
func (s *Session) Transport() *Transport {
	return s.transport
}

// sendFrame modulates and transmits a protocol frame.
func (s *Session) sendFrame(frame *Frame) error {
	// Encode frame to bytes
	frameBytes := frame.Encode()

	// RS encode
	encoded, err := s.rsEncoder.Encode(frameBytes)
	if err != nil {
		return fmt.Errorf("RS encode: %w", err)
	}

	// Generate OFDM signal
	signal := modem.GenerateFrame(encoded, s.modulation)

	// Convert to float32 and transmit
	samples32 := modem.SamplesToFloat32(signal)

	if err := s.audioIO.StartOutput(); err != nil {
		return fmt.Errorf("start output: %w", err)
	}
	defer s.audioIO.StopOutput()

	return s.audioIO.WriteSamples(samples32)
}

// receiveFrame receives and demodulates a protocol frame.
func (s *Session) receiveFrame(timeout time.Duration) (*Frame, error) {
	if err := s.audioIO.StartInput(); err != nil {
		return nil, fmt.Errorf("start input: %w", err)
	}
	defer s.audioIO.StopInput()

	// Calculate expected number of samples
	// Preamble (2 symbols) + channel est (1 symbol) + at least 1 data symbol
	minSamples := 4 * modem.SymbolLen
	// Read extra samples for detection margin
	totalSamples := minSamples + 10*modem.SymbolLen

	deadline := time.Now().Add(timeout)
	var allSamples []float64

	for time.Now().Before(deadline) {
		samples32, err := s.audioIO.Read()
		if err != nil {
			return nil, fmt.Errorf("read audio: %w", err)
		}
		allSamples = append(allSamples, modem.Float32ToSamples(samples32)...)

		if len(allSamples) >= totalSamples {
			break
		}
	}

	if len(allSamples) < minSamples {
		return nil, fmt.Errorf("timeout: insufficient samples (%d < %d)", len(allSamples), minSamples)
	}

	// Apply DC removal and AGC
	allSamples = modem.ApplyDCRemoval(allSamples)
	allSamples = modem.ApplyAGC(allSamples, 0.3)

	// Demodulate
	bitsPerSym := modem.BitsPerOFDMSymbol(s.modulation)
	data, err := modem.ReceiveFrame(allSamples, s.modulation, bitsPerSym)
	if err != nil {
		return nil, fmt.Errorf("demodulate: %w", err)
	}

	// RS decode
	decoded, err := s.rsEncoder.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("RS decode: %w", err)
	}

	// Parse frame
	frame, err := DecodeFrame(decoded)
	if err != nil {
		return nil, fmt.Errorf("decode frame: %w", err)
	}

	return frame, nil
}

func (s *Session) setStatus(status SessionStatus, message string) {
	s.status = status
	event := SessionEvent{
		Status:  status,
		Message: message,
	}
	select {
	case s.eventChan <- event:
	default:
		log.Printf("Event channel full, dropping: %s - %s", status, message)
	}
}
