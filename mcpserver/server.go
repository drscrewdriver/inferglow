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
	"log"
)

// ToolDescriptor describes a tool exposed by the server.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolResult is the outcome of calling a tool.
type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent is a single content item in a tool result.
type ToolContent struct {
	Type string `json:"type"` // "text" | "image" | "resource_link"
	Text string `json:"text,omitempty"`
}

// ToolProvider is the interface the Server uses to discover and invoke tools.
type ToolProvider interface {
	// ListTools returns all available tool descriptors.
	ListTools() []ToolDescriptor

	// CallTool invokes a tool by name with the given arguments.
	CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error)
}

// Server is an MCP server that handles JSON-RPC 2.0 requests.
type Server struct {
	// Provider supplies tool listings and execution.
	Provider ToolProvider

	// initialized tracks whether the MCP handshake has completed.
	initialized bool
}

// NewServer creates a Server with the given tool provider.
func NewServer(provider ToolProvider) *Server {
	return &Server{Provider: provider}
}

// Serve runs the JSON-RPC protocol loop, reading requests from the
// transport and writing responses until the transport returns an error
// or the context is canceled.
func (s *Server) Serve(ctx context.Context, transport FrameTransport) error {
	defer transport.Close()

	for {
		data, err := transport.Recv(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return err
			}
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(data, &req); err != nil {
			resp := newErrorResponse(0, errCodeInvalid, "parse error")
			s.sendResponse(ctx, transport, resp)
			continue
		}

		resp := s.handleRequest(ctx, &req)
		if req.ID != 0 {
			s.sendResponse(ctx, transport, resp)
		}
	}
}

// HandleMessage processes a single JSON-RPC message and returns the response.
// This is useful for HTTP-based transports where each request is independent.
func (s *Server) HandleMessage(ctx context.Context, data []byte) *jsonRPCResponse {
	var req jsonRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return &jsonRPCResponse{
			JSONRPC: jsonRPCVersion,
			ID:      0,
			Error:   &jsonRPCError{Code: errCodeInvalid, Message: "parse error"},
		}
	}
	resp := s.handleRequest(ctx, &req)
	return &resp
}

func (s *Server) handleRequest(ctx context.Context, req *jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		s.initialized = true
		return newResponse(req.ID, map[string]any{})
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return newErrorResponse(req.ID, errCodeNotFound, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req *jsonRPCRequest) jsonRPCResponse {
	s.initialized = true
	result := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    serverName,
			"version": serverVersion,
		},
	}
	return newResponse(req.ID, result)
}

func (s *Server) handleToolsList(req *jsonRPCRequest) jsonRPCResponse {
	tools := s.Provider.ListTools()
	return newResponse(req.ID, map[string]any{
		"tools": tools,
	})
}

func (s *Server) handleToolsCall(ctx context.Context, req *jsonRPCRequest) jsonRPCResponse {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newErrorResponse(req.ID, errCodeInvalid, "invalid params")
	}

	result, err := s.Provider.CallTool(ctx, params.Name, params.Arguments)
	if err != nil {
		return newErrorResponse(req.ID, errCodeInternal, err.Error())
	}

	return newResponse(req.ID, result)
}

func (s *Server) sendResponse(ctx context.Context, transport FrameTransport, resp jsonRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("mcpserver: marshal response: %v", err)
		return
	}
	if err := transport.Send(ctx, data); err != nil {
		log.Printf("mcpserver: send response: %v", err)
	}
}
