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

import "errors"

// ErrUnsupportedTransport is returned by NewTransportFromConfig when
// cfg.Transport names a transport this package does not implement.
//
// P0 supports only "stdio"; HTTP and SSE are P1.
var ErrUnsupportedTransport = errors.New("mcp: unsupported transport")

// NewTransportFromConfig builds a Transport from an MCPServerConfig.
//
// In P0 only the "stdio" transport is implemented: it returns a
// freshly-constructed *StdioTransport wired to cfg.Command / cfg.Args
// / cfg.Env. Callers must still invoke Start on the returned
// transport before using it.
func NewTransportFromConfig(cfg MCPServerConfig) (Transport, error) {
	switch cfg.Transport {
	case "stdio", "":
		// Treat empty Transport as stdio to make hand-written
		// configs more forgiving.
		return &StdioTransport{
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
		}, nil
	case "sse":
		return &HTTPTransport{
			baseURL: cfg.Endpoint,
			sendURL: cfg.Endpoint,
		}, nil
	case "streamable-http":
		return &StreamableHTTPTransport{
			Endpoint: cfg.Endpoint,
		}, nil
	default:
		return nil, ErrUnsupportedTransport
	}
}
