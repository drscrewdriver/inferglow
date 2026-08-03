package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

func TestLogging_Middleware(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	handler := Logging(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", rec.Body.String())
	}
	if buf.Len() == 0 {
		t.Error("expected log output, got empty")
	}
}

// ---------------------------------------------------------------------------
// Recovery
// ---------------------------------------------------------------------------

func TestRecovery_Middleware(t *testing.T) {
	panickingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := Recovery(panickingHandler)

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

func TestRecovery_Middleware_NormalHandler(t *testing.T) {
	handler := Recovery(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// CORS
// ---------------------------------------------------------------------------

func TestCORS_Middleware_AllowedOrigin(t *testing.T) {
	handler := CORS(http.HandlerFunc(okHandler), []string{"https://example.com"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin 'https://example.com', got '%s'",
			rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_Middleware_DisallowedOrigin(t *testing.T) {
	handler := CORS(http.HandlerFunc(okHandler), []string{"https://example.com"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS header for disallowed origin, got '%s'",
			rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_Middleware_WildcardOrigin(t *testing.T) {
	handler := CORS(http.HandlerFunc(okHandler), []string{"*"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://anything.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://anything.com" {
		t.Errorf("expected Access-Control-Allow-Origin 'https://anything.com', got '%s'",
			rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_Middleware_NoOrigin(t *testing.T) {
	handler := CORS(http.HandlerFunc(okHandler), []string{"https://example.com"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS header when no origin, got '%s'",
			rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_Middleware_Preflight(t *testing.T) {
	handler := CORS(http.HandlerFunc(okHandler), []string{"https://example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204 for preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin header on preflight")
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("expected Access-Control-Allow-Methods header on preflight")
	}
}

// ---------------------------------------------------------------------------
// APIKeyAuth
// ---------------------------------------------------------------------------

func TestAPIKeyAuth_Middleware_Valid(t *testing.T) {
	handler := APIKeyAuth(http.HandlerFunc(okHandler), "secret-key-123")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-key-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestAPIKeyAuth_Middleware_MissingHeader(t *testing.T) {
	handler := APIKeyAuth(http.HandlerFunc(okHandler), "secret-key-123")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAPIKeyAuth_Middleware_InvalidKey(t *testing.T) {
	handler := APIKeyAuth(http.HandlerFunc(okHandler), "secret-key-123")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAPIKeyAuth_Middleware_HealthCheckBypass(t *testing.T) {
	handler := APIKeyAuth(http.HandlerFunc(okHandler), "secret-key-123")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for /health bypass, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// RateLimit
// ---------------------------------------------------------------------------

func TestRateLimit_Middleware_AllowsRequest(t *testing.T) {
	handler := RateLimit(http.HandlerFunc(okHandler), 10)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", rec.Body.String())
	}
}

func TestRateLimit_Middleware_ZeroRPM(t *testing.T) {
	handler := RateLimit(http.HandlerFunc(okHandler), 0)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestRateLimit_Middleware_NegativeRPM(t *testing.T) {
	handler := RateLimit(http.HandlerFunc(okHandler), -1)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}
