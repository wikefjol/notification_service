package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testServerWithCache creates a test server with replay cache initialized.
func testServerWithCache() *Server {
	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ttl := time.Duration(cfg.MaxSkewSeconds*2) * time.Second
	return &Server{
		config:      cfg,
		logger:      logger,
		replayCache: NewReplayCache(cfg.ReplayCacheMaxEntries, ttl),
	}
}

// makeSignature creates a valid signature for testing.
func makeSignature(secret, timestamp string, body []byte) string {
	message := timestamp + ".POST./notify." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// makeRequest creates an HTTP request with the given headers and body.
func makeRequest(body, keyID, timestamp, signature string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if keyID != "" {
		req.Header.Set(HeaderKeyID, keyID)
	}
	if timestamp != "" {
		req.Header.Set(HeaderTimestamp, timestamp)
	}
	if signature != "" {
		req.Header.Set(HeaderSignature, signature)
	}
	return req
}

func TestComputeSignature(t *testing.T) {
	secret := "test-secret"
	timestamp := "1234567890"
	body := []byte(`{"source":"test","message":"hello"}`)

	sig := computeSignature(secret, timestamp, body)

	// Verify it's valid base64
	decoded, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		t.Fatalf("signature is not valid base64: %v", err)
	}

	// SHA256 produces 32 bytes
	if len(decoded) != 32 {
		t.Errorf("expected 32 byte signature, got %d", len(decoded))
	}

	// Verify deterministic
	sig2 := computeSignature(secret, timestamp, body)
	if sig != sig2 {
		t.Error("signature should be deterministic")
	}

	// Different secret produces different signature
	sig3 := computeSignature("other-secret", timestamp, body)
	if sig == sig3 {
		t.Error("different secret should produce different signature")
	}

	// Different timestamp produces different signature
	sig4 := computeSignature(secret, "9999999999", body)
	if sig == sig4 {
		t.Error("different timestamp should produce different signature")
	}

	// Different body produces different signature
	sig5 := computeSignature(secret, timestamp, []byte(`{"source":"test","message":"world"}`))
	if sig == sig5 {
		t.Error("different body should produce different signature")
	}
}

func TestAuthenticate_ValidSignature(t *testing.T) {
	srv := testServerWithCache()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"source":"test-agent","message":"hello"}`)
	signature := makeSignature("test-secret", timestamp, body)

	req := makeRequest(string(body), "test-agent", timestamp, signature)
	err := srv.authenticate(req, body, "test-agent")

	if err != nil {
		t.Errorf("expected no error for valid signature, got: %v", err)
	}
}

func TestAuthenticate_MissingHeaders(t *testing.T) {
	srv := testServerWithCache()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"source":"test-agent","message":"hello"}`)
	signature := makeSignature("test-secret", timestamp, body)

	tests := []struct {
		name      string
		keyID     string
		timestamp string
		signature string
	}{
		{"missing key-id", "", timestamp, signature},
		{"missing timestamp", "test-agent", "", signature},
		{"missing signature", "test-agent", timestamp, ""},
		{"missing all", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeRequest(string(body), tt.keyID, tt.timestamp, tt.signature)
			err := srv.authenticate(req, body, "test-agent")
			if err != errMissingHeaders {
				t.Errorf("expected errMissingHeaders, got: %v", err)
			}
		})
	}
}

func TestAuthenticate_TimestampTooOld(t *testing.T) {
	srv := testServerWithCache()

	// Timestamp 2 minutes ago (beyond 60s skew)
	oldTimestamp := strconv.FormatInt(time.Now().Unix()-120, 10)
	body := []byte(`{"source":"test-agent","message":"hello"}`)
	signature := makeSignature("test-secret", oldTimestamp, body)

	req := makeRequest(string(body), "test-agent", oldTimestamp, signature)
	err := srv.authenticate(req, body, "test-agent")

	if err != errTimestampExpired {
		t.Errorf("expected errTimestampExpired for old timestamp, got: %v", err)
	}
}

func TestAuthenticate_TimestampTooNew(t *testing.T) {
	srv := testServerWithCache()

	// Timestamp 2 minutes in the future (beyond 60s skew)
	futureTimestamp := strconv.FormatInt(time.Now().Unix()+120, 10)
	body := []byte(`{"source":"test-agent","message":"hello"}`)
	signature := makeSignature("test-secret", futureTimestamp, body)

	req := makeRequest(string(body), "test-agent", futureTimestamp, signature)
	err := srv.authenticate(req, body, "test-agent")

	if err != errTimestampExpired {
		t.Errorf("expected errTimestampExpired for future timestamp, got: %v", err)
	}
}

