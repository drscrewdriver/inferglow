package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTransport is an in-memory Transport used to drive Client tests.
//
// Send blocks until a reader consumes the message (or ctx is canceled)
// and forwards the bytes to the sentCh channel for inspection. Recv
// blocks until a scripted response is pushed onto recvCh (or ctx is
// canceled). This lets tests deterministically script the server side
// of a JSON-RPC conversation.
type fakeTransport struct {
	mu      sync.Mutex
	started bool
	stopped bool

	sentCh chan []byte
	recvCh chan []byte

	startErr error
	stopErr  error
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		sentCh: make(chan []byte, 16),
		recvCh: make(chan []byte, 16),
	}
}

func (f *fakeTransport) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.started = true
	return nil
}

func (f *fakeTransport) Send(ctx context.Context, msg []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case f.sentCh <- append([]byte(nil), msg...):
		return nil
	}
}

func (f *fakeTransport) Recv(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-f.recvCh:
		if !ok {
			return nil, errors.New("fakeTransport: recvCh closed")
		}
		return msg, nil
	}
}

func (f *fakeTransport) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = true
	return f.stopErr
}

// queueResponse pushes a JSON-RPC response onto recvCh so the next
// Recv call returns it.
func (f *fakeTransport) queueResponse(resp *jsonRPCResponse) {
	data, _ := json.Marshal(resp)
	f.recvCh <- data
}

// waitForRequest blocks for up to 2 seconds reading the next request
// sent through the transport and returns its parsed form.
func (f *fakeTransport) waitForRequest(t *testing.T) *jsonRPCRequest {
	t.Helper()
	select {
	case data := <-f.sentCh:
		var req jsonRPCRequest
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("parse sent request: %v\nraw: %s", err, data)
		}
		return &req
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for client to send a request")
		return nil
	}
}

func TestClient_Initialize(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)

	// Drive the handshake in a goroutine so we can script the
	// server's responses concurrently.
	done := make(chan error, 1)
	go func() {
		_, err := client.Initialize(context.Background())
		done <- err
	}()

	// 1. Expect an `initialize` request.
	req := ft.waitForRequest(t)
	if req.Method != "initialize" {
		t.Fatalf("expected method %q, got %q", "initialize", req.Method)
	}
	if req.JSONRPC != jsonRPCVersion {
		t.Errorf("JSONRPC field = %q, want %q", req.JSONRPC, jsonRPCVersion)
	}
	if req.ID != 1 {
		t.Errorf("first request id = %d, want 1", req.ID)
	}
	// Verify params payload.
	var params map[string]any
	_ = json.Unmarshal(mustMarshal(t, req.Params), &params)
	if got := params["protocolVersion"]; got != protocolVersion {
		t.Errorf("protocolVersion = %v, want %q", got, protocolVersion)
	}
	ci, _ := params["clientInfo"].(map[string]any)
	if ci == nil || ci["name"] != clientName || ci["version"] != clientVersion {
		t.Errorf("clientInfo = %v, want {name:%q version:%q}", ci, clientName, clientVersion)
	}

	// 2. Script the server's initialize response.
	ft.queueResponse(&jsonRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      req.ID,
		Result: mustMarshal(t, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "test-server",
				"version": "0.0.1",
			},
		}),
	})

	// 3. Expect the `notifications/initialized` notification.
	notif := ft.waitForRequest(t)
	if notif.Method != "notifications/initialized" {
		t.Errorf("expected method %q, got %q", "notifications/initialized", notif.Method)
	}
	if notif.ID != 0 {
		t.Errorf("notification should have id=0, got %d", notif.ID)
	}

	// 4. Wait for Initialize to return.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Initialize returned err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Initialize did not return")
	}
}

func TestClient_InitializePropagatesRPCError(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)

	done := make(chan error, 1)
	go func() {
		_, err := client.Initialize(context.Background())
		done <- err
	}()

	req := ft.waitForRequest(t)
	ft.queueResponse(&jsonRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      req.ID,
		Error: &jsonRPCError{
			Code:    -32000,
			Message: "server unavailable",
		},
	})

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "server unavailable") {
			t.Errorf("error %q does not contain server message", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Initialize did not return")
	}
}

func TestClient_ListTools(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)

	done := make(chan error, 1)
	var tools []Tool
	go func() {
		var err error
		tools, err = client.ListTools(context.Background())
		done <- err
	}()

	req := ft.waitForRequest(t)
	if req.Method != "tools/list" {
		t.Fatalf("expected method %q, got %q", "tools/list", req.Method)
	}
	ft.queueResponse(&jsonRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      req.ID,
		Result: mustMarshal(t, map[string]any{
			"tools": []map[string]any{
				{
					"name":        "echo",
					"description": "echoes back its input",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"msg": map[string]any{"type": "string"},
						},
					},
				},
				{
					"name":        "add",
					"description": "adds two numbers",
				},
			},
		}),
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListTools did not return")
	}

	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].Name != "echo" || tools[0].Description != "echoes back its input" {
		t.Errorf("tools[0] = %+v", tools[0])
	}
	if tools[1].Name != "add" {
		t.Errorf("tools[1].Name = %q, want %q", tools[1].Name, "add")
	}
}

func TestClient_CallTool(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)

	done := make(chan error, 1)
	var contents []Content
	go func() {
		var err error
		contents, err = client.CallTool(context.Background(), "echo", map[string]any{"msg": "hi"})
		done <- err
	}()

	req := ft.waitForRequest(t)
	if req.Method != "tools/call" {
		t.Fatalf("expected method %q, got %q", "tools/call", req.Method)
	}
	var params map[string]any
	_ = json.Unmarshal(mustMarshal(t, req.Params), &params)
	if params["name"] != "echo" {
		t.Errorf("params.name = %v, want %q", params["name"], "echo")
	}

	ft.queueResponse(&jsonRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      req.ID,
		Result: mustMarshal(t, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "hi"},
			},
		}),
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool did not return")
	}

	if len(contents) != 1 {
		t.Fatalf("got %d contents, want 1", len(contents))
	}
	if contents[0].Type != "text" || contents[0].Text != "hi" {
		t.Errorf("contents[0] = %+v", contents[0])
	}
}

func TestClient_ContextCancellationAbortsRequest(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.CallTool(ctx, "slow", nil)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
}

func TestClient_CloseSignalsTransport(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)
	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !ft.stopped {
		t.Error("transport.Stop was not called")
	}
	// Double Close should be a no-op.
	if err := client.Close(context.Background()); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestClient_RequestAfterCloseFails(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)
	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := client.ListTools(context.Background()); err == nil {
		t.Error("ListTools after Close should fail")
	}
}

func TestClient_ConcurrentRequestsGetDistinctIDs(t *testing.T) {
	ft := newFakeTransport()
	client := NewClient(ft)

	const N = 5
	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = client.ListTools(context.Background())
		}(i)
	}

	// Read N requests and respond with empty tool lists.
	seenIDs := map[int64]bool{}
	for i := 0; i < N; i++ {
		req := ft.waitForRequest(t)
		if seenIDs[req.ID] {
			t.Errorf("duplicate request id %d", req.ID)
		}
		seenIDs[req.ID] = true
		ft.queueResponse(&jsonRPCResponse{
			JSONRPC: jsonRPCVersion,
			ID:      req.ID,
			Result:  mustMarshal(t, map[string]any{"tools": []any{}}),
		})
	}

	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}
}

// mustMarshal is a test helper that marshals v or fails the test.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
