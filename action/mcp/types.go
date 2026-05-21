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

// Package mcp implements a minimal Model Context Protocol (MCP) client
// over stdio using JSON-RPC 2.0.
//
// This package deliberately avoids any third-party MCP SDK. It depends
// only on the Go standard library and exposes the small subset of the
// protocol required by Inferglow's Action Runtime: stdio transport,
// the initialize / tools/list / tools/call methods, and the basic
// content types returned by tool calls.
package mcp

// Tool describes a single tool exposed by an MCP server.
//
// InputSchema is an opaque JSON Schema fragment (a map[string]any)
// describing the arguments the tool accepts. It is copied verbatim
// into the Action.Schema of any Action created via action.NewMCPAction.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Content represents a single content item returned from a tools/call
// invocation. The Type field discriminates between the three shapes
// supported by this implementation:
//
//   - "text"          — Text holds the textual result.
//   - "image"         — Data holds the base64-encoded image payload
//     and MimeType describes its format.
//   - "resource_link" — URI and Name identify a server-side resource
//     the caller may resolve separately.
type Content struct {
	Type     string `json:"type"` // "text" | "image" | "resource_link"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"` // base64 for image
	MimeType string `json:"mimeType,omitempty"`
	// resource_link fields
	URI  string `json:"uri,omitempty"`
	Name string `json:"name,omitempty"`
}

// ServerInfo is the subset of the MCP initialize response surfaced to
// callers. Capabilities mirrors the top-level "capabilities" map sent
// by the server during the initialize handshake.
type ServerInfo struct {
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Capabilities map[string]any `json:"capabilities"`
}

// MCPServerConfig describes how to construct a Transport for an MCP
// server. Only the "stdio" transport is supported by this package;
// HTTP/SSE is intentionally out of scope (P1).
type MCPServerConfig struct { //nolint:revive
	Transport string   `json:"transport"` // "stdio" (only supported value)
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Env       []string `json:"env"`
}
