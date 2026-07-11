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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/inferglow/cli"
)

// runTeam handles the "team" subcommand.
func runTeam(ctx context.Context, cfg cli.CLIConfig, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: inferglow team run --roles=planner,coder,reviewer <task>")
	}

	switch args[0] {
	case "run":
		return runTeamRun(ctx, cfg, args[1:])
	default:
		return fmt.Errorf("unknown team subcommand: %s", args[0])
	}
}

// runTeamRun builds a team and executes a coordination round.
func runTeamRun(ctx context.Context, cfg cli.CLIConfig, args []string) error {
	fs := flag.NewFlagSet("team run", flag.ExitOnError)
	rolesRaw := fs.String("roles", "planner,coder,reviewer", "Comma-separated roles (optional handoff: planner>coder)")
	maxRounds := fs.Int("rounds", 3, "Maximum coordination rounds")
	_ = fs.Parse(args)

	task := strings.Join(fs.Args(), " ")
	if task == "" {
		return fmt.Errorf("usage: inferglow team run --roles=... <task prompt>")
	}

	roles := cli.ParseRoles(*rolesRaw)
	if len(roles) == 0 {
		return fmt.Errorf("no valid roles specified")
	}

	fmt.Fprintf(os.Stderr, "Building team with roles: %s\n", formatRoles(roles))

	coord, agents, err := cli.BuildTeam(cfg, roles, "team-run")
	if err != nil {
		return fmt.Errorf("build team: %w", err)
	}
	_ = agents // agents available for future inspection/cleanup

	// Apply maxRounds override (default already set in BuildTeam).
	_ = maxRounds // TODO: pass as option to BuildTeam

	fmt.Fprintf(os.Stderr, "Running coordination round...\n\n")

	result, err := coord.Round(ctx, task)
	if err != nil {
		return fmt.Errorf("team round: %w", err)
	}

	// Print per-member outputs.
	for role, output := range result.MemberOutputs {
		fmt.Printf("--- %s ---\n%s\n\n", role, output)
	}

	// Print final consolidated response.
	fmt.Printf("=== Final Response (%d rounds) ===\n%s\n", result.Rounds, result.FinalResponse)

	return nil
}

func formatRoles(roles []cli.TeamMemberRole) string {
	parts := make([]string, len(roles))
	for i, r := range roles {
		parts[i] = r.Role
		if len(r.Handoff) > 0 {
			parts[i] += ">" + strings.Join(r.Handoff, ",")
		}
	}
	return strings.Join(parts, ", ")
}
