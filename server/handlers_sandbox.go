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
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/inferglow/sandbox"
)

// sandboxPermPreset is one predefined permission level exposed to the sidebar.
type sandboxPermPreset struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// sandboxRejection records a tool call that hit a sandbox boundary and was
// refused (later surfaceable for upgrade approval).
type sandboxRejection struct {
	ID         string    `json:"id"`
	Capability string    `json:"capability"`
	Reason     string    `json:"reason"`
	Timestamp  time.Time `json:"timestamp"`
}

// sandboxRegistry is a thin in-memory registry for the /v1/sandbox endpoints.
// It enumerates the sandbox module's runtimes, exposes permission presets, and
// records rejections. The sandbox module itself exposes no preset getter, so
// this server-side registry backs the API.
type sandboxRegistry struct {
	mu         sync.RWMutex
	runtimes   []string
	presets    []sandboxPermPreset
	active     string
	runtime    string
	rejections []sandboxRejection
	seq        int
}

// newSandboxRegistry builds a default registry: all five sandbox runtimes and
// the three permission presets, active preset = workspace_write.
func newSandboxRegistry() *sandboxRegistry {
	r := &sandboxRegistry{
		runtimes: []string{
			string(sandbox.ModeTrustedLocal),
			string(sandbox.ModeLocal),
			string(sandbox.ModeDocker),
			string(sandbox.ModeGVisor),
			string(sandbox.ModeAuto),
		},
		presets: []sandboxPermPreset{
			{ID: "read_only", Description: "read-only filesystem, no writes"},
			{ID: "workspace_write", Description: "writes confined to the workspace root"},
			{ID: "full_access", Description: "unrestricted filesystem access"},
		},
		active:  "workspace_write",
		runtime: string(sandbox.ModeAuto),
	}
	return r
}

// sandboxRegistry lazily resolves the server's sandbox registry, creating a
// default in-memory one when none was injected.
func (s *Server) sbx() *sandboxRegistry {
	if s.sbxRegistry == nil {
		s.sbxRegistry = newSandboxRegistry()
	}
	return s.sbxRegistry
}

// handleListSandboxRuntimes handles GET /v1/sandbox/runtimes — enumerate the
// supported sandbox runtimes.
func (s *Server) handleListSandboxRuntimes(w http.ResponseWriter, _ *http.Request) {
	r := s.sbx()
	r.mu.RLock()
	defer r.mu.RUnlock()
	runtimes := make([]map[string]string, 0, len(r.runtimes))
	for _, id := range r.runtimes {
		runtimes = append(runtimes, map[string]string{"id": id})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runtimes": runtimes,
		"default":  string(sandbox.ModeAuto),
	})
}

// handleSandboxPresets handles GET /v1/sandbox/presets — return the predefined
// permission presets plus the currently active preset/runtime.
func (s *Server) handleSandboxPresets(w http.ResponseWriter, _ *http.Request) {
	r := s.sbx()
	r.mu.RLock()
	defer r.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"presets": r.presets,
		"default": "workspace_write",
		"active":  r.active,
		"runtime": r.runtime,
	})
}

// handleSandboxSetPreset handles POST /v1/sandbox/preset — set the active
// permission preset (and optionally the runtime).
func (s *Server) handleSandboxSetPreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Preset  string `json:"preset"`
		Runtime string `json:"runtime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Preset == "" {
		writeError(w, http.StatusBadRequest, "preset is required")
		return
	}
	sb := s.sbx()
	sb.mu.Lock()
	if !sb.validPreset(req.Preset) {
		sb.mu.Unlock()
		writeError(w, http.StatusBadRequest, "unknown preset: "+req.Preset)
		return
	}
	sb.active = req.Preset
	if req.Runtime != "" {
		sb.runtime = req.Runtime
	}
	active, runtime := sb.active, sb.runtime
	sb.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"preset": active,
		"runtime": runtime,
		"status": "set",
	})
}

func (r *sandboxRegistry) validPreset(id string) bool {
	for _, p := range r.presets {
		if p.ID == id {
			return true
		}
	}
	return false
}

// handleListSandboxRejections handles GET /v1/sandbox/rejections — list the
// recorded sandbox-boundary refusals.
func (s *Server) handleListSandboxRejections(w http.ResponseWriter, _ *http.Request) {
	r := s.sbx()
	r.mu.RLock()
	rejections := append([]sandboxRejection{}, r.rejections...)
	r.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"rejections": rejections,
		"count":      len(rejections),
	})
}

// handleRecordSandboxRejection handles POST /v1/sandbox/rejections — record a
// sandbox-boundary refusal for later upgrade approval.
func (s *Server) handleRecordSandboxRejection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Capability string `json:"capability"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Capability == "" {
		writeError(w, http.StatusBadRequest, "capability is required")
		return
	}
	sb := s.sbx()
	sb.mu.Lock()
	sb.seq++
	rec := sandboxRejection{
		ID:         "rej-" + strconv.Itoa(sb.seq),
		Capability: req.Capability,
		Reason:     req.Reason,
		Timestamp:  time.Now().UTC(),
	}
	sb.rejections = append(sb.rejections, rec)
	sb.mu.Unlock()
	writeJSON(w, http.StatusCreated, rec)
}