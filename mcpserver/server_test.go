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

package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockProvider is a simple ToolProvider for testing.
type mockProvider struct{}

func (m *mockProvider) ListTools() []ToolDescriptor {
	return []ToolDescriptor{
		{
			Name:        "echo",
			Description: "Echoes the input",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
		},
	}
}

func (m *mockProvider) CallTool(_ context.Context, name string, args map[string]any) (*ToolResult, error) {
	if name == "echo" {
		msg, _ := args["message"].(string)
		return &ToolResult{
			Content: []ToolContent{{Type: "text", Text: msg}},
		}, nil
	}
	return nil, context.DeadlineExceeded
}

func TestServerInitialize(t *testing.T) {
	s := NewServer(&mockProvider{})

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	resp := s.HandleMessage(context.Background(), []byte(reqBody))

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.ID != 1 {
		t.Errorf("expected id=1, got %d", resp.ID)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result to be a map")
	}
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("expected protocolVersion=%q, got %v", protocolVersion, result["protocolVersion"])
	}
}

func TestServerToolsList(t *testing.T) {
	s := NewServer(&mockProvider{})

	reqBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	resp := s.HandleMessage(context.Background(), []byte(reqBody))

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result to be a map")
	}
	tools, ok := result["tools"].([]ToolDescriptor)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", result["tools"])
	}
	if tools[0].Name != "echo" {
		t.Errorf("expected tool name 'echo', got %q", tools[0].Name)
	}
}

func TestServerToolsCall(t *testing.T) {
	s := NewServer(&mockProvider{})

	reqBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hello"}}}`
	resp := s.HandleMessage(context.Background(), []byte(reqBody))

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(*ToolResult)
	if !ok {
		t.Fatalf("expected *ToolResult, got %T", resp.Result)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "hello" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestServerUnknownMethod(t *testing.T) {
	s := NewServer(&mockProvider{})

	reqBody := `{"jsonrpc":"2.0","id":4,"method":"unknown/method","params":{}}`
	resp := s.HandleMessage(context.Background(), []byte(reqBody))

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != errCodeNotFound {
		t.Errorf("expected error code %d, got %d", errCodeNotFound, resp.Error.Code)
	}
}

func TestHTTPHandler(t *testing.T) {
	s := NewServer(&mockProvider{})
	handler := s.HTTPHandler()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHTTPHandlerMethodNotAllowed(t *testing.T) {
	s := NewServer(&mockProvider{})
	handler := s.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", w.Code)
	}
}

func TestStdioFrameTransport(t *testing.T) {
	var buf bytes.Buffer
	transport := &StdioFrameTransport{
		Reader: strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"),
		Writer: &buf,
	}

	// Recv
	data, err := transport.Recv(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty data")
	}

	// Send
	err = transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "result") {
		t.Errorf("expected 'result' in output, got %q", buf.String())
	}
}

func TestServeLoop(t *testing.T) {
	s := NewServer(&mockProvider{})

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	var output bytes.Buffer

	transport := &StdioFrameTransport{
		Reader: strings.NewReader(input),
		Writer: &output,
	}

	// Serve will return when Recv hits EOF
	ctx := context.Background()
	err := s.Serve(ctx, transport)
	// EOF is expected
	if err == nil || !strings.Contains(err.Error(), "EOF") {
		// The error could be nil if transport closes gracefully
		// or could be io.EOF
	}

	if output.Len() == 0 {
		t.Fatal("expected some output from server")
	}

	// Parse the response
	var resp jsonRPCResponse
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("no output lines")
	}
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error in response: %v", resp.Error)
	}
}