func TestAuthenticate_TimestampAtBoundary(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name      string
		offset    int64
		shouldErr bool
	}{
		{"exactly at -60s", -60, false},
		{"exactly at +60s", +60, false},
		{"one second beyond -60s", -61, true},
		{"one second beyond +60s", +61, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh server to avoid replay cache interference
			srv := testServerWithCache()

			timestamp := strconv.FormatInt(now+tt.offset, 10)
			body := []byte(`{"source":"test-agent","message":"hello"}`)
			signature := makeSignature("test-secret", timestamp, body)

			req := makeRequest(string(body), "test-agent", timestamp, signature)
			err := srv.authenticate(req, body, "test-agent")

			if tt.shouldErr && err != errTimestampExpired {
				t.Errorf("expected errTimestampExpired, got: %v", err)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestAuthenticate_InvalidSignature(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"source":"test-agent","message":"hello"}`)

	tests := []struct {
		name      string
		signature string
	}{
		{"wrong signature", "dGhpcyBpcyBub3QgYSB2YWxpZCBzaWduYXR1cmU="},
		{"tampered signature", makeSignature("wrong-secret", timestamp, body)},
		{"invalid base64", "notbase64!!@@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := testServerWithCache() // Fresh server per test
			req := makeRequest(string(body), "test-agent", timestamp, tt.signature)
			err := srv.authenticate(req, body, "test-agent")
			if err != errInvalidSignature {
				t.Errorf("expected errInvalidSignature, got: %v", err)
			}
		})
	}
}

func TestAuthenticate_UnknownSender(t *testing.T) {
	srv := testServerWithCache()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"source":"unknown-agent","message":"hello"}`)
	signature := makeSignature("test-secret", timestamp, body)

	req := makeRequest(string(body), "unknown-agent", timestamp, signature)
	err := srv.authenticate(req, body, "unknown-agent")

	if err != errUnknownSender {
		t.Errorf("expected errUnknownSender, got: %v", err)
	}
}

func TestAuthenticate_MultipleSecrets(t *testing.T) {
	// Config with multiple secrets for key rotation
	cfg := &Config{
		ListenAddr:            "127.0.0.1:8666",
		MaxBodyBytes:          4096,
		MaxSkewSeconds:        60,
		ReplayCacheMaxEntries: 10000,
		DefaultSound:          "/System/Library/Sounds/Ping.aiff",
		Senders: map[string]SenderConfig{
			"rotating-agent": {
				Secrets: []string{"old-secret", "new-secret"},
			},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ttl := time.Duration(cfg.MaxSkewSeconds*2) * time.Second
	srv := &Server{
		config:      cfg,
		logger:      logger,
		replayCache: NewReplayCache(cfg.ReplayCacheMaxEntries, ttl),
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"source":"rotating-agent","message":"hello"}`)

	// Sign with old secret (first in list)
	sigOld := makeSignature("old-secret", timestamp, body)
	req := makeRequest(string(body), "rotating-agent", timestamp, sigOld)
	err := srv.authenticate(req, body, "rotating-agent")
	if err != nil {
		t.Errorf("expected old secret to work, got: %v", err)
	}

	// Sign with new secret (second in list) - need fresh timestamp to avoid replay
	timestamp2 := strconv.FormatInt(time.Now().Unix()+1, 10)
	body2 := []byte(`{"source":"rotating-agent","message":"hello2"}`)
	sigNew := makeSignature("new-secret", timestamp2, body2)
	req2 := makeRequest(string(body2), "rotating-agent", timestamp2, sigNew)
	err = srv.authenticate(req2, body2, "rotating-agent")
	if err != nil {
		t.Errorf("expected new secret to work, got: %v", err)
	}

	// Wrong secret should fail
	sigWrong := makeSignature("wrong-secret", timestamp, body)
	req3 := makeRequest(string(body), "rotating-agent", timestamp, sigWrong)
	err = srv.authenticate(req3, body, "rotating-agent")
	if err != errInvalidSignature {
		t.Errorf("expected errInvalidSignature for wrong secret, got: %v", err)
	}
}

func TestAuthenticate_ReplayDetected(t *testing.T) {
	srv := testServerWithCache()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"source":"test-agent","message":"hello"}`)
	signature := makeSignature("test-secret", timestamp, body)

	// First request should succeed
	req1 := makeRequest(string(body), "test-agent", timestamp, signature)
	err := srv.authenticate(req1, body, "test-agent")
	if err != nil {
		t.Fatalf("first request should succeed: %v", err)
	}

	// Replay should be rejected
	req2 := makeRequest(string(body), "test-agent", timestamp, signature)
	err = srv.authenticate(req2, body, "test-agent")
	if err != errReplayDetected {
		t.Errorf("expected errReplayDetected for replay, got: %v", err)
	}
}

func TestAuthenticate_SourceMismatch(t *testing.T) {
	srv := testServerWithCache()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	// Body has different source than header
	body := []byte(`{"source":"other-agent","message":"hello"}`)
	signature := makeSignature("test-secret", timestamp, body)

	req := makeRequest(string(body), "test-agent", timestamp, signature)
	err := srv.authenticate(req, body, "other-agent")

	if err != errSourceMismatch {
		t.Errorf("expected errSourceMismatch, got: %v", err)
	}
}

func TestAuthenticate_SourceMissing(t *testing.T) {
	srv := testServerWithCache()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := []byte(`{"message":"hello"}`) // No source field
	signature := makeSignature("test-secret", timestamp, body)

	req := makeRequest(string(body), "test-agent", timestamp, signature)
	err := srv.authenticate(req, body, "") // Empty source from parsed body

	if err != errSourceMismatch {
		t.Errorf("expected errSourceMismatch for missing source, got: %v", err)
	}
}

func TestAuthenticate_InvalidTimestampFormat(t *testing.T) {
	srv := testServerWithCache()

	body := []byte(`{"source":"test-agent","message":"hello"}`)

	tests := []struct {
		name      string
		timestamp string
	}{
		{"non-numeric", "not-a-number"},
		{"float", "1234567890.123"},
		{"empty after validation", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need a signature, though it won't matter for this test
			signature := makeSignature("test-secret", tt.timestamp, body)
			req := makeRequest(string(body), "test-agent", tt.timestamp, signature)
			err := srv.authenticate(req, body, "test-agent")
			if err != errTimestampExpired {
				t.Errorf("expected errTimestampExpired for invalid format, got: %v", err)
			}
		})
	}
}
