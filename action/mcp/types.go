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
//                       and MimeType describes its format.
//   - "resource_link" — URI and Name identify a server-side resource
//                       the caller may resolve separately.
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
type MCPServerConfig struct {
	Transport string   `json:"transport"` // "stdio" (only supported value)
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Env       []string `json:"env"`
}
