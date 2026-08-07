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

package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

var startTime = time.Now()

// Health returns server health status.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"uptime_s":  int(time.Since(startTime).Seconds()),
		"go_version": runtime.Version(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// OpenAPISpec returns a minimal OpenAPI 3.0 specification.
func OpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "InferGlow API",
			"description": "InferGlow Agent Framework REST API",
			"version":     "6.0.0",
		},
		"paths": map[string]any{
			"/health": map[string]any{
				"get": map[string]any{
					"summary":     "Health check",
					"operationId": "healthCheck",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
			"/v1/agents": map[string]any{
				"get": map[string]any{
					"summary":     "List agents",
					"operationId": "listAgents",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
				"post": map[string]any{
					"summary":     "Create agent",
					"operationId": "createAgent",
					"responses": map[string]any{
						"201": map[string]any{"description": "Created"},
					},
				},
			},
			"/v1/agents/{id}": map[string]any{
				"get": map[string]any{
					"summary":     "Get agent",
					"operationId": "getAgent",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
						"404": map[string]any{"description": "Not Found"},
					},
				},
			},
			"/v1/agents/{id}/chat": map[string]any{
				"post": map[string]any{
					"summary":     "Chat with agent",
					"operationId": "chatWithAgent",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
			"/v1/agents/{id}/stream": map[string]any{
				"post": map[string]any{
					"summary":     "Stream chat with agent (SSE)",
					"operationId": "streamChatWithAgent",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "SSE stream"},
					},
				},
			},
			"/v1/tools": map[string]any{
				"get": map[string]any{
					"summary":     "List tools",
					"operationId": "listTools",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
			"/v1/memories": map[string]any{
				"get": map[string]any{
					"summary":     "Search memories",
					"operationId": "searchMemories",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
				"post": map[string]any{
					"summary":     "Create memory",
					"operationId": "createMemory",
					"responses": map[string]any{
						"201": map[string]any{"description": "Created"},
					},
				},
			},
			"/v1/sessions": map[string]any{
				"get": map[string]any{
					"summary":     "List sessions",
					"operationId": "listSessions",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
				"post": map[string]any{
					"summary":     "Create session",
					"operationId": "createSession",
					"responses": map[string]any{
						"201": map[string]any{"description": "Created"},
					},
				},
			},
			"/v1/sessions/{id}": map[string]any{
				"get": map[string]any{
					"summary":     "Get session",
					"operationId": "getSession",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
						"404": map[string]any{"description": "Not Found"},
					},
				},
				"patch": map[string]any{
					"summary":     "Update session metadata (title/group/pinned/status)",
					"operationId": "updateSessionMeta",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
						"404": map[string]any{"description": "Not Found"},
					},
				},
				"delete": map[string]any{
					"summary":     "Delete session",
					"operationId": "deleteSession",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(spec)
}
