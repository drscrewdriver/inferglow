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
)

// AgentStatusProvider is the interface for agents that expose TurnLoop state.
// agent.Agent satisfies this interface via its TurnPhase() method.
type AgentStatusProvider interface {
	TurnPhase() string
}

// AgentCancelChecker is the interface for agents that expose cancel state.
type AgentCancelChecker interface {
	HasPendingCancel() bool
}

// handleAgentStatus returns the runtime status of an agent, including
// the current TurnLoop phase and cancel-pending flag.
// GET /v1/agents/{id}/status
func (s *Server) handleAgentStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ag := s.agentStore.Get(id)
	if ag == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	status := map[string]any{
		"agent_id":    id,
		"turn_phase":  "unknown",
		"cancel_pending": false,
	}

	// Extract TurnLoop phase if the agent exposes it.
	if sp, ok := ag.(AgentStatusProvider); ok {
		status["turn_phase"] = sp.TurnPhase()
	}

	// Extract cancel-pending flag if available.
	if cc, ok := ag.(AgentCancelChecker); ok {
		status["cancel_pending"] = cc.HasPendingCancel()
	}

	writeJSON(w, http.StatusOK, status)
}
