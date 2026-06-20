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
	"encoding/json"
	"fmt"
)

// JSON-RPC 2.0 protocol constants and error codes.
const (
	JSONRPCVersion  = "2.0"
	ProtocolVersion = "2024-11-05"
	ClientName      = "inferglow"
	ClientVersion   = "0.1.0"
	ErrCodeInternal = -32603
	ErrCodeNotFound = -32601
	ErrCodeInvalid  = -32600
)

// JSONRPCRequest is the wire shape of a JSON-RPC 2.0 request or
// notification. Requests carry an ID; notifications omit it (the
// encoder leaves ID at the zero value and the omitempty tag drops it).
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// JSONRPCResponse is the wire shape of a JSON-RPC 2.0 response. Either
// Result or Error is populated; never both.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}
