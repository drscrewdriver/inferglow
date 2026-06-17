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

package server

import (
	"net/http"

	"github.com/inferglow/server/handler"
	"github.com/inferglow/server/middleware"
)

// registerRoutes sets up all API routes.
func (s *Server) registerRoutes() {
	// Health check (no auth)
	s.mux.HandleFunc("GET /health", handler.Health)

	// API routes (with optional middleware)
	api := http.NewServeMux()

	// Agent CRUD + chat
	api.HandleFunc("GET /v1/agents", s.handleListAgents)
	api.HandleFunc("POST /v1/agents", s.handleCreateAgent)
	api.HandleFunc("GET /v1/agents/{id}", s.handleGetAgent)
	api.HandleFunc("DELETE /v1/agents/{id}", s.handleDeleteAgent)
	api.HandleFunc("POST /v1/agents/{id}/chat", s.handleChat)
	api.HandleFunc("POST /v1/agents/{id}/stream", s.handleStream)

	// Session
	api.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)

	// Tools
	api.HandleFunc("GET /v1/tools", s.handleListTools)

	// Memory
	api.HandleFunc("POST /v1/memories", s.handleCreateMemory)
	api.HandleFunc("GET /v1/memories", s.handleSearchMemory)

	// OpenAPI spec
	api.HandleFunc("GET /openapi.json", handler.OpenAPISpec)

	// Apply middleware chain
	var h http.Handler = api
	h = middleware.Logging(h)
	h = middleware.Recovery(h)
	if len(s.cfg.CORSOrigins) > 0 {
		h = middleware.CORS(h, s.cfg.CORSOrigins)
	}
	if s.cfg.APIKey != "" {
		h = middleware.APIKeyAuth(h, s.cfg.APIKey)
	}

	s.mux.Handle("/", h)
}
