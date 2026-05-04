package model

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestP1Integration_ConfigProviderToAnthropicProvider exercises the P1
// ConfigProvider -> Provider factory -> AnthropicCompatibleProvider chain.
//
// It builds a CompositeConfigProvider backed by a StaticConfigProvider,
// constructs an AnthropicCompatibleProvider via NewAnthropicProviderFromConfig,
// verifies the provider fields are populated from the config, then drives a
// real RequestModel call against an httptest.Server that mocks the Claude
// Messages API and inspects the outbound request headers (x-api-key,
// anthropic-version).
func TestP1Integration_ConfigProviderToAnthropicProvider(t *testing.T) {
	// 1. Build a CompositeConfigProvider with static config values.
	cp := NewComposite(&StaticConfigProvider{
		Values: map[string]any{
			"anthropic": map[string]any{
				"api_key": "test-key",
				"model":   "claude-test",
			},
		},
	})

	// 2. Construct the AnthropicCompatibleProvider via the factory.
	provider, err := NewAnthropicProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("NewAnthropicProviderFromConfig failed: %v", err)
	}

	// 3. Verify provider identity and fields.
	if provider.Name() != "anthropic" {
		t.Errorf("Name() = %q, want \"anthropic\"", provider.Name())
	}
	if provider.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want \"test-key\"", provider.APIKey)
	}
	if provider.Model != "claude-test" {
		t.Errorf("Model = %q, want \"claude-test\"", provider.Model)
	}

	// 4. Spin up an httptest.Server that mocks the Claude /v1/messages endpoint
	//    and captures the inbound x-api-key / anthropic-version headers.
	var receivedAPIKey, receivedAnthropicVersion, receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("x-api-key")
		receivedAnthropicVersion = r.Header.Get("anthropic-version")
		receivedPath = r.URL.Path

		// Minimal SSE stream that terminates immediately.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	// Point the provider at the mock server.
	provider.BaseURL = server.URL

	// 5. Generate the request data and call RequestModel.
	data, err := provider.GenerateRequestData(context.Background(), &ModelRequest{
		Input: "Hello",
	})
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	// Drain the stream so the server handler runs to completion.
	for range stream {
	}

	// 6. Verify the outbound request carried the correct headers and path.
	if receivedPath != "/v1/messages" {
		t.Errorf("request path = %q, want \"/v1/messages\"", receivedPath)
	}
	if receivedAPIKey != "test-key" {
		t.Errorf("x-api-key header = %q, want \"test-key\"", receivedAPIKey)
	}
	if receivedAnthropicVersion != "2024-10-22" {
		t.Errorf("anthropic-version header = %q, want \"2024-10-22\"", receivedAnthropicVersion)
	}
}

// TestP1Integration_ConfigProviderMissingAPIKey verifies that
// NewAnthropicProviderFromConfig surfaces ErrMissingRequiredConfig (via %w
// wrapping in the factory) when the ConfigProvider does not supply an
// api_key for the "anthropic" prefix.
func TestP1Integration_ConfigProviderMissingAPIKey(t *testing.T) {
	cp := NewComposite(&StaticConfigProvider{
		Values: map[string]any{
			"anthropic": map[string]any{
				// api_key intentionally omitted
				"model": "claude-test",
			},
		},
	})

	_, err := NewAnthropicProviderFromConfig(cp)
	if err == nil {
		t.Fatal("expected error for missing api_key, got nil")
	}
	if !errors.Is(err, ErrMissingRequiredConfig) {
		t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
	}
}
