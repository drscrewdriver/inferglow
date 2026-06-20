// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamableHTTPTransportSendRecv(t *testing.T) {
	// Mock server that returns JSON response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		// Set session ID header
		w.Header().Set("Mcp-Session-Id", "test-session-123")
		w.Header().Set("Content-Type", "application/json")

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]any{"status": "ok"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := &StreamableHTTPTransport{
		Endpoint: server.URL,
	}

	if err := transport.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer transport.Stop(context.Background())

	// Send a request
	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if err := transport.Send(context.Background(), msg); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	// Verify session ID was captured
	if transport.SessionID != "test-session-123" {
		t.Errorf("expected session ID 'test-session-123', got %q", transport.SessionID)
	}

	// Recv the response
	data, err := transport.Recv(context.Background())
	if err != nil {
		t.Fatalf("recv failed: %v", err)
	}
	if !strings.Contains(string(data), "ok") {
		t.Errorf("expected 'ok' in response, got %q", string(data))
	}
}

func TestStreamableHTTPTransportStartEmpty(t *testing.T) {
	transport := &StreamableHTTPTransport{}
	err := transport.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestStreamableHTTPTransportDoubleStart(t *testing.T) {
	transport := &StreamableHTTPTransport{Endpoint: "http://localhost:9999"}
	if err := transport.Start(context.Background()); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	defer transport.Stop(context.Background())

	err := transport.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for double start")
	}
}

func TestNewTransportFromConfigSSE(t *testing.T) {
	cfg := MCPServerConfig{
		Transport: "sse",
		Endpoint:  "http://localhost:8080/sse",
	}
	tr, err := NewTransportFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := tr.(*HTTPTransport); !ok {
		t.Fatalf("expected *HTTPTransport, got %T", tr)
	}
}

func TestNewTransportFromConfigStreamableHTTP(t *testing.T) {
	cfg := MCPServerConfig{
		Transport: "streamable-http",
		Endpoint:  "http://localhost:8080/mcp",
	}
	tr, err := NewTransportFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := tr.(*StreamableHTTPTransport); !ok {
		t.Fatalf("expected *StreamableHTTPTransport, got %T", tr)
	}
}
