package server

import (
	"fmt"
	"log"
	"net/http"
)

// Server is the HTTP server for the web interface.
type Server struct {
	mux       *http.ServeMux
	handler   *Handlers
	addr      string
	staticDir string
}

// NewServer creates a new HTTP server.
func NewServer(addr string, handler *Handlers, staticDir string) *Server {
	s := &Server{
		mux:       http.NewServeMux(),
		handler:   handler,
		addr:      addr,
		staticDir: staticDir,
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// API routes
	s.mux.HandleFunc("/api/upload", s.handler.HandleUpload)
	s.mux.HandleFunc("/api/send", s.handler.HandleSend)
	s.mux.HandleFunc("/api/receive/start", s.handler.HandleReceiveStart)
	s.mux.HandleFunc("/api/status", s.handler.HandleStatus)
	s.mux.HandleFunc("/api/devices", s.handler.HandleDevices)
	s.mux.HandleFunc("/api/download/", s.handler.HandleDownload)

	// WebSocket
	s.mux.HandleFunc("/ws", s.handler.HandleWebSocket)

	// Static files
	s.mux.Handle("/", http.FileServer(http.Dir(s.staticDir)))
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	log.Printf("Starting server on %s", s.addr)
	fmt.Printf("\n  Audio Modem Server running at http://%s\n\n", s.addr)
	return http.ListenAndServe(s.addr, s.mux)
}
