// Package server implements the notify-server daemon.
package server

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	// shutdownTimeout is the maximum time to wait for graceful shutdown.
	shutdownTimeout = 5 * time.Second
)

// Run is the main entry point for the server.
// It loads config, sets up signal handling, and runs the server.
// Returns exit code: 0 for graceful shutdown, 1 for error/forced.
func Run() int {
	// Set up structured logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration
	configPath := DefaultConfigPath()
	cfg, err := LoadConfig(configPath)
	if err != nil {
		logger.Error("failed to load config",
			"path", configPath,
			"error", err,
		)
		return 1
	}

	// Set up signal handling context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Create and start server
	srv := NewServer(cfg, logger, ctx)

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for signal or server error
	select {
	case err := <-errCh:
		if err != nil {
			logger.Error("server error", "error", err)
			return 1
		}
		// Server stopped without error (shouldn't happen normally)
		return 0

	case <-ctx.Done():
		// Signal received, initiate graceful shutdown
		logger.Info("shutdown signal received, starting graceful shutdown")
	}

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		// Stop sounds even on error
		srv.StopSounds()
		logger.Warn("shutdown forced due to timeout")
		return 1
	}

	// Stop sound playback goroutines
	srv.StopSounds()

	logger.Info("shutdown completed")
	return 0
}
