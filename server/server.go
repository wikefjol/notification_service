// Package server implements the notify-server daemon.
package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"
)

// Server is the HTTP server for handling notification requests.
type Server struct {
	config      *Config
	httpServer  *http.Server
	logger      *slog.Logger
	replayCache *ReplayCache
	rateLimiter *RateLimiter
	soundPlayer *SoundPlayer
}

// NotifyRequest is the expected JSON payload for POST /notify.
type NotifyRequest struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

// NewServer creates a new Server with the given configuration.
// The provided context is used as the base context for cancellation on shutdown.
func NewServer(cfg *Config, logger *slog.Logger, ctx context.Context) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Initialize replay cache with TTL = 2 * max_skew_seconds
	ttl := time.Duration(cfg.MaxSkewSeconds*2) * time.Second
	replayCache := NewReplayCache(cfg.ReplayCacheMaxEntries, ttl)

	// Initialize rate limiter
	rateLimiter := NewRateLimiter(cfg.RateLimitPerMinute, cfg.RateLimitBurst)

	// Initialize sound player with base context
	soundPlayer := NewSoundPlayer(cfg, logger, ctx)

	s := &Server{
		config:      cfg,
		logger:      logger,
		replayCache: replayCache,
		rateLimiter: rateLimiter,
		soundPlayer: soundPlayer,
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

// StopSounds cancels all in-flight sound playback and waits for completion.
// This should be called after Shutdown() during graceful shutdown.
func (s *Server) StopSounds() {
	s.soundPlayer.Stop()
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
	// Validate Content-Type (accept charset parameters per RFC 7231)
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
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

	// Rate limiting (after auth to avoid rate-limiting unauthenticated requests)
	if !s.rateLimiter.Allow(req.Source) {
		s.logger.Warn("rate limit exceeded",
			"source", req.Source,
			"remote_addr", r.RemoteAddr,
		)
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	// Play sound asynchronously (returns immediately)
	s.soundPlayer.Play(req.Source)

	s.logger.Info("notification received",
		"source", req.Source,
		"message_length", len(req.Message),
	)

	w.WriteHeader(http.StatusNoContent)
}
