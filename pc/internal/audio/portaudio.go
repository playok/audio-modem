package audio

import (
	"fmt"
	"sync"

	"github.com/gordonklaus/portaudio"
)

const (
	SampleRate    = 44100
	FramesPerBuf  = 576 // OFDM symbol length (512 FFT + 64 CP)
	NumChannels   = 1
	SampleFormat  = 32 // float32
)

// AudioIO wraps PortAudio for audio input/output.
type AudioIO struct {
	inputStream  *portaudio.Stream
	outputStream *portaudio.Stream
	inputBuf     []float32
	outputBuf    []float32
	mu           sync.Mutex
	initialized  bool
}

// Init initializes PortAudio.
func Init() error {
	return portaudio.Initialize()
}

// Terminate cleans up PortAudio.
func Terminate() error {
	return portaudio.Terminate()
}

// NewAudioIO creates a new AudioIO instance.
func NewAudioIO() *AudioIO {
	return &AudioIO{
		inputBuf:  make([]float32, FramesPerBuf),
		outputBuf: make([]float32, FramesPerBuf),
	}
}

// OpenInput opens the default input stream.
func (a *AudioIO) OpenInput() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	stream, err := portaudio.OpenDefaultStream(
		NumChannels, // input channels
		0,           // output channels
		float64(SampleRate),
		FramesPerBuf,
		a.inputBuf,
	)
	if err != nil {
		return fmt.Errorf("open input stream: %w", err)
	}
	a.inputStream = stream
	return nil
}

// OpenOutput opens the default output stream.
func (a *AudioIO) OpenOutput() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	stream, err := portaudio.OpenDefaultStream(
		0,           // input channels
		NumChannels, // output channels
		float64(SampleRate),
		FramesPerBuf,
		a.outputBuf,
	)
	if err != nil {
		return fmt.Errorf("open output stream: %w", err)
	}
	a.outputStream = stream
	return nil
}

// OpenDuplex opens a full-duplex stream for simultaneous I/O.
func (a *AudioIO) OpenDuplex() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	inBuf := make([]float32, FramesPerBuf)
	outBuf := make([]float32, FramesPerBuf)
	a.inputBuf = inBuf
	a.outputBuf = outBuf

	// Open separate streams for half-duplex operation
	inStream, err := portaudio.OpenDefaultStream(1, 0, float64(SampleRate), FramesPerBuf, inBuf)
	if err != nil {
		return fmt.Errorf("open input stream: %w", err)
	}
	a.inputStream = inStream

	outStream, err := portaudio.OpenDefaultStream(0, 1, float64(SampleRate), FramesPerBuf, outBuf)
	if err != nil {
		inStream.Close()
		return fmt.Errorf("open output stream: %w", err)
	}
	a.outputStream = outStream
	return nil
}

// StartInput starts the input stream.
func (a *AudioIO) StartInput() error {
	if a.inputStream == nil {
		return fmt.Errorf("input stream not opened")
	}
	return a.inputStream.Start()
}

// StartOutput starts the output stream.
func (a *AudioIO) StartOutput() error {
	if a.outputStream == nil {
		return fmt.Errorf("output stream not opened")
	}
	return a.outputStream.Start()
}

// Read reads samples from the input stream.
func (a *AudioIO) Read() ([]float32, error) {
	if a.inputStream == nil {
		return nil, fmt.Errorf("input stream not opened")
	}
	err := a.inputStream.Read()
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	out := make([]float32, len(a.inputBuf))
	copy(out, a.inputBuf)
	return out, nil
}

// Write writes samples to the output stream.
func (a *AudioIO) Write(samples []float32) error {
	if a.outputStream == nil {
		return fmt.Errorf("output stream not opened")
	}
	copy(a.outputBuf, samples)
	return a.outputStream.Write()
}

// WriteSamples writes a large buffer of samples in FramesPerBuf chunks.
func (a *AudioIO) WriteSamples(samples []float32) error {
	for i := 0; i < len(samples); i += FramesPerBuf {
		end := i + FramesPerBuf
		if end > len(samples) {
			// Pad with zeros
			chunk := make([]float32, FramesPerBuf)
			copy(chunk, samples[i:])
			if err := a.Write(chunk); err != nil {
				return err
			}
		} else {
			if err := a.Write(samples[i:end]); err != nil {
				return err
			}
		}
	}
	return nil
}

// ReadSamples reads n samples from the input stream.
func (a *AudioIO) ReadSamples(n int) ([]float32, error) {
	result := make([]float32, 0, n)
	for len(result) < n {
		chunk, err := a.Read()
		if err != nil {
			return nil, err
		}
		result = append(result, chunk...)
	}
	return result[:n], nil
}

// StopInput stops the input stream.
func (a *AudioIO) StopInput() error {
	if a.inputStream == nil {
		return nil
	}
	return a.inputStream.Stop()
}

// StopOutput stops the output stream.
func (a *AudioIO) StopOutput() error {
	if a.outputStream == nil {
		return nil
	}
	return a.outputStream.Stop()
}

// Close closes all streams.
func (a *AudioIO) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var errs []error
	if a.inputStream != nil {
		if err := a.inputStream.Close(); err != nil {
			errs = append(errs, err)
		}
		a.inputStream = nil
	}
	if a.outputStream != nil {
		if err := a.outputStream.Close(); err != nil {
			errs = append(errs, err)
		}
		a.outputStream = nil
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}
