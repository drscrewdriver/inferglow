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

package cli

import (
	"fmt"
	"strings"

	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/orchestrator/team"
)

// TeamMemberRole maps a role name to an optional handoff list.
type TeamMemberRole struct {
	Role    string
	Handoff []string
}

// BuildTeam creates a team.Coordinator from a list of role names.
// Each role gets an independent agent built via buildAgent.
// The bridge is shared for memory/context; each agent gets its own session.
func BuildTeam(cfg CLIConfig, roles []TeamMemberRole, sessionPrefix string) (*team.Coordinator, []*agent.Agent, error) {
	members := make([]team.Member, 0, len(roles))
	agents := make([]*agent.Agent, 0, len(roles))

	for i, r := range roles {
		sessionID := fmt.Sprintf("%s-%s-%d", sessionPrefix, r.Role, i)
		bridge, err := NewMemoryBridge(cfg, sessionID)
		if err != nil {
			return nil, nil, fmt.Errorf("team build: memory bridge for %s: %w", r.Role, err)
		}

		ag, err := buildAgent(cfg, bridge, sessionID)
		if err != nil {
			return nil, nil, fmt.Errorf("team build: agent for %s: %w", r.Role, err)
		}

		adapted := team.AdaptAgent(ag)
		members = append(members, team.Member{
			Agent:   adapted,
			Role:    r.Role,
			Handoff: r.Handoff,
		})
		agents = append(agents, ag)
	}

	coord := team.NewCoordinator(members, team.WithMaxRounds(3))
	return coord, agents, nil
}

// ParseRoles parses a comma-separated role list into TeamMemberRole slices.
// Optional handoff syntax: "planner>coder,coder>reviewer" means planner hands
// off to coder, coder hands off to reviewer.
func ParseRoles(raw string) []TeamMemberRole {
	parts := strings.Split(raw, ",")
	roles := make([]TeamMemberRole, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		r := TeamMemberRole{Role: p}
		// Check for handoff syntax: role>target
		if idx := strings.Index(p, ">"); idx >= 0 {
			r.Role = strings.TrimSpace(p[:idx])
			target := strings.TrimSpace(p[idx+1:])
			if target != "" {
				r.Handoff = []string{target}
			}
		}
		roles = append(roles, r)
	}
	return roles
}
