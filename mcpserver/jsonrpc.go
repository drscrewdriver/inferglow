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
	"encoding/json"
	"fmt"
)

// JSON-RPC 2.0 protocol constants.
const (
	jsonRPCVersion  = "2.0"
	protocolVersion = "2024-11-05"
	serverName      = "inferglow-mcpserver"
	serverVersion   = "0.1.0"
	errCodeInternal = -32603
	errCodeNotFound = -32601
	errCodeInvalid  = -32600
)

// jsonRPCRequest is the wire shape of a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is the wire shape of a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// newResponse creates a successful JSON-RPC response.
func newResponse(id int64, result any) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// newErrorResponse creates a JSON-RPC error response.
func newErrorResponse(id int64, code int, message string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: message},
	}
}
