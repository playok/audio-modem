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
	ModeDuplex
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
	mode        SessionMode

	status    SessionStatus
	eventChan chan SessionEvent

	preambleGen *modem.PreambleGenerator
	hasInput    bool
	hasOutput   bool
}

// NewSession creates a new communication session.
func NewSession(mod modem.Modulation, mode SessionMode) (*Session, error) {
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
		mode:        mode,
		eventChan:   make(chan SessionEvent, 100),
		preambleGen: modem.NewPreambleGenerator(42),
	}

	s.transport = NewTransport(s.sendFrame, s.receiveFrame)

	return s, nil
}

// Open initializes the audio I/O based on the session mode.
func (s *Session) Open() error {
	s.setStatus(StatusConnecting, "Opening audio devices...")

	switch s.mode {
	case ModeSend:
		// Send mode: need output (required) + input (optional, for ACK)
		if err := s.audioIO.OpenOutput(); err != nil {
			s.setStatus(StatusError, fmt.Sprintf("Audio output open failed: %v", err))
			return err
		}
		s.hasOutput = true

		if err := s.audioIO.OpenInput(); err != nil {
			log.Printf("Warning: No input device available. ACK reception disabled: %v", err)
			s.hasInput = false
		} else {
			s.hasInput = true
		}

	case ModeReceive:
		// Receive mode: need input (required) + output (optional, for ACK)
		if err := s.audioIO.OpenInput(); err != nil {
			s.setStatus(StatusError, fmt.Sprintf("Audio input open failed: %v", err))
			return err
		}
		s.hasInput = true

		if err := s.audioIO.OpenOutput(); err != nil {
			log.Printf("Warning: No output device available. ACK sending disabled: %v", err)
			s.hasOutput = false
		} else {
			s.hasOutput = true
		}

	case ModeDuplex:
		// Need both
		if err := s.audioIO.OpenDuplex(); err != nil {
			s.setStatus(StatusError, fmt.Sprintf("Audio open failed: %v", err))
			return err
		}
		s.hasInput = true
		s.hasOutput = true
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
	if !s.hasOutput {
		return fmt.Errorf("no output device available")
	}

	frameBytes := frame.Encode()

	encoded, err := s.rsEncoder.Encode(frameBytes)
	if err != nil {
		return fmt.Errorf("RS encode: %w", err)
	}

	signal := modem.GenerateFrame(encoded, s.modulation)
	samples32 := modem.SamplesToFloat32(signal)

	if err := s.audioIO.StartOutput(); err != nil {
		return fmt.Errorf("start output: %w", err)
	}
	defer s.audioIO.StopOutput()

	return s.audioIO.WriteSamples(samples32)
}

// receiveFrame receives and demodulates a protocol frame.
func (s *Session) receiveFrame(timeout time.Duration) (*Frame, error) {
	if !s.hasInput {
		return nil, fmt.Errorf("no input device available")
	}

	if err := s.audioIO.StartInput(); err != nil {
		return nil, fmt.Errorf("start input: %w", err)
	}
	defer s.audioIO.StopInput()

	minSamples := 4 * modem.SymbolLen
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

	allSamples = modem.ApplyDCRemoval(allSamples)
	allSamples = modem.ApplyAGC(allSamples, 0.3)

	bitsPerSym := modem.BitsPerOFDMSymbol(s.modulation)
	data, err := modem.ReceiveFrame(allSamples, s.modulation, bitsPerSym)
	if err != nil {
		return nil, fmt.Errorf("demodulate: %w", err)
	}

	decoded, err := s.rsEncoder.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("RS decode: %w", err)
	}

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
