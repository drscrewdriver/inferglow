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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSSEFrameTransport_SendRecv(t *testing.T) {
	tr := newSSEFrameTransport()
	defer tr.Close()

	ctx := context.Background()

	// Send a message.
	if err := tr.Send(ctx, []byte(`{"test":"hello"}`)); err != nil {
		t.Fatalf("Send error: %v", err)
	}

	// Read it from sendCh (simulating SSE goroutine).
	select {
	case data := <-tr.sendCh:
		if string(data) != `{"test":"hello"}` {
			t.Errorf("got %q, want %q", data, `{"test":"hello"}`)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout reading sendCh")
	}

	// Deliver a message (simulating POST handler).
	if err := tr.deliver([]byte(`{"request":"ping"}`)); err != nil {
		t.Fatalf("deliver error: %v", err)
	}

	// Recv should return it.
	data, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv error: %v", err)
	}
	if string(data) != `{"request":"ping"}` {
		t.Errorf("got %q, want %q", data, `{"request":"ping"}`)
	}
}

func TestSSEFrameTransport_Close(t *testing.T) {
	tr := newSSEFrameTransport()

	// Close should be idempotent.
	if err := tr.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}

	// Send after close should fail.
	if err := tr.Send(context.Background(), []byte("test")); err == nil {
		t.Error("expected error on Send after Close")
	}

	// Recv after close should fail.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := tr.Recv(ctx); err == nil {
		t.Error("expected error on Recv after Close")
	}
}

func TestSSEHandler_InitializeAndToolsList(t *testing.T) {
	s := NewServer(&mockProvider{})
	handler := s.SSEHandler()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Step 1: Establish SSE connection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/sse", nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /sse error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /sse status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read the endpoint event to get the sessionId.
	reader := bufio.NewReader(resp.Body)
	var sessionID string
	for sessionID == "" {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE event: %v", err)
		}
		if strings.HasPrefix(line, "data: /messages?sessionId=") {
			sessionID = strings.TrimPrefix(line, "data: /messages?sessionId=")
			sessionID = strings.TrimSpace(sessionID)
		}
	}
	if sessionID == "" {
		t.Fatal("did not receive session ID from SSE endpoint event")
	}

	// Step 2: Send initialize request via POST.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	postURL := fmt.Sprintf("%s/messages?sessionId=%s", ts.URL, sessionID)
	postResp, err := http.Post(postURL, "application/json", strings.NewReader(initBody))
	if err != nil {
		t.Fatalf("POST /messages error: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusAccepted {
		t.Errorf("POST status = %d, want 202", postResp.StatusCode)
	}

	// Step 3: Read the initialize response from SSE stream.
	var initData string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimSpace(data)
			// Skip the endpoint event we already read.
			if strings.HasPrefix(data, "/messages") {
				continue
			}
			initData = data
			break
		}
	}
	if initData == "" {
		t.Fatal("did not receive initialize response from SSE stream")
	}

	var initResp jsonRPCResponse
	if err := json.Unmarshal([]byte(initData), &initResp); err != nil {
		t.Fatalf("unmarshal init response: %v", err)
	}
	if initResp.ID != 1 {
		t.Errorf("init response ID = %d, want 1", initResp.ID)
	}
	if initResp.Error != nil {
		t.Errorf("init response error: %v", initResp.Error)
	}

	// Step 4: Send tools/list request.
	toolsBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	postResp2, err := http.Post(postURL, "application/json", strings.NewReader(toolsBody))
	if err != nil {
		t.Fatalf("POST tools/list error: %v", err)
	}
	postResp2.Body.Close()

	// Read tools/list response from SSE.
	var toolsData string
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimSpace(data)
			if data == "" || strings.HasPrefix(data, "/messages") {
				continue
			}
			toolsData = data
			break
		}
	}
	if toolsData == "" {
		t.Fatal("did not receive tools/list response from SSE stream")
	}

	var toolsResp jsonRPCResponse
	if err := json.Unmarshal([]byte(toolsData), &toolsResp); err != nil {
		t.Fatalf("unmarshal tools response: %v", err)
	}
	if toolsResp.ID != 2 {
		t.Errorf("tools response ID = %d, want 2", toolsResp.ID)
	}
}

func TestSSEHandler_MissingSessionID(t *testing.T) {
	s := NewServer(&mockProvider{})
	handler := s.SSEHandler()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/messages", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSSEHandler_SessionNotFound(t *testing.T) {
	s := NewServer(&mockProvider{})
	handler := s.SSEHandler()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/messages?sessionId=nonexistent", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSSESessionManager_MaxSessions(t *testing.T) {
	mgr := newSSESessionManager(2)

	tr1 := newSSEFrameTransport()
	id1, err := mgr.register(tr1)
	if err != nil {
		t.Fatalf("register 1: %v", err)
	}
	defer mgr.unregister(id1)

	tr2 := newSSEFrameTransport()
	id2, err := mgr.register(tr2)
	if err != nil {
		t.Fatalf("register 2: %v", err)
	}
	defer mgr.unregister(id2)

	// Third registration should fail.
	tr3 := newSSEFrameTransport()
	_, err = mgr.register(tr3)
	if err == nil {
		t.Error("expected error when exceeding max sessions")
	}
}

func TestSSEHandler_ConcurrentSessions(t *testing.T) {
	s := NewServer(&mockProvider{})
	handler := s.SSEHandler()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	const numClients = 10
	var wg sync.WaitGroup
	errors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/sse", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errors <- fmt.Errorf("client %d: GET error: %w", idx, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("client %d: status = %d", idx, resp.StatusCode)
				return
			}

			// Read session ID.
			reader := bufio.NewReader(resp.Body)
			var sessionID string
			for sessionID == "" {
				line, err := reader.ReadString('\n')
				if err != nil {
					errors <- fmt.Errorf("client %d: read error: %w", idx, err)
					return
				}
				if strings.HasPrefix(line, "data: /messages?sessionId=") {
					sessionID = strings.TrimPrefix(line, "data: /messages?sessionId=")
					sessionID = strings.TrimSpace(sessionID)
				}
			}

			// Send initialize.
			initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
			postURL := fmt.Sprintf("%s/messages?sessionId=%s", ts.URL, sessionID)
			postResp, err := http.Post(postURL, "application/json", strings.NewReader(initBody))
			if err != nil {
				errors <- fmt.Errorf("client %d: POST error: %w", idx, err)
				return
			}
			postResp.Body.Close()

			if postResp.StatusCode != http.StatusAccepted {
				errors <- fmt.Errorf("client %d: POST status = %d", idx, postResp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}
