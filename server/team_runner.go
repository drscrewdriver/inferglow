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
	"context"
	"fmt"

	"github.com/inferglow/orchestrator/team"
)

// agentLikeRunner bridges server.AgentLike to team.AgentRunner.
// Both interfaces have the same Run(ctx, string) (string, error) signature,
// so this is a trivial 1:1 adapter.
type agentLikeRunner struct {
	like AgentLike
}

func (a *agentLikeRunner) Run(ctx context.Context, msg string) (string, error) {
	return a.like.Run(ctx, msg)
}

// TeamRunner builds team.Coordinator instances from TeamConfig + AgentStore.
type TeamRunner struct {
	agentStore AgentStore
	teamStore  *TeamStore
}

// NewTeamRunner creates a TeamRunner.
func NewTeamRunner(agentStore AgentStore, teamStore *TeamStore) *TeamRunner {
	return &TeamRunner{agentStore: agentStore, teamStore: teamStore}
}

// BuildCoordinator constructs a team.Coordinator from a stored TeamConfig.
// Each member's AgentID is resolved via the AgentStore; if any agent is
// missing, an error is returned.
func (tr *TeamRunner) BuildCoordinator(cfg *TeamConfig) (*team.Coordinator, error) {
	members := make([]team.Member, 0, len(cfg.Members))
	for _, mc := range cfg.Members {
		ag := tr.agentStore.Get(mc.AgentID)
		if ag == nil {
			return nil, fmt.Errorf("member %q: agent %q not found", mc.Role, mc.AgentID)
		}
		members = append(members, team.Member{
			Agent:   &agentLikeRunner{like: ag},
			Role:    mc.Role,
			Handoff: mc.Handoff,
		})
	}
	return team.NewCoordinator(members, team.WithMaxRounds(cfg.MaxRounds)), nil
}
