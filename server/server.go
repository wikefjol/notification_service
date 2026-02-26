// Package server implements the notify-server daemon.
package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Server is the HTTP server for handling notification requests.
type Server struct {
	config      *Config
	httpServer  *http.Server
	logger      *slog.Logger
	replayCache *ReplayCache
}

// NotifyRequest is the expected JSON payload for POST /notify.
type NotifyRequest struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg *Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize replay cache with TTL = 2 * max_skew_seconds
	ttl := time.Duration(cfg.MaxSkewSeconds*2) * time.Second
	replayCache := NewReplayCache(cfg.ReplayCacheMaxEntries, ttl)

	s := &Server{
		config:      cfg,
		logger:      logger,
		replayCache: replayCache,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /notify", s.handleNotify)
	// Catch-all for /notify with wrong method
	mux.HandleFunc("/notify", s.handleNotifyMethodNotAllowed)

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return s
}

// Start begins listening for requests. It blocks until the server is shut down.
func (s *Server) Start() error {
	s.logger.Info("server starting", "addr", s.config.ListenAddr)
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// handleHealthz responds with 200 OK for health checks.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleNotifyMethodNotAllowed handles non-POST requests to /notify.
func (s *Server) handleNotifyMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleNotify processes notification requests.
func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	// Validate Content-Type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		s.logger.Warn("invalid content-type",
			"content_type", contentType,
			"remote_addr", r.RemoteAddr,
		)
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	// Limit body size
	r.Body = http.MaxBytesReader(w, r.Body, int64(s.config.MaxBodyBytes))

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// MaxBytesReader returns a specific error when limit exceeded
		if err.Error() == "http: request body too large" {
			s.logger.Warn("request body too large",
				"max_bytes", s.config.MaxBodyBytes,
				"remote_addr", r.RemoteAddr,
			)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		s.logger.Warn("failed to read request body",
			"error", err,
			"remote_addr", r.RemoteAddr,
		)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Parse JSON
	var req NotifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.logger.Warn("invalid json",
			"error", err,
			"remote_addr", r.RemoteAddr,
		)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Authenticate request
	if err := s.authenticate(r, body, req.Source); err != nil {
		// All auth errors return 401 with empty body
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// TODO: Rate limiting (issue #3)
	// TODO: Sound playback (issue #6)

	s.logger.Info("notification received",
		"source", req.Source,
		"message_length", len(req.Message),
	)

	w.WriteHeader(http.StatusNoContent)
}
