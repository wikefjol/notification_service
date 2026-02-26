package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestComputeSignature(t *testing.T) {
	// Test that client signature matches server expectations
	secret := "test-secret"
	timestamp := "1234567890"
	body := []byte(`{"source":"test","message":"hello"}`)

	sig := computeSignature(secret, timestamp, body)

	// Signature should be non-empty base64
	if sig == "" {
		t.Error("signature should not be empty")
	}

	// Same inputs should produce same signature
	sig2 := computeSignature(secret, timestamp, body)
	if sig != sig2 {
		t.Error("same inputs should produce same signature")
	}

	// Different secret should produce different signature
	sig3 := computeSignature("different-secret", timestamp, body)
	if sig == sig3 {
		t.Error("different secret should produce different signature")
	}

	// Different timestamp should produce different signature
	sig4 := computeSignature(secret, "9999999999", body)
	if sig == sig4 {
		t.Error("different timestamp should produce different signature")
	}

	// Different body should produce different signature
	sig5 := computeSignature(secret, timestamp, []byte(`{"source":"test","message":"different"}`))
	if sig == sig5 {
		t.Error("different body should produce different signature")
	}
}

func TestRunWithArgs_MissingSource(t *testing.T) {
	// Clear relevant env vars
	os.Unsetenv("NOTIFY_KEY_ID")
	os.Unsetenv("NOTIFY_SECRET")

	code := runWithArgs([]string{"--message", "test"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunWithArgs_MissingMessage(t *testing.T) {
	os.Setenv("NOTIFY_KEY_ID", "test")
	os.Setenv("NOTIFY_SECRET", "secret")
	defer os.Unsetenv("NOTIFY_KEY_ID")
	defer os.Unsetenv("NOTIFY_SECRET")

	code := runWithArgs([]string{"--source", "test"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunWithArgs_MissingSecret(t *testing.T) {
	os.Unsetenv("NOTIFY_SECRET")
	os.Setenv("NOTIFY_KEY_ID", "test")
	defer os.Unsetenv("NOTIFY_KEY_ID")

	code := runWithArgs([]string{"--message", "test"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunWithArgs_SuccessfulRequest(t *testing.T) {
	// Create a test server that validates the request
	var receivedKeyID, receivedTimestamp, receivedSignature string
	var receivedBody NotifyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/notify" {
			t.Errorf("expected /notify, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		receivedKeyID = r.Header.Get("X-Key-Id")
		receivedTimestamp = r.Header.Get("X-Timestamp")
		receivedSignature = r.Header.Get("X-Signature")

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	os.Setenv("NOTIFY_SECRET", "test-secret")
	os.Setenv("NOTIFY_KEY_ID", "claude")
	defer os.Unsetenv("NOTIFY_SECRET")
	defer os.Unsetenv("NOTIFY_KEY_ID")

	code := runWithArgs([]string{"--message", "hello world", "--url", server.URL})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	// Verify headers were set
	if receivedKeyID != "claude" {
		t.Errorf("expected X-Key-Id 'claude', got '%s'", receivedKeyID)
	}
	if receivedTimestamp == "" {
		t.Error("X-Timestamp should not be empty")
	}
	if receivedSignature == "" {
		t.Error("X-Signature should not be empty")
	}

	// Verify timestamp is recent
	ts, err := strconv.ParseInt(receivedTimestamp, 10, 64)
	if err != nil {
		t.Errorf("failed to parse timestamp: %v", err)
	}
	now := time.Now().Unix()
	if ts < now-5 || ts > now+5 {
		t.Errorf("timestamp %d not within 5 seconds of now %d", ts, now)
	}

	// Verify body
	if receivedBody.Source != "claude" {
		t.Errorf("expected source 'claude', got '%s'", receivedBody.Source)
	}
	if receivedBody.Message != "hello world" {
		t.Errorf("expected message 'hello world', got '%s'", receivedBody.Message)
	}
}

func TestRunWithArgs_OptionalFields(t *testing.T) {
	var receivedBody NotifyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	os.Setenv("NOTIFY_SECRET", "test-secret")
	defer os.Unsetenv("NOTIFY_SECRET")

	code := runWithArgs([]string{
		"--source", "ci",
		"--message", "build done",
		"--event", "done",
		"--repo", "my-repo",
		"--context-id", "build-123",
		"--severity", "info",
		"--url", server.URL,
	})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	if receivedBody.Source != "ci" {
		t.Errorf("expected source 'ci', got '%s'", receivedBody.Source)
	}
	if receivedBody.Message != "build done" {
		t.Errorf("expected message 'build done', got '%s'", receivedBody.Message)
	}
	if receivedBody.Event != "done" {
		t.Errorf("expected event 'done', got '%s'", receivedBody.Event)
	}
	if receivedBody.Repo != "my-repo" {
		t.Errorf("expected repo 'my-repo', got '%s'", receivedBody.Repo)
	}
	if receivedBody.ContextID != "build-123" {
		t.Errorf("expected context_id 'build-123', got '%s'", receivedBody.ContextID)
	}
	if receivedBody.Severity != "info" {
		t.Errorf("expected severity 'info', got '%s'", receivedBody.Severity)
	}
}

func TestRunWithArgs_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	os.Setenv("NOTIFY_SECRET", "wrong-secret")
	os.Setenv("NOTIFY_KEY_ID", "test")
	defer os.Unsetenv("NOTIFY_SECRET")
	defer os.Unsetenv("NOTIFY_KEY_ID")

	code := runWithArgs([]string{"--message", "test", "--url", server.URL})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunWithArgs_SourceFromFlag(t *testing.T) {
	// Source from --source flag should override NOTIFY_KEY_ID
	var receivedKeyID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKeyID = r.Header.Get("X-Key-Id")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	os.Setenv("NOTIFY_SECRET", "test-secret")
	os.Setenv("NOTIFY_KEY_ID", "from-env")
	defer os.Unsetenv("NOTIFY_SECRET")
	defer os.Unsetenv("NOTIFY_KEY_ID")

	code := runWithArgs([]string{"--source", "from-flag", "--message", "test", "--url", server.URL})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	if receivedKeyID != "from-flag" {
		t.Errorf("expected X-Key-Id 'from-flag', got '%s'", receivedKeyID)
	}
}

func TestRunWithArgs_URLFromEnv(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	os.Setenv("NOTIFY_SECRET", "test-secret")
	os.Setenv("NOTIFY_KEY_ID", "test")
	os.Setenv("NOTIFY_URL", server.URL)
	defer os.Unsetenv("NOTIFY_SECRET")
	defer os.Unsetenv("NOTIFY_KEY_ID")
	defer os.Unsetenv("NOTIFY_URL")

	code := runWithArgs([]string{"--message", "test"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	if !called {
		t.Error("server should have been called via NOTIFY_URL")
	}
}
