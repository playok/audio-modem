package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeongseonghan/audio-modem/internal/audio"
	"github.com/jeongseonghan/audio-modem/internal/server"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "Server address")
	uploadDir := flag.String("upload-dir", "./uploads", "Upload directory")
	receiveDir := flag.String("receive-dir", "./received", "Receive directory")
	listDevices := flag.Bool("list-devices", false, "List audio devices and exit")
	flag.Parse()

	// Initialize PortAudio
	if err := audio.Init(); err != nil {
		log.Fatalf("Failed to initialize PortAudio: %v", err)
	}
	defer audio.Terminate()

	if *listDevices {
		if err := audio.PrintDevices(); err != nil {
			log.Fatalf("Failed to list devices: %v", err)
		}
		return
	}

	// Create directories
	os.MkdirAll(*uploadDir, 0755)
	os.MkdirAll(*receiveDir, 0755)

	// Create handlers and server
	handlers := server.NewHandlers(*uploadDir, *receiveDir)
	srv := server.NewServer(*addr, handlers, "./web/static")

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		audio.Terminate()
		os.Exit(0)
	}()

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
