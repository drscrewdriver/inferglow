package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth_Endpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	Health(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	// Verify status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify Content-Type header
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	// Verify JSON response body
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if status, ok := body["status"]; !ok {
		t.Error("expected 'status' field in response")
	} else if status != "healthy" {
		t.Errorf("expected status 'healthy', got %v", status)
	}

	// Verify other expected fields exist
	for _, field := range []string{"uptime_s", "go_version", "timestamp"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected '%s' field in response", field)
		}
	}
}

func TestOpenAPISpec_Endpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	OpenAPISpec(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	// Verify status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify Content-Type header
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	// Verify JSON response body is a valid OpenAPI spec
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body["openapi"] != "3.0.3" {
		t.Errorf("expected openapi version 3.0.3, got %v", body["openapi"])
	}

	info, ok := body["info"].(map[string]any)
	if !ok {
		t.Fatal("expected 'info' field to be an object")
	}
	if info["title"] != "InferGlow API" {
		t.Errorf("expected title 'InferGlow API', got %v", info["title"])
	}
	if info["version"] != "6.0.0" {
		t.Errorf("expected version '6.0.0', got %v", info["version"])
	}

	paths, ok := body["paths"].(map[string]any)
	if !ok {
		t.Fatal("expected 'paths' field to be an object")
	}
	if _, ok := paths["/health"]; !ok {
		t.Error("expected '/health' path in OpenAPI spec")
	}
}

func TestHealth_ResponseHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	Health(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	expectedHeaders := map[string]string{
		"Content-Type": "application/json",
	}

	for key, expected := range expectedHeaders {
		if got := resp.Header.Get(key); got != expected {
			t.Errorf("expected header %s = %s, got %s", key, expected, got)
		}
	}
}

func TestOpenAPISpec_ResponseHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	OpenAPISpec(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	expectedHeaders := map[string]string{
		"Content-Type": "application/json",
	}

	for key, expected := range expectedHeaders {
		if got := resp.Header.Get(key); got != expected {
			t.Errorf("expected header %s = %s, got %s", key, expected, got)
		}
	}
}
