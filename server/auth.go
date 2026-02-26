// Package server implements the notify-server daemon.
package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Authentication errors. All result in 401 Unauthorized.
var (
	errMissingHeaders   = errors.New("missing auth headers")
	errTimestampExpired = errors.New("timestamp outside window")
	errUnknownSender    = errors.New("unknown sender")
	errInvalidSignature = errors.New("invalid signature")
	errReplayDetected   = errors.New("replay detected")
	errSourceMismatch   = errors.New("source mismatch")
)

// Header names for authentication.
const (
	HeaderKeyID     = "X-Key-Id"
	HeaderTimestamp = "X-Timestamp"
	HeaderSignature = "X-Signature"
)

// computeSignature computes the expected HMAC-SHA256 signature.
// Format: base64(HMAC-SHA256(secret, "<timestamp>.POST./notify.<body>"))
func computeSignature(secret, timestamp string, body []byte) string {
	message := timestamp + ".POST./notify." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// authenticate verifies the request authentication.
// It checks headers, timestamp, signature, replay cache, and source match.
// Returns nil on success, or an error describing the failure.
func (s *Server) authenticate(r *http.Request, body []byte, source string) error {
	// Extract required headers
	keyID := r.Header.Get(HeaderKeyID)
	timestampStr := r.Header.Get(HeaderTimestamp)
	signature := r.Header.Get(HeaderSignature)

	if keyID == "" || timestampStr == "" || signature == "" {
		s.logger.Warn("missing auth headers",
			"has_key_id", keyID != "",
			"has_timestamp", timestampStr != "",
			"has_signature", signature != "",
			"remote_addr", r.RemoteAddr,
		)
		return errMissingHeaders
	}

	// Validate timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		s.logger.Warn("invalid timestamp format",
			"key_id", keyID,
			"remote_addr", r.RemoteAddr,
		)
		return errTimestampExpired
	}

	now := time.Now().Unix()
	skew := int64(s.config.MaxSkewSeconds)
	if timestamp < now-skew || timestamp > now+skew {
		s.logger.Warn("timestamp outside window",
			"key_id", keyID,
			"timestamp", timestamp,
			"now", now,
			"max_skew", skew,
			"remote_addr", r.RemoteAddr,
		)
		return errTimestampExpired
	}

	// Look up sender
	sender, exists := s.config.Senders[keyID]
	if !exists {
		s.logger.Warn("unknown sender",
			"key_id", keyID,
			"remote_addr", r.RemoteAddr,
		)
		return errUnknownSender
	}

	// Verify signature against all configured secrets (for key rotation)
	validSignature := false
	for _, secret := range sender.Secrets {
		expected := computeSignature(secret, timestampStr, body)
		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1 {
			validSignature = true
			break
		}
	}

	if !validSignature {
		s.logger.Warn("invalid signature",
			"key_id", keyID,
			"remote_addr", r.RemoteAddr,
		)
		return errInvalidSignature
	}

	// Check replay cache
	if s.replayCache.Contains(signature) {
		s.logger.Warn("replay detected",
			"key_id", keyID,
			"remote_addr", r.RemoteAddr,
		)
		return errReplayDetected
	}

	// Verify source matches key-id (per ADR-002)
	if source == "" || source != keyID {
		s.logger.Warn("source mismatch",
			"key_id", keyID,
			"body_source", source,
			"remote_addr", r.RemoteAddr,
		)
		return errSourceMismatch
	}

	// Add to replay cache on success
	s.replayCache.Add(signature)

	s.logger.Debug("authentication successful",
		"key_id", keyID,
		"remote_addr", r.RemoteAddr,
	)

	return nil
}
