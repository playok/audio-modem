package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jeongseonghan/audio-modem/internal/audio"
	"github.com/jeongseonghan/audio-modem/internal/modem"
	"github.com/jeongseonghan/audio-modem/internal/protocol"
)

// Handlers holds the HTTP API handlers.
type Handlers struct {
	session    *protocol.Session
	wsHub      *WSHub
	uploadDir  string
	receiveDir string
	mu         sync.Mutex
}

// NewHandlers creates new API handlers.
func NewHandlers(uploadDir, receiveDir string) *Handlers {
	return &Handlers{
		wsHub:      NewWSHub(),
		uploadDir:  uploadDir,
		receiveDir: receiveDir,
	}
}

// HandleWebSocket handles WebSocket upgrade requests.
func (h *Handlers) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	h.wsHub.AddClient(conn)

	// Read messages (for potential commands from client)
	go func() {
		defer h.wsHub.RemoveClient(conn)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

// HandleUpload handles file upload for sending.
func (h *Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, fmt.Sprintf("Parse form: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("Get file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save to upload directory
	os.MkdirAll(h.uploadDir, 0755)
	outPath := filepath.Join(h.uploadDir, header.Filename)
	outFile, err := os.Create(outPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Create file: %v", err), http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, file)
	if err != nil {
		http.Error(w, fmt.Sprintf("Save file: %v", err), http.StatusInternalServerError)
		return
	}

	h.wsHub.BroadcastLog("info", fmt.Sprintf("File uploaded: %s (%d bytes)", header.Filename, written))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"filename": header.Filename,
		"size":     written,
		"status":   "uploaded",
	})
}

// HandleSend initiates file sending.
func (h *Handlers) HandleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename   string `json:"filename"`
		Modulation string `json:"modulation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Parse request: %v", err), http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(h.uploadDir, req.Filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Determine modulation
	mod := modem.Mod16QAM
	switch strings.ToUpper(req.Modulation) {
	case "QPSK":
		mod = modem.ModQPSK
	case "64-QAM", "64QAM":
		mod = modem.Mod64QAM
	}

	// Start sending in background
	go func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		session, err := protocol.NewSession(mod, protocol.ModeSend)
		if err != nil {
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Session create failed: %v", err))
			return
		}
		h.session = session
		defer session.Close()

		if err := session.Open(); err != nil {
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Audio open failed: %v", err))
			return
		}

		h.wsHub.BroadcastStatus("connecting", "Performing handshake...")

		// Handshake
		if err := session.Transport().Handshake(); err != nil {
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Handshake failed: %v", err))
			return
		}

		h.wsHub.BroadcastStatus("transferring", "Sending file...")

		// Send file
		sender := protocol.NewFileSender(session.Transport())
		sender.SetProgressCallback(func(sent, total int64, status string) {
			progress := float64(sent) / float64(total)
			h.wsHub.BroadcastProgress("transferring", status, progress, sent, total)
		})

		if err := sender.SendFile(filePath); err != nil {
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Send failed: %v", err))
			return
		}

		h.wsHub.BroadcastStatus("completed", "File sent successfully!")
	}()

	json.NewEncoder(w).Encode(map[string]string{
		"status": "sending",
	})
}

// HandleReceiveStart starts receiving mode.
func (h *Handlers) HandleReceiveStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Modulation string `json:"modulation"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	mod := modem.Mod16QAM
	switch strings.ToUpper(req.Modulation) {
	case "QPSK":
		mod = modem.ModQPSK
	case "64-QAM", "64QAM":
		mod = modem.Mod64QAM
	}

	go func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		session, err := protocol.NewSession(mod, protocol.ModeReceive)
		if err != nil {
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Session create failed: %v", err))
			return
		}
		h.session = session
		defer session.Close()

		if err := session.Open(); err != nil {
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Audio open failed: %v", err))
			return
		}

		h.wsHub.BroadcastStatus("connecting", "Waiting for handshake...")

		// Wait for handshake
		if err := session.Transport().WaitForHandshake(30 * 1000000000); err != nil { // 30 seconds
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Handshake failed: %v", err))
			return
		}

		h.wsHub.BroadcastStatus("transferring", "Receiving file...")

		// Receive file
		os.MkdirAll(h.receiveDir, 0755)
		receiver := protocol.NewFileReceiver(session.Transport(), h.receiveDir)
		receiver.SetProgressCallback(func(received, total int64, status string) {
			progress := float64(received) / float64(total)
			h.wsHub.BroadcastProgress("transferring", status, progress, received, total)
		})

		meta, err := receiver.ReceiveFile(60 * 1000000000) // 60 second timeout
		if err != nil {
			h.wsHub.BroadcastStatus("error", fmt.Sprintf("Receive failed: %v", err))
			return
		}

		h.wsHub.BroadcastStatus("completed", fmt.Sprintf("File received: %s (%d bytes)", meta.Filename, meta.Size))
	}()

	json.NewEncoder(w).Encode(map[string]string{
		"status": "receiving",
	})
}

// HandleStatus returns current session status.
func (h *Handlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	status := "idle"
	if h.session != nil {
		status = "active"
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
	})
}

// HandleDevices lists available audio devices.
func (h *Handlers) HandleDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := audio.ListDevices()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"devices":   devices,
		"hasInput":  audio.HasInputDevice(),
		"hasOutput": audio.HasOutputDevice(),
	})
}

// HandleDownload serves received files for download.
func (h *Handlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if filename == "" {
		http.Error(w, "Filename required", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(h.receiveDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeFile(w, r, filePath)
}
