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

	defaultIn, err := portaudio.DefaultInputDevice()
	if err != nil {
		return nil, fmt.Errorf("default input device: %w", err)
	}
	defaultOut, err := portaudio.DefaultOutputDevice()
	if err != nil {
		return nil, fmt.Errorf("default output device: %w", err)
	}

	var result []DeviceInfo
	for _, d := range devices {
		isDefault := (d.Name == defaultIn.Name) || (d.Name == defaultOut.Name)
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

// PrintDevices prints all available audio devices.
func PrintDevices() error {
	devices, err := ListDevices()
	if err != nil {
		return err
	}
	fmt.Println("Audio Devices:")
	for i, d := range devices {
		defaultStr := ""
		if d.IsDefault {
			defaultStr = " [DEFAULT]"
		}
		fmt.Printf("  %d: %s (in:%d out:%d rate:%.0f)%s\n",
			i, d.Name, d.MaxInputChannels, d.MaxOutputChannels,
			d.DefaultSampleRate, defaultStr)
	}
	return nil
}
