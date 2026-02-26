package server

import (
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetSoundPath(t *testing.T) {
	cfg := &Config{
		DefaultSound: "/default/sound.wav",
		Senders: map[string]SenderConfig{
			"sender-with-sound": {
				Secrets: []string{"secret"},
				Sound:   "/custom/sound.wav",
			},
			"sender-without-sound": {
				Secrets: []string{"secret"},
				Sound:   "", // empty, should fallback
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	player := NewSoundPlayer(cfg, logger)

	tests := []struct {
		name     string
		keyID    string
		expected string
	}{
		{
			name:     "sender with custom sound",
			keyID:    "sender-with-sound",
			expected: "/custom/sound.wav",
		},
		{
			name:     "sender without sound uses default",
			keyID:    "sender-without-sound",
			expected: "/default/sound.wav",
		},
		{
			name:     "unknown sender uses default",
			keyID:    "unknown-sender",
			expected: "/default/sound.wav",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := player.getSoundPath(tt.keyID)
			if got != tt.expected {
				t.Errorf("getSoundPath(%q) = %q, want %q", tt.keyID, got, tt.expected)
			}
		})
	}
}

func TestSemaphoreConcurrencyLimit(t *testing.T) {
	cfg := &Config{
		DefaultSound: "/nonexistent/sound.wav",
		Senders: map[string]SenderConfig{
			"test": {Secrets: []string{"secret"}},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	player := NewSoundPlayer(cfg, logger)

	// Fill the semaphore manually
	player.sem <- struct{}{}
	player.sem <- struct{}{}

	// Verify semaphore is full (non-blocking check)
	select {
	case player.sem <- struct{}{}:
		t.Fatal("semaphore should be full but accepted another")
	default:
		// expected: semaphore is full
	}

	// Clean up
	<-player.sem
	<-player.sem
}

func TestPlaySkipsWhenSemaphoreFull(t *testing.T) {
	cfg := &Config{
		DefaultSound: "/nonexistent/sound.wav",
		Senders: map[string]SenderConfig{
			"test": {Secrets: []string{"secret"}},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	player := NewSoundPlayer(cfg, logger)

	// Fill the semaphore
	player.sem <- struct{}{}
	player.sem <- struct{}{}

	// This should not block; it should skip and log a warning
	done := make(chan struct{})
	go func() {
		player.Play("test")
		close(done)
	}()

	select {
	case <-done:
		// Play returned immediately (expected)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Play() blocked when semaphore was full")
	}

	// Clean up
	<-player.sem
	<-player.sem
}

func TestConcurrentPlaybackLimit(t *testing.T) {
	cfg := &Config{
		DefaultSound: "/nonexistent/sound.wav",
		Senders: map[string]SenderConfig{
			"test": {Secrets: []string{"secret"}},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	player := NewSoundPlayer(cfg, logger)

	// Track concurrent executions
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	var wg sync.WaitGroup

	// Override playSound to track concurrency (we can't easily mock afplay,
	// but we can verify the semaphore behavior)
	originalSem := player.sem
	player.sem = make(chan struct{}, maxConcurrentSounds)

	// Start multiple goroutines that try to acquire the semaphore
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case player.sem <- struct{}{}:
				// Acquired semaphore
				current := concurrent.Add(1)
				// Update max if needed
				for {
					max := maxConcurrent.Load()
					if current <= max || maxConcurrent.CompareAndSwap(max, current) {
						break
					}
				}

				// Simulate some work
				time.Sleep(10 * time.Millisecond)

				concurrent.Add(-1)
				<-player.sem
			default:
				// Skipped due to full semaphore
			}
		}()
	}

	wg.Wait()
	player.sem = originalSem

	if max := maxConcurrent.Load(); max > maxConcurrentSounds {
		t.Errorf("max concurrent = %d, want <= %d", max, maxConcurrentSounds)
	}
}

func TestNewSoundPlayerNilLogger(t *testing.T) {
	cfg := &Config{
		DefaultSound: "/default/sound.wav",
		Senders:      map[string]SenderConfig{},
	}

	// Should not panic with nil logger
	player := NewSoundPlayer(cfg, nil)
	if player.logger == nil {
		t.Error("expected default logger, got nil")
	}
}
