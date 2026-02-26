// Package server implements the notify-server daemon.
package server

import (
	"context"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

const (
	// soundTimeout is the maximum time to wait for afplay to complete.
	soundTimeout = 30 * time.Second

	// maxConcurrentSounds is the maximum number of concurrent sound playbacks.
	maxConcurrentSounds = 2
)

// SoundPlayer handles sound playback with concurrency limiting.
type SoundPlayer struct {
	config *Config
	logger *slog.Logger
	sem    chan struct{}      // semaphore for concurrency limiting
	ctx    context.Context    // base context for cancellation
	cancel context.CancelFunc // cancel function for shutdown
	wg     sync.WaitGroup     // tracks in-flight playback goroutines
}

// NewSoundPlayer creates a new SoundPlayer with the given configuration.
// The provided context is used as the base context for cancellation on shutdown.
func NewSoundPlayer(cfg *Config, logger *slog.Logger, ctx context.Context) *SoundPlayer {
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	playerCtx, cancel := context.WithCancel(ctx)
	return &SoundPlayer{
		config: cfg,
		logger: logger,
		sem:    make(chan struct{}, maxConcurrentSounds),
		ctx:    playerCtx,
		cancel: cancel,
	}
}

// Play plays the sound configured for the given key-id.
// It runs asynchronously in a goroutine and returns immediately.
// If the concurrency limit is reached, playback is skipped with a warning log.
// If the player has been stopped, playback is skipped.
func (p *SoundPlayer) Play(keyID string) {
	// Check if we're shutting down
	select {
	case <-p.ctx.Done():
		p.logger.Debug("sound playback skipped: player stopped",
			"key_id", keyID,
		)
		return
	default:
	}

	// Try to acquire semaphore (non-blocking)
	select {
	case p.sem <- struct{}{}:
		// Acquired, proceed with playback in goroutine
		p.wg.Add(1)
		go p.playSound(keyID)
	default:
		// Semaphore full, skip playback
		p.logger.Warn("sound playback skipped: concurrency limit reached",
			"key_id", keyID,
		)
	}
}

// playSound executes afplay for the given key-id.
// Must be called after acquiring the semaphore.
func (p *SoundPlayer) playSound(keyID string) {
	defer p.wg.Done()          // signal completion
	defer func() { <-p.sem }() // release semaphore when done

	// Look up sound path for this sender
	soundPath := p.getSoundPath(keyID)

	// Try to play the configured sound
	if err := p.executeAfplay(soundPath); err != nil {
		p.logger.Warn("sound playback failed, trying default",
			"key_id", keyID,
			"sound", soundPath,
			"error", err,
		)

		// If it wasn't already the default, try the default
		if soundPath != p.config.DefaultSound {
			if err := p.executeAfplay(p.config.DefaultSound); err != nil {
				p.logger.Error("default sound playback failed",
					"key_id", keyID,
					"sound", p.config.DefaultSound,
					"error", err,
				)
			}
		}
	}
}

// getSoundPath returns the sound path for the given key-id.
// Falls back to DefaultSound if no sender-specific sound is configured.
func (p *SoundPlayer) getSoundPath(keyID string) string {
	if sender, ok := p.config.Senders[keyID]; ok && sender.Sound != "" {
		return sender.Sound
	}
	return p.config.DefaultSound
}

// executeAfplay runs afplay with the given sound path.
// Uses a timeout context to prevent hanging processes.
// Respects the player's base context for cancellation on shutdown.
func (p *SoundPlayer) executeAfplay(soundPath string) error {
	// Create a context that cancels on either timeout or player shutdown
	ctx, cancel := context.WithTimeout(p.ctx, soundTimeout)
	defer cancel()

	// Use exec.CommandContext with argv array (no shell expansion)
	// This is critical for security - never use shell=true or command strings
	cmd := exec.CommandContext(ctx, "afplay", soundPath)

	return cmd.Run()
}

// Stop cancels all in-flight sound playback and waits for goroutines to complete.
// This should be called during graceful shutdown.
func (p *SoundPlayer) Stop() {
	p.logger.Debug("stopping sound player")
	p.cancel()  // Signal all playback to stop
	p.wg.Wait() // Wait for in-flight goroutines to complete
	p.logger.Debug("sound player stopped")
}
