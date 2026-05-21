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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// JSON-RPC 2.0 protocol constants and error codes.
const (
	jsonRPCVersion  = "2.0"
	protocolVersion = "2024-11-05"
	clientName      = "inferglow"
	clientVersion   = "0.1.0"
	errCodeInternal = -32603
)

// jsonRPCRequest is the wire shape of a JSON-RPC 2.0 request or
// notification. Requests carry an ID; notifications omit it (the
// encoder leaves ID at the zero value and the omitempty tag drops it).
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse is the wire shape of a JSON-RPC 2.0 response. Either
// Result or Error is populated; never both.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Client is a minimal MCP / JSON-RPC 2.0 client bound to a single
// Transport. It multiplexes outbound requests onto the transport's
// write side and demultiplexes inbound responses onto per-id
// channels via a background reader goroutine.
//
// A Client is safe for concurrent use by multiple goroutines once
// NewClient has returned.
type Client struct {
	transport Transport

	reqID   int64 // atomic; first allocated id is 1
	mu      sync.Mutex
	pending map[int64]chan *jsonRPCResponse

	startOnce sync.Once
	done      chan struct{}
	closed    atomic.Bool
}

// NewClient returns a Client wired to t. The caller is responsible
// for invoking t.Start before issuing requests, although doing so
// lazily on the first request is also acceptable. NewClient starts a
// background reader goroutine that exits when Close is called or the
// transport returns an unrecoverable error from Recv.
func NewClient(t Transport) *Client {
	c := &Client{
		transport: t,
		pending:   make(map[int64]chan *jsonRPCResponse),
		done:      make(chan struct{}),
	}
	c.startReadLoop()
	return c
}

// startReadLoop launches the background goroutine that drains
// transport.Recv and routes each response to its registered pending
// channel. Notifications (responses with id == 0) are dropped: this
// client never registers interest in them.
func (c *Client) startReadLoop() {
	c.startOnce.Do(func() {
		go c.readLoop()
	})
}

func (c *Client) readLoop() {
	for {
		if c.closed.Load() {
			return
		}
		// Use a fresh context per Recv so that an individual read
		// is not tied to any specific request's lifetime. The loop
		// exits when Close signals done or when the transport
		// returns a terminal error.
		ctx, cancel := context.WithCancel(context.Background())
		// Tie cancellation to the client's done channel so Close
		// interrupts a blocking Recv where the transport supports it.
		go func() {
			select {
			case <-c.done:
				cancel()
			case <-ctx.Done():
			}
		}()
		data, err := c.transport.Recv(ctx)
		cancel()
		if err != nil {
			// Distinguish "transport closed cleanly" from real
			// errors. Either way the loop exits — further Recv
			// calls would fail identically.
			select {
			case <-c.done:
				return
			default:
			}
			// On transport error, fail every pending request so
			// callers don't block forever.
			c.failAll(err)
			return
		}
		if len(data) == 0 {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			// Not a JSON-RPC response (e.g. server log spam on
			// stdout). Skip — the protocol is line-delimited and
			// stray lines are not actionable.
			continue
		}
		if resp.ID == 0 {
			// Notification from server; out of scope for this client.
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
		if !ok {
			continue
		}
		select {
		case ch <- &resp:
		default:
			// Channel has buffer 1; if it's already full the caller
			// has timed out and abandoned the request. Drop it.
		}
	}
}

// failAll delivers err to every outstanding pending channel. Used
// when the transport dies and readLoop is exiting.
func (c *Client) failAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- &jsonRPCResponse{
			ID:    id,
			Error: &jsonRPCError{Code: errCodeInternal, Message: err.Error()},
		}:
		default:
		}
		delete(c.pending, id)
	}
}

// sendRequest writes a JSON-RPC 2.0 request with the given method
// and params, then blocks until the matching response arrives or ctx
// is canceled. On success the raw result payload is returned for the
// caller to unmarshal into a typed shape.
func (c *Client) sendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, errors.New("mcp: client closed")
	}
	id := atomic.AddInt64(&c.reqID, 1)
	req := jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}

	ch := make(chan *jsonRPCResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.transport.Send(ctx, data); err != nil {
		return nil, fmt.Errorf("mcp: send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("mcp: %s: %w", method, ctx.Err())
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp: %s: %w", method, resp.Error)
		}
		return resp.Result, nil
	}
}

// sendNotification writes a JSON-RPC 2.0 notification (no id, no
// response expected) with the given method and params.
func (c *Client) sendNotification(ctx context.Context, method string, params any) error {
	if c.closed.Load() {
		return errors.New("mcp: client closed")
	}
	req := jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mcp: marshal notification: %w", err)
	}
	if err := c.transport.Send(ctx, data); err != nil {
		return fmt.Errorf("mcp: send notification %s: %w", method, err)
	}
	return nil
}

// Initialize performs the MCP initialize handshake: it sends the
// `initialize` request with this client's protocolVersion /
// capabilities / clientInfo, parses the server's ServerInfo from the
// result, then sends the `notifications/initialized` notification to
// complete the handshake. The returned ServerInfo includes the
// server's advertised capabilities merged from the top-level
// `capabilities` field of the initialize result.
func (c *Client) Initialize(ctx context.Context) (*ServerInfo, error) {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    clientName,
			"version": clientVersion,
		},
	}
	raw, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	var resp struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("mcp: parse initialize result: %w", err)
	}

	if err := c.sendNotification(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return nil, err
	}

	return &ServerInfo{
		Name:         resp.ServerInfo.Name,
		Version:      resp.ServerInfo.Version,
		Capabilities: resp.Capabilities,
	}, nil
}

// ListTools issues a `tools/list` request and returns the slice of
// Tools the server exposes.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.sendRequest(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/list result: %w", err)
	}
	return resp.Tools, nil
}

// CallTool issues a `tools/call` request to invoke the named tool
// with the given arguments and returns the Content items the server
// produced.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) ([]Content, error) {
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	raw, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Content []Content `json:"content"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/call result: %w", err)
	}
	return resp.Content, nil
}

// Close signals the background reader goroutine to exit and asks
// the transport to stop. After Close returns the Client must not be
// reused.
func (c *Client) Close(ctx context.Context) error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(c.done)
	return c.transport.Stop(ctx)
}
