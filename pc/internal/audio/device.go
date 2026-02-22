package audio

import (
	"fmt"

	"github.com/gordonklaus/portaudio"
)

// DeviceInfo holds audio device information.
type DeviceInfo struct {
	Name              string
	MaxInputChannels  int
	MaxOutputChannels int
	DefaultSampleRate float64
	IsDefault         bool
}

// ListDevices returns all available audio devices.
func ListDevices() ([]DeviceInfo, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	var defaultInName, defaultOutName string
	if d, err := portaudio.DefaultInputDevice(); err == nil {
		defaultInName = d.Name
	}
	if d, err := portaudio.DefaultOutputDevice(); err == nil {
		defaultOutName = d.Name
	}

	var result []DeviceInfo
	for _, d := range devices {
		isDefault := (d.Name == defaultInName) || (d.Name == defaultOutName)
		result = append(result, DeviceInfo{
			Name:              d.Name,
			MaxInputChannels:  d.MaxInputChannels,
			MaxOutputChannels: d.MaxOutputChannels,
			DefaultSampleRate: d.DefaultSampleRate,
			IsDefault:         isDefault,
		})
	}
	return result, nil
}

// HasInputDevice returns true if a default input device is available.
func HasInputDevice() bool {
	_, err := portaudio.DefaultInputDevice()
	return err == nil
}

// HasOutputDevice returns true if a default output device is available.
func HasOutputDevice() bool {
	_, err := portaudio.DefaultOutputDevice()
	return err == nil
}

// PrintDevices prints all available audio devices.
func PrintDevices() error {
	devices, err := ListDevices()
	if err != nil {
		return err
	}
	fmt.Println("Audio Devices:")
	if len(devices) == 0 {
		fmt.Println("  (no devices found)")
		return nil
	}
	for i, d := range devices {
		defaultStr := ""
		if d.IsDefault {
			defaultStr = " [DEFAULT]"
		}
		fmt.Printf("  %d: %s (in:%d out:%d rate:%.0f)%s\n",
			i, d.Name, d.MaxInputChannels, d.MaxOutputChannels,
			d.DefaultSampleRate, defaultStr)
	}

	if !HasInputDevice() {
		fmt.Println("\n  WARNING: No default input device. Receive mode unavailable.")
	}
	if !HasOutputDevice() {
		fmt.Println("\n  WARNING: No default output device. Send mode unavailable.")
	}
	return nil
}
