// Package main implements the notify-send CLI wrapper.
// It sends authenticated POST requests to the notify-server.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	defaultURL     = "http://127.0.0.1:8666"
	defaultTimeout = 10 * time.Second
)

// Header names for authentication.
const (
	headerKeyID     = "X-Key-Id"
	headerTimestamp = "X-Timestamp"
	headerSignature = "X-Signature"
)

// NotifyRequest represents the JSON body sent to the server.
type NotifyRequest struct {
	Source    string `json:"source"`
	Message   string `json:"message"`
	Event     string `json:"event,omitempty"`
	Repo      string `json:"repo,omitempty"`
	ContextID string `json:"context_id,omitempty"`
	Severity  string `json:"severity,omitempty"`
}

// computeSignature computes the HMAC-SHA256 signature.
// Format: base64(HMAC-SHA256(secret, "<timestamp>.POST./notify.<body>"))
func computeSignature(secret, timestamp string, body []byte) string {
	message := timestamp + ".POST./notify." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func main() {
	os.Exit(runWithArgs(os.Args[1:]))
}

func runWithArgs(args []string) int {
	// Create a new FlagSet for each invocation (enables testing)
	fs := flag.NewFlagSet("notify-send", flag.ContinueOnError)

	// Define flags
	source := fs.String("source", "", "Source identifier (required, or set NOTIFY_KEY_ID)")
	message := fs.String("message", "", "Message to send (required)")
	event := fs.String("event", "", "Event type (optional)")
	repo := fs.String("repo", "", "Repository name (optional)")
	contextID := fs.String("context-id", "", "Context ID (optional)")
	severity := fs.String("severity", "", "Severity level: info|warn|crit (optional)")
	serverURL := fs.String("url", "", "Server URL (default: NOTIFY_URL or http://127.0.0.1:8666)")
	timeout := fs.Duration("timeout", defaultTimeout, "Request timeout")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: notify-send --source <source> --message <message> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Send an authenticated notification to notify-server.\n\n")
		fmt.Fprintf(os.Stderr, "Environment variables:\n")
		fmt.Fprintf(os.Stderr, "  NOTIFY_KEY_ID    Source identifier (can override with --source)\n")
		fmt.Fprintf(os.Stderr, "  NOTIFY_SECRET    Shared secret for HMAC signing (required)\n")
		fmt.Fprintf(os.Stderr, "  NOTIFY_URL       Server URL (default: http://127.0.0.1:8666)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Resolve source: flag > env
	keyID := *source
	if keyID == "" {
		keyID = os.Getenv("NOTIFY_KEY_ID")
	}
	if keyID == "" {
		fmt.Fprintln(os.Stderr, "error: source is required (--source or NOTIFY_KEY_ID)")
		return 1
	}

	// Validate message
	if *message == "" {
		fmt.Fprintln(os.Stderr, "error: message is required (--message)")
		return 1
	}

	// Get secret from env
	secret := os.Getenv("NOTIFY_SECRET")
	if secret == "" {
		fmt.Fprintln(os.Stderr, "error: NOTIFY_SECRET environment variable is required")
		return 1
	}

	// Resolve URL: flag > env > default
	url := *serverURL
	if url == "" {
		url = os.Getenv("NOTIFY_URL")
	}
	if url == "" {
		url = defaultURL
	}

	// Build request body
	reqBody := NotifyRequest{
		Source:    keyID,
		Message:   *message,
		Event:     *event,
		Repo:      *repo,
		ContextID: *contextID,
		Severity:  *severity,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to encode JSON: %v\n", err)
		return 1
	}

	// Compute timestamp and signature
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := computeSignature(secret, timestamp, body)

	// Create HTTP request
	endpoint := url + "/notify"
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create request: %v\n", err)
		return 1
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerKeyID, keyID)
	req.Header.Set(headerTimestamp, timestamp)
	req.Header.Set(headerSignature, signature)

	// Send request
	client := &http.Client{Timeout: *timeout}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: request failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	// Handle response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("OK (%d)\n", resp.StatusCode)
		return 0
	}

	// Read error body if any (though server returns empty body on errors)
	errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if len(errBody) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d %s: %s\n", resp.StatusCode, resp.Status, string(errBody))
	} else {
		fmt.Fprintf(os.Stderr, "error: %d %s\n", resp.StatusCode, resp.Status)
	}
	return 1
}
