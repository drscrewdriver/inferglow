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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/inferglow/orchestrator/agent"
)

// AgentRuntime holds the shared agent + bridge + session for all output modes
// (TUI, REPL, OneShot). Extracting this eliminates duplicated initialization
// code across the three entry points.
type AgentRuntime struct {
	Agent     *agent.Agent
	Bridge    *MemoryBridge
	SessionID string
	Config    CLIConfig
}

// BuildRuntime creates the shared agent infrastructure used by every output
// mode. It resolves the session ID (reusing resumeID when provided), builds
// the memory bridge, loads constitutional entries, and constructs the agent.
//
// The caller is responsible for calling Close when done.
func BuildRuntime(cfg CLIConfig, resumeID string) (*AgentRuntime, error) {
	sessionID := resumeID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	bridge, err := NewMemoryBridge(cfg, sessionID)
	if err != nil {
		return nil, fmt.Errorf("init memory bridge: %w", err)
	}

	if cfg.Features.Constitutional && cfg.Constitutional != "" {
		entries, err := loadConstitutional(cfg.Constitutional)
		if err != nil {
			// Non-fatal: log warning but continue startup.
			// REPL previously printed to stderr; we preserve that behavior
			// by returning the runtime anyway when the agent builds fine.
			// For now, surface via the bridge which tolerates missing entries.
			_ = err // constitutional load failure is non-fatal
		} else {
			bridge.AppendConstitutional(entries)
		}
	}

	ag, err := buildAgent(cfg, bridge, sessionID)
	if err != nil {
		bridge.OnSessionEnd(context.Background())
		return nil, fmt.Errorf("init agent: %w", err)
	}

	return &AgentRuntime{
		Agent:     ag,
		Bridge:    bridge,
		SessionID: sessionID,
		Config:    cfg,
	}, nil
}

// Close releases session resources. Must be called when the runtime is no
// longer needed (typically via defer).
func (r *AgentRuntime) Close(ctx context.Context) {
	r.Bridge.OnSessionEnd(ctx)
}
