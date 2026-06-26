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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// sseSendChSize is the buffer capacity for outbound SSE events per session.
// Chosen to cover burst responses without unbounded memory growth.
const sseSendChSize = 64

// sseKeepaliveInterval is how often the server sends a comment frame to
// prevent reverse proxies from closing idle SSE connections.
const sseKeepaliveInterval = 30 * time.Second

// sseMaxSessions is the default maximum number of concurrent SSE connections.
const sseMaxSessions = 1000

// SSEFrameTransport implements FrameTransport using SSE for server-to-client
// pushes and HTTP POST for client-to-server requests.
//
// Send writes to an internal channel that the SSE goroutine drains and
// writes as `data:` events. Recv reads from a channel populated by the
// POST handler. Close is idempotent.
type SSEFrameTransport struct {
	sendCh chan []byte
	recvCh chan []byte
	done   chan struct{}
	closeOnce sync.Once
	closed atomic.Bool
}

// newSSEFrameTransport creates a transport ready for use.
func newSSEFrameTransport() *SSEFrameTransport {
	return &SSEFrameTransport{
		sendCh: make(chan []byte, sseSendChSize),
		recvCh: make(chan []byte, sseSendChSize),
		done:   make(chan struct{}),
	}
}

// Send enqueues a JSON-RPC response for delivery via SSE. If the channel
// is full, the oldest message is dropped and the new one is enqueued.
func (t *SSEFrameTransport) Send(_ context.Context, data []byte) error {
	if t.closed.Load() {
		return fmt.Errorf("transport closed")
	}
	// Non-blocking send: drop oldest if full.
	select {
	case t.sendCh <- data:
	default:
		// Channel full — drop oldest to make room.
		select {
		case <-t.sendCh:
		default:
		}
		select {
		case t.sendCh <- data:
		default:
			return fmt.Errorf("send channel full after drop")
		}
	}
	return nil
}

// Recv blocks until a client POST delivers a JSON-RPC request or the
// transport is closed.
func (t *SSEFrameTransport) Recv(ctx context.Context) ([]byte, error) {
	select {
	case data := <-t.recvCh:
		return data, nil
	case <-t.done:
		return nil, fmt.Errorf("transport closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close shuts down the transport. Safe to call multiple times.
func (t *SSEFrameTransport) Close() error {
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		close(t.done)
	})
	return nil
}

// deliver enqueues an incoming message from the POST handler into the
// recv channel for consumption by Server.Serve.
func (t *SSEFrameTransport) deliver(data []byte) error {
	if t.closed.Load() {
		return fmt.Errorf("transport closed")
	}
	select {
	case t.recvCh <- data:
		return nil
	case <-t.done:
		return fmt.Errorf("transport closed")
	}
}

// sseSessionManager tracks active SSE sessions.
type sseSessionManager struct {
	sessions sync.Map // sessionID → *SSEFrameTransport
	count    atomic.Int64
	max      int64
	nextID   atomic.Int64
}

func newSSESessionManager(max int) *sseSessionManager {
	return &sseSessionManager{max: int64(max)}
}

func (m *sseSessionManager) register(t *SSEFrameTransport) (string, error) {
	if m.count.Load() >= m.max {
		return "", fmt.Errorf("max SSE sessions reached (%d)", m.max)
	}
	id := fmt.Sprintf("sse-%d", m.nextID.Add(1))
	m.sessions.Store(id, t)
	m.count.Add(1)
	return id, nil
}

func (m *sseSessionManager) lookup(id string) *SSEFrameTransport {
	v, ok := m.sessions.Load(id)
	if !ok {
		return nil
	}
	return v.(*SSEFrameTransport)
}

func (m *sseSessionManager) unregister(id string) {
	if _, loaded := m.sessions.LoadAndDelete(id); loaded {
		m.count.Add(-1)
	}
}

// SSEHandler returns an http.Handler implementing the MCP SSE transport.
//
// Endpoints:
//
//	GET  /sse       — establishes an SSE connection; the server pushes
//	                  JSON-RPC responses as `data:` events.
//	POST /messages   — accepts a JSON-RPC request body. The query parameter
//	                  `sessionId` identifies the target SSE session.
func (s *Server) SSEHandler() http.Handler {
	return s.SSEHandlerWithMax(sseMaxSessions)
}

// SSEHandlerWithMax is like SSEHandler but allows configuring the maximum
// number of concurrent SSE connections.
func (s *Server) SSEHandlerWithMax(maxSessions int) http.Handler {
	mgr := newSSESessionManager(maxSessions)

	mux := http.NewServeMux()

	// GET /sse — establish SSE connection.
	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, r *http.Request) {
		transport := newSSEFrameTransport()
		sessionID, err := mgr.register(transport)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer mgr.unregister(sessionID)
		defer transport.Close()

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Send the session endpoint URL as the first SSE event so the
		// client knows where to POST messages.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fmt.Fprintf(w, "event: endpoint\ndata: /messages?sessionId=%s\n\n", sessionID)
		flusher.Flush()

		// Start the protocol loop in a goroutine.
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() {
			// Serve blocks until Recv returns an error (transport closed).
			_ = s.Serve(ctx, transport)
		}()

		// Drain sendCh and write SSE events until the client disconnects.
		keepalive := time.NewTicker(sseKeepaliveInterval)
		defer keepalive.Stop()
		for {
			select {
			case data := <-transport.sendCh:
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-keepalive.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			case <-ctx.Done():
				return
			}
		}
	})

	// POST /messages — accept JSON-RPC request for a specific session.
	mux.HandleFunc("POST /messages", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("sessionId")
		if sessionID == "" {
			http.Error(w, "missing sessionId", http.StatusBadRequest)
			return
		}

		transport := mgr.lookup(sessionID)
		if transport == nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Process the message and send the response via the SSE transport.
		resp := s.HandleMessage(r.Context(), body)
		respData, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "failed to marshal response", http.StatusInternalServerError)
			return
		}

		if err := transport.Send(r.Context(), respData); err != nil {
			http.Error(w, "failed to deliver response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"status":"accepted"}`)
	})

	return mux
}

// ServeSSE starts an HTTP server with SSE transport on the given address.
// It blocks until the server is shut down.
func ServeSSE(s *Server, addr string) error {
	handler := s.SSEHandler()
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	log.Printf("mcpserver: SSE transport listening on %s", addr)
	return srv.ListenAndServe()
}
