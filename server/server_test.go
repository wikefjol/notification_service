package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testConfig returns a valid Config for testing.
func testConfig() *Config {
	return &Config{
		ListenAddr:            "127.0.0.1:8666",
		MaxBodyBytes:          4096,
		MaxSkewSeconds:        60,
		RateLimitPerMinute:    10,
		RateLimitBurst:        3,
		ReplayCacheMaxEntries: 10000,
		DefaultSound:          "/System/Library/Sounds/Ping.aiff",
		Senders: map[string]SenderConfig{
			"test-agent": {
				Secrets: []string{"test-secret"},
				Sound:   "/custom/sound.aiff",
			},
		},
	}
}

// discardLogger returns a logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthz(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz: expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestNotify_MethodNotAllowed(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/notify", nil)
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /notify: expected status %d, got %d", method, http.StatusMethodNotAllowed, rec.Code)
			}
			// Verify empty body
			if rec.Body.Len() != 0 {
				t.Errorf("%s /notify: expected empty body, got %q", method, rec.Body.String())
			}
		})
	}
}

func TestNotify_MissingContentType(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	body := `{"source": "test-agent", "message": "hello"}`
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	// No Content-Type header set
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("POST /notify without Content-Type: expected status %d, got %d",
			http.StatusUnsupportedMediaType, rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", rec.Body.String())
	}
}

func TestNotify_WrongContentType(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	wrongTypes := []string{
		"text/plain",
		"application/xml",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
	}

	for _, ct := range wrongTypes {
		t.Run(ct, func(t *testing.T) {
			body := `{"source": "test-agent", "message": "hello"}`
			req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
			req.Header.Set("Content-Type", ct)
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnsupportedMediaType {
				t.Errorf("POST /notify with Content-Type %q: expected status %d, got %d",
					ct, http.StatusUnsupportedMediaType, rec.Code)
			}
		})
	}
}

func TestNotify_BodyTooLarge(t *testing.T) {
	cfg := testConfig()
	cfg.MaxBodyBytes = 100 // Small limit for testing
	srv := NewServer(cfg, discardLogger())

	// Create a body larger than the limit
	largeBody := `{"source": "test-agent", "message": "` + strings.Repeat("x", 200) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("POST /notify with large body: expected status %d, got %d",
			http.StatusRequestEntityTooLarge, rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", rec.Body.String())
	}
}

func TestNotify_InvalidJSON(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	invalidBodies := []string{
		"not json at all",
		"{invalid json}",
		`{"source": "test"`, // truncated
		"",
		"[]", // array, not object
	}

	for _, body := range invalidBodies {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("POST /notify with body %q: expected status %d, got %d",
					body, http.StatusBadRequest, rec.Code)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("expected empty body, got %q", rec.Body.String())
			}
		})
	}
}

func TestNotify_MissingSource(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	bodies := []string{
		`{"message": "hello"}`,          // missing source
		`{"source": "", "message": "x"}`, // empty source
	}

	for _, body := range bodies {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("POST /notify with body %q: expected status %d, got %d",
					body, http.StatusBadRequest, rec.Code)
			}
		})
	}
}

func TestNotify_ValidRequest(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	body := `{"source": "test-agent", "message": "hello world"}`
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	// For now, without auth, valid JSON returns 204
	// Once auth is implemented, this will need to change
	if rec.Code != http.StatusNoContent {
		t.Errorf("POST /notify with valid body: expected status %d, got %d",
			http.StatusNoContent, rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", rec.Body.String())
	}
}

func TestNotify_BodyAtExactLimit(t *testing.T) {
	cfg := testConfig()
	cfg.MaxBodyBytes = 100
	srv := NewServer(cfg, discardLogger())

	// Create a body exactly at the limit
	// JSON overhead: {"source":"test-agent","message":""} = 36 bytes
	// So we can have 64 bytes of message content
	msgLen := 100 - len(`{"source":"test-agent","message":""}`)
	body := `{"source":"test-agent","message":"` + strings.Repeat("x", msgLen) + `"}`

	if len(body) != 100 {
		t.Fatalf("test setup error: body length is %d, expected 100", len(body))
	}

	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("POST /notify with body at limit: expected status %d, got %d",
			http.StatusNoContent, rec.Code)
	}
}

func TestNotify_BodyOneOverLimit(t *testing.T) {
	cfg := testConfig()
	cfg.MaxBodyBytes = 100
	srv := NewServer(cfg, discardLogger())

	// Create a body one byte over the limit
	body := bytes.Repeat([]byte("x"), 101)
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("POST /notify with body over limit: expected status %d, got %d",
			http.StatusRequestEntityTooLarge, rec.Code)
	}
}

func TestServerTimeouts(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	// Verify timeouts are set correctly per ADR-007
	if srv.httpServer.ReadHeaderTimeout != 5*1e9 { // 5 seconds in nanoseconds
		t.Errorf("ReadHeaderTimeout: expected 5s, got %v", srv.httpServer.ReadHeaderTimeout)
	}
	if srv.httpServer.ReadTimeout != 10*1e9 {
		t.Errorf("ReadTimeout: expected 10s, got %v", srv.httpServer.ReadTimeout)
	}
	if srv.httpServer.WriteTimeout != 10*1e9 {
		t.Errorf("WriteTimeout: expected 10s, got %v", srv.httpServer.WriteTimeout)
	}
	if srv.httpServer.IdleTimeout != 60*1e9 {
		t.Errorf("IdleTimeout: expected 60s, got %v", srv.httpServer.IdleTimeout)
	}
}

func TestServerBindsToConfiguredAddress(t *testing.T) {
	cfg := testConfig()
	cfg.ListenAddr = "127.0.0.1:9999"
	srv := NewServer(cfg, discardLogger())

	if srv.httpServer.Addr != "127.0.0.1:9999" {
		t.Errorf("Server addr: expected %q, got %q", "127.0.0.1:9999", srv.httpServer.Addr)
	}
}

func TestNotify_EmptyBody(t *testing.T) {
	srv := NewServer(testConfig(), discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/notify", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	// Empty body is invalid JSON
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /notify with empty body: expected status %d, got %d",
			http.StatusBadRequest, rec.Code)
	}
}
