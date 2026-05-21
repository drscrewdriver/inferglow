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
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPTransport is a Transport that exchanges JSON-RPC 2.0 frames
// with an MCP server over HTTP. Outbound requests are POSTed to
// sendURL as plain JSON; inbound responses arrive as `data: {json}`
// events on a long-lived Server-Sent Events (SSE) stream opened
// during Start.
//
// It is the network counterpart to StdioTransport and supports MCP
// servers that expose themselves over HTTP. Set baseURL to the SSE
// endpoint; if sendURL is empty it defaults to baseURL at Start time.
type HTTPTransport struct {
	// baseURL is the SSE endpoint opened with GET during Start.
	baseURL string
	// httpClient is used for POST requests in Send. When nil a
	// default client with a 30s timeout is allocated in Start. The
	// SSE stream uses a separate no-timeout client so the long-lived
	// connection is not torn down while waiting for events.
	httpClient *http.Client
	// reader wraps resp.Body for line-buffered SSE parsing.
	reader *bufio.Reader
	// resp is the long-lived SSE HTTP response closed by Stop.
	resp *http.Response
	// sendURL is the POST endpoint; defaults to baseURL when empty.
	sendURL string
	// mu guards the shared mutable fields above and the started
	// flag. It is held briefly during Start/Stop and during the
	// state-snapshot at the top of Send/Recv; network and channel
	// operations run without the lock so Send and Recv may proceed
	// concurrently from different goroutines.
	mu        sync.Mutex
	done      chan struct{}
	pendingCh chan []byte

	closeOnce sync.Once
	readErr   error
	started   bool
}

// Compile-time assertion that HTTPTransport satisfies Transport.
var _ Transport = (*HTTPTransport)(nil)

// Start opens the SSE long-lived connection to baseURL and spawns a
// background reader goroutine that parses Server-Sent Events and
// enqueues JSON-RPC frames onto pendingCh. The provided ctx governs
// the initial GET request; once the stream is established the
// connection persists until Stop is called.
func (t *HTTPTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return errors.New("mcp: transport already started")
	}
	if t.baseURL == "" {
		return errors.New("mcp: HTTPTransport.baseURL is empty")
	}
	if t.httpClient == nil {
		t.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	sendURL := t.sendURL
	if sendURL == "" {
		sendURL = t.baseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// The SSE connection is long-lived: a per-client timeout would
	// tear it down while idle, so use a client with no timeout.
	sseClient := &http.Client{}
	resp, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("mcp: SSE connect to %s failed: %s", t.baseURL, resp.Status)
	}

	t.resp = resp
	t.reader = bufio.NewReaderSize(resp.Body, 1024*1024)
	t.sendURL = sendURL
	t.done = make(chan struct{})
	t.pendingCh = make(chan []byte, 64)
	t.started = true

	go t.readLoop()
	return nil
}

// readLoop drains the SSE stream, parses `data:` events, and delivers
// each assembled JSON-RPC frame to pendingCh. It exits when the
// underlying connection is closed (by Stop or by the server) — the
// resulting read error is stored so Recv can surface it to callers.
func (t *HTTPTransport) readLoop() {
	var dataLines []string
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			// Flush a trailing partial line so a server that omits
			// the final newline still delivers its last event.
			if partial := strings.TrimRight(line, "\r\n"); partial != "" && !strings.HasPrefix(partial, ":") {
				if field, value := parseSSEField(partial); field == "data" {
					dataLines = append(dataLines, value)
				}
			}
			if len(dataLines) > 0 {
				t.enqueue([]byte(strings.Join(dataLines, "\n")))
			}
			t.closeDone(err)
			return
		}

		line = strings.TrimRight(line, "\r\n")

		// An empty line dispatches the current event.
		if line == "" {
			if len(dataLines) > 0 {
				if !t.enqueue([]byte(strings.Join(dataLines, "\n"))) {
					return
				}
				dataLines = nil
			}
			continue
		}

		// Comment lines start with ':' and are ignored.
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Only `data:` fields are processed; `event:`, `id:` and
		// `retry:` are recognized by the parser but discarded.
		if field, value := parseSSEField(line); field == "data" {
			dataLines = append(dataLines, value)
		}
	}
}

// enqueue delivers msg to pendingCh unless the transport has been
// shut down. It returns false when the transport is closing so the
// caller (readLoop) can exit promptly.
func (t *HTTPTransport) enqueue(msg []byte) bool {
	select {
	case <-t.done:
		return false
	case t.pendingCh <- msg:
		return true
	}
}

// parseSSEField splits an SSE line "field:value" into its field name
// and value, stripping a single leading space from the value per the
// SSE specification (so "data: x" and "data:x" both yield "x").
func parseSSEField(line string) (field, value string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	field = line[:idx]
	value = line[idx+1:]
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	return field, value
}

// closeDone signals shutdown via the done channel exactly once. The
// read error (if any) is stored so Recv can report why the transport
// died. Calling closeDone after Stop is a no-op.
func (t *HTTPTransport) closeDone(err error) {
	t.closeOnce.Do(func() {
		t.readErr = err
		close(t.done)
	})
}

// Send POSTs a single JSON-RPC frame to sendURL with
// Content-Type: application/json. HTTP error status codes (>= 400)
// are reported as errors.
func (t *HTTPTransport) Send(ctx context.Context, msg []byte) error {
	t.mu.Lock()
	started := t.started
	sendURL := t.sendURL
	client := t.httpClient
	t.mu.Unlock()

	if !started {
		return errors.New("mcp: transport not started")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("mcp: HTTP send to %s failed: %s", sendURL, resp.Status)
	}
	return nil
}

// Recv blocks until a complete JSON-RPC frame is available on the SSE
// stream and returns it. ctx cancellation aborts a pending read.
// When the transport has been stopped or the connection dropped, Recv
// returns the stored error (or a generic "closed" error).
func (t *HTTPTransport) Recv(ctx context.Context) ([]byte, error) {
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
		// Drain any messages buffered before shutdown so callers
		// don't lose responses already delivered by the server.
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

// Stop tears down the SSE connection and signals the background
// reader goroutine to exit. It is idempotent: calling Stop more than
// once is a no-op.
func (t *HTTPTransport) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.started {
		return nil
	}
	t.closeDone(nil)
	var err error
	if t.resp != nil {
		err = t.resp.Body.Close()
	}
	t.started = false
	return err
}
