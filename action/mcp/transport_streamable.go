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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StreamableHTTPTransport implements the Transport interface using the
// MCP Streamable HTTP protocol. Outbound requests are POSTed as JSON;
// responses arrive either as a direct JSON body or as Server-Sent
// Events on a long-lived stream.
//
// This is an experimental transport based on the MCP Streamable HTTP
// specification which may evolve.
type StreamableHTTPTransport struct {
	// Endpoint is the URL of the MCP server's Streamable HTTP endpoint.
	Endpoint string

	// SessionID is the Mcp-Session-Id header value, set after the
	// first response from the server.
	SessionID string

	// HTTPClient is used for requests. When nil, a default client
	// with a 30s timeout is used.
	HTTPClient *http.Client

	mu        sync.Mutex
	started   bool
	done      chan struct{}
	pendingCh chan []byte
	reader    *bufio.Reader
	resp      *http.Response
	closeOnce sync.Once
	readErr   error
}

// Compile-time assertion that StreamableHTTPTransport satisfies Transport.
var _ Transport = (*StreamableHTTPTransport)(nil)

// Start initializes the transport. No persistent connection is opened
// upfront; the SSE stream is established lazily on the first Recv call
// if the server responds with an SSE stream.
func (t *StreamableHTTPTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return errors.New("mcp: transport already started")
	}
	if t.Endpoint == "" {
		return errors.New("mcp: StreamableHTTPTransport.Endpoint is empty")
	}
	if t.HTTPClient == nil {
		t.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	t.done = make(chan struct{})
	t.pendingCh = make(chan []byte, 64)
	t.started = true
	return nil
}

// Send POSTs a JSON-RPC frame to the endpoint. If the response is
// JSON (not SSE), it is enqueued directly into pendingCh. If the
// response is an SSE stream, a background reader is spawned.
func (t *StreamableHTTPTransport) Send(ctx context.Context, msg []byte) error {
	t.mu.Lock()
	started := t.started
	t.mu.Unlock()

	if !started {
		return errors.New("mcp: transport not started")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	t.mu.Lock()
	if t.SessionID != "" {
		req.Header.Set("Mcp-Session-Id", t.SessionID)
	}
	t.mu.Unlock()

	client := t.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	// Capture session ID if present
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		t.SessionID = sid
		t.mu.Unlock()
	}

	contentType := resp.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "text/event-stream") {
		// SSE response: spawn reader
		t.mu.Lock()
		t.resp = resp
		t.reader = bufio.NewReaderSize(resp.Body, 1024*1024)
		t.mu.Unlock()
		go t.readSSELoop()
		return nil
	}

	// JSON response: read body and enqueue
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("mcp: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mcp: HTTP %d: %s", resp.StatusCode, string(body))
	}
	if len(body) > 0 {
		t.enqueue(body)
	}
	return nil
}

// Recv blocks until a JSON-RPC frame is available.
func (t *StreamableHTTPTransport) Recv(ctx context.Context) ([]byte, error) {
	t.mu.Lock()
	started := t.started
	done := t.done
	pendingCh := t.pendingCh
	t.mu.Unlock()

	if !started {
		return nil, errors.New("mcp: transport not started")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		select {
		case msg := <-pendingCh:
			return msg, nil
		default:
		}
		if t.readErr != nil {
			return nil, fmt.Errorf("mcp: transport closed: %w", t.readErr)
		}
		return nil, errors.New("mcp: transport closed")
	case msg := <-pendingCh:
		return msg, nil
	}
}

// Stop closes the transport and any active SSE connection.
func (t *StreamableHTTPTransport) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.started {
		return nil
	}
	t.closeOnce.Do(func() {
		t.readErr = nil
		close(t.done)
	})
	var err error
	if t.resp != nil {
		err = t.resp.Body.Close()
	}
	t.started = false
	return err
}

// readSSELoop drains the SSE stream and enqueues JSON-RPC frames.
func (t *StreamableHTTPTransport) readSSELoop() {
	var dataLines []string
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			if partial := strings.TrimRight(line, "\r\n"); partial != "" && !strings.HasPrefix(partial, ":") {
				if field, value := parseSSEField(partial); field == "data" {
					dataLines = append(dataLines, value)
				}
			}
			if len(dataLines) > 0 {
				t.enqueue([]byte(strings.Join(dataLines, "\n")))
			}
			t.closeDoneSSE(err)
			return
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if len(dataLines) > 0 {
				if !t.enqueue([]byte(strings.Join(dataLines, "\n"))) {
					return
				}
				dataLines = nil
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		if field, value := parseSSEField(line); field == "data" {
			dataLines = append(dataLines, value)
		}
	}
}

func (t *StreamableHTTPTransport) closeDoneSSE(err error) {
	t.closeOnce.Do(func() {
		t.readErr = err
		close(t.done)
	})
}

func (t *StreamableHTTPTransport) enqueue(msg []byte) bool {
	select {
	case <-t.done:
		return false
	case t.pendingCh <- msg:
		return true
	}
}
