package protocol

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Transport configuration
const (
	ACKTimeout      = 500 * time.Millisecond
	MaxRetries      = 3
	TurnAroundDelay = 50 * time.Millisecond // half-duplex switch delay
)

// TransportState represents the ARQ state machine.
type TransportState int

const (
	StateIdle TransportState = iota
	StateSending
	StateWaitingACK
	StateReceiving
)

// String returns the state name.
func (s TransportState) String() string {
	switch s {
	case StateIdle:
		return "IDLE"
	case StateSending:
		return "SENDING"
	case StateWaitingACK:
		return "WAITING_ACK"
	case StateReceiving:
		return "RECEIVING"
	default:
		return "UNKNOWN"
	}
}

// FrameSender is a function that modulates and transmits a frame.
type FrameSender func(frame *Frame) error

// FrameReceiver is a function that receives and demodulates a frame.
type FrameReceiver func(timeout time.Duration) (*Frame, error)

// Transport implements Stop-and-Wait ARQ for reliable frame delivery.
type Transport struct {
	sender   FrameSender
	receiver FrameReceiver
	state    TransportState
	seqNum   byte
	mu       sync.Mutex

	// Stats
	framesSent     int
	framesReceived int
	retries        int
	errors         int

	// Callbacks
	OnStateChange func(state TransportState)
	OnProgress    func(sent, total int)
}

// NewTransport creates a new transport layer.
func NewTransport(sender FrameSender, receiver FrameReceiver) *Transport {
	return &Transport{
		sender:   sender,
		receiver: receiver,
		state:    StateIdle,
	}
}

// SendFrame sends a frame and waits for ACK (Stop-and-Wait ARQ).
func (t *Transport) SendFrame(frame *Frame) error {
	t.mu.Lock()
	frame.SeqNum = t.seqNum
	t.mu.Unlock()

	for retry := 0; retry <= MaxRetries; retry++ {
		if retry > 0 {
			log.Printf("Retry %d/%d for frame seq=%d type=%s",
				retry, MaxRetries, frame.SeqNum, frame.TypeName())
			t.retries++
		}

		t.setState(StateSending)

		// Send frame
		err := t.sender(frame)
		if err != nil {
			t.errors++
			return fmt.Errorf("send frame: %w", err)
		}
		t.framesSent++

		// Wait for turnaround
		time.Sleep(TurnAroundDelay)
		t.setState(StateWaitingACK)

		// Wait for ACK
		ackFrame, err := t.receiver(ACKTimeout)
		if err != nil {
			log.Printf("ACK timeout for seq=%d: %v", frame.SeqNum, err)
			continue
		}

		if ackFrame.Type == TypeACK && ackFrame.SeqNum == frame.SeqNum {
			// Success
			t.mu.Lock()
			t.seqNum++
			t.mu.Unlock()
			t.setState(StateIdle)
			return nil
		}

		if ackFrame.Type == TypeNACK {
			log.Printf("NACK received for seq=%d", frame.SeqNum)
			continue
		}

		log.Printf("Unexpected response: type=%s seq=%d (expected ACK seq=%d)",
			ackFrame.TypeName(), ackFrame.SeqNum, frame.SeqNum)
	}

	t.errors++
	t.setState(StateIdle)
	return fmt.Errorf("max retries exceeded for frame seq=%d", frame.SeqNum)
}

// ReceiveFrame waits for and receives a data frame, sending ACK/NACK.
func (t *Transport) ReceiveFrame(timeout time.Duration) (*Frame, error) {
	t.setState(StateReceiving)

	frame, err := t.receiver(timeout)
	if err != nil {
		t.setState(StateIdle)
		return nil, fmt.Errorf("receive: %w", err)
	}
	t.framesReceived++

	// Send ACK after turnaround delay
	time.Sleep(TurnAroundDelay)
	t.setState(StateSending)

	ack := NewACKFrame(frame.SeqNum)
	if err := t.sender(ack); err != nil {
		log.Printf("Failed to send ACK for seq=%d: %v", frame.SeqNum, err)
	}

	t.setState(StateIdle)
	return frame, nil
}

// SendControlFrame sends a control frame without ARQ (fire-and-forget for simple exchanges).
func (t *Transport) SendControlFrame(frame *Frame) error {
	t.setState(StateSending)
	err := t.sender(frame)
	t.setState(StateIdle)
	return err
}

// Handshake performs a PING/PONG handshake to verify connectivity.
func (t *Transport) Handshake() error {
	// Send PING
	ping := NewPingFrame()
	if err := t.sender(ping); err != nil {
		return fmt.Errorf("send PING: %w", err)
	}

	time.Sleep(TurnAroundDelay)

	// Wait for PONG
	pong, err := t.receiver(2 * ACKTimeout)
	if err != nil {
		return fmt.Errorf("PONG timeout: %w", err)
	}

	if pong.Type != TypePong {
		return fmt.Errorf("expected PONG, got %s", pong.TypeName())
	}

	log.Println("Handshake successful")
	return nil
}

// WaitForHandshake waits for an incoming PING and responds with PONG.
func (t *Transport) WaitForHandshake(timeout time.Duration) error {
	ping, err := t.receiver(timeout)
	if err != nil {
		return fmt.Errorf("waiting for PING: %w", err)
	}

	if ping.Type != TypePing {
		return fmt.Errorf("expected PING, got %s", ping.TypeName())
	}

	time.Sleep(TurnAroundDelay)

	pong := NewPongFrame()
	if err := t.sender(pong); err != nil {
		return fmt.Errorf("send PONG: %w", err)
	}

	log.Println("Handshake received and responded")
	return nil
}

func (t *Transport) setState(state TransportState) {
	t.mu.Lock()
	t.state = state
	t.mu.Unlock()

	if t.OnStateChange != nil {
		t.OnStateChange(state)
	}
}

// Stats returns transport statistics.
func (t *Transport) Stats() (sent, received, retries, errors int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.framesSent, t.framesReceived, t.retries, t.errors
}

// Reset resets the transport state and sequence number.
func (t *Transport) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = StateIdle
	t.seqNum = 0
	t.framesSent = 0
	t.framesReceived = 0
	t.retries = 0
	t.errors = 0
}
