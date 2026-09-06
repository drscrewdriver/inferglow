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

	"github.com/inferglow/builtins/actions"
)

// subagentRegistry is the shared spawn observation store. The spawn_agent
// action records into it (R9 Phase 0); the webui 任务管理 panel reads via
// GET /v1/subagents. Nil keeps the endpoints returning 503.
var subagentRegistry *actions.SubagentRegistry

// SetSubagentRegistry wires the shared registry (called from main.go, before
// agent assembly so the spawn_agent registration sees a non-nil store).
func SetSubagentRegistry(r *actions.SubagentRegistry) { subagentRegistry = r }

// handleListSubagents handles GET /v1/subagents?session= — list spawn records,
// optionally filtered by the owning chat session. Newest first.
func (s *Server) handleListSubagents(w http.ResponseWriter, r *http.Request) {
	if subagentRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "subagent registry not configured")
		return
	}
	spawns := subagentRegistry.List(r.URL.Query().Get("session"))
	writeJSON(w, http.StatusOK, map[string]any{"spawns": spawns, "count": len(spawns)})
}
