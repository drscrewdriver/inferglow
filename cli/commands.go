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
	"sort"
	"strings"
	"time"

	"github.com/inferglow/audit"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/session"
)

// CommandFunc is a slash command handler. Returns true if the REPL should quit.
type CommandFunc func(ctx context.Context, args string, ag *agent.Agent, bridge *MemoryBridge, cfg CLIConfig, rt *AgentRuntime) bool

// commandTable maps command names to handlers.
var commandTable = map[string]CommandFunc{
	"help":        cmdHelp,
	"memory":      cmdMemory,
	"compact":     cmdCompact,
	"quit":        cmdQuit,
	"exit":        cmdQuit,
	"audit":       cmdAudit,
	"cost":        cmdCost,
	"cache-stats":  cmdCacheStats,
	"cache-report": cmdCacheReport,
}

// dispatchCommand parses and dispatches a slash command.
// Returns true if the REPL should quit.
func dispatchCommand(ctx context.Context, line string, ag *agent.Agent, bridge *MemoryBridge, cfg CLIConfig, rt *AgentRuntime) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}

	cmdName := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	handler, ok := commandTable[cmdName]
	if !ok {
		fmt.Printf("Unknown command: /%s. Type /help for available commands.\n", cmdName)
		return false
	}

	return handler(ctx, args, ag, bridge, cfg, rt)
}

func cmdHelp(_ context.Context, _ string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig, _ *AgentRuntime) bool {
	fmt.Println(`Available commands:
  /help                  Show this help message
  /memory search <q>     Search memory for a query
  /memory stats          Show memory statistics
  /compact               Manually trigger context compression
  /audit query [flags]   Query audit trail
  /audit stats           Show audit statistics
  /cost                  Show session cost summary
  /cache-stats           Show cache hit statistics
  /cache-report [flags]  Show cross-session cache efficiency report
  /quit                  End session and exit`)
	return false
}

func cmdMemory(ctx context.Context, args string, _ *agent.Agent, bridge *MemoryBridge, _ CLIConfig, _ *AgentRuntime) bool {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 {
		fmt.Println("Usage: /memory search <query> | /memory stats")
		return false
	}

	switch parts[0] {
	case "search":
		if len(parts) < 2 || parts[1] == "" {
			fmt.Println("Usage: /memory search <query>")
			return false
		}
		results, err := bridge.SearchMemory(ctx, parts[1])
		if err != nil {
			fmt.Printf("Memory search error: %v\n", err)
			return false
		}
		if len(results) == 0 {
			fmt.Println("No memories found.")
			return false
		}
		for _, r := range results {
			fmt.Printf("  [%s] (%s, confidence: %.2f) %s\n",
				r.MemID, r.Category, r.Confidence, strings.Join(r.Facts, "; "))
		}

	case "stats":
		stats := bridge.Stats()
		fmt.Printf("Memory stats:\n")
		fmt.Printf("  Total steps:   %d\n", stats.TotalSteps)
		fmt.Printf("  Active steps:  %d\n", stats.ActiveSteps)
		fmt.Printf("  Total tokens:  %d\n", stats.TotalTokens)

	default:
		fmt.Printf("Unknown memory subcommand: %s\n", parts[0])
	}

	return false
}

func cmdCompact(ctx context.Context, _ string, _ *agent.Agent, bridge *MemoryBridge, _ CLIConfig, _ *AgentRuntime) bool {
	if err := bridge.Compact(ctx); err != nil {
		fmt.Printf("Compression error: %v\n", err)
	} else {
		fmt.Println("Compression triggered.")
	}
	return false
}

func cmdQuit(_ context.Context, _ string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig, _ *AgentRuntime) bool {
	fmt.Println("Goodbye!")
	return true
}

// ---------------------------------------------------------------------------
// /audit command
// ---------------------------------------------------------------------------

func cmdAudit(_ context.Context, args string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig, rt *AgentRuntime) bool {
	if rt.AuditChain == nil || !rt.AuditChain.IsEnabled() {
		fmt.Println("Audit trail is not enabled. Set audit.enabled = true in config.")
		return false
	}

	parts := strings.Fields(args)
	if len(parts) == 0 {
		fmt.Println("Usage: /audit query [--source=] [--action=] [--from=] [--to=] | /audit stats")
		return false
	}

	switch parts[0] {
	case "query":
		return cmdAuditQuery(rt.AuditChain, parts[1:])
	case "stats":
		return cmdAuditStats(rt.AuditChain)
	default:
		fmt.Printf("Unknown audit subcommand: %s\n", parts[0])
		return false
	}
}

func cmdAuditQuery(chain *audit.AuditChain, args []string) bool {
	var filter audit.QueryFilter
	for _, arg := range args {
		if strings.HasPrefix(arg, "--source=") {
			filter.Source = strings.TrimPrefix(arg, "--source=")
		} else if strings.HasPrefix(arg, "--action=") {
			filter.Action = strings.TrimPrefix(arg, "--action=")
		} else if strings.HasPrefix(arg, "--from=") {
			val := strings.TrimPrefix(arg, "--from=")
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				fmt.Printf("Invalid --from value %q: expected RFC3339 format (e.g. 2026-01-01T00:00:00Z)\n", val)
				return false
			}
			filter.From = t
		} else if strings.HasPrefix(arg, "--to=") {
			val := strings.TrimPrefix(arg, "--to=")
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				fmt.Printf("Invalid --to value %q: expected RFC3339 format (e.g. 2026-01-01T00:00:00Z)\n", val)
				return false
			}
			filter.To = t
		} else {
			fmt.Printf("Unknown flag: %s\n", arg)
			return false
		}
	}

	entries, err := chain.Query(filter)
	if err != nil {
		fmt.Printf("Audit query error: %v\n", err)
		return false
	}

	if len(entries) == 0 {
		fmt.Println("No matching audit entries found.")
		return false
	}

	fmt.Printf("Audit entries (%d):\n", len(entries))
	for _, e := range entries {
		ts := e.Timestamp.Format(time.RFC3339)
		id := e.ID
		if len(id) > 16 {
			id = id[:16]
		}
		fmt.Printf("  [%s] %s | source=%s action=%s\n", ts, id, e.Source, e.Action)
		if e.Error != "" {
			fmt.Printf("         error: %s\n", e.Error)
		}
	}

	return false
}

func cmdAuditStats(chain *audit.AuditChain) bool {
	entries := chain.Snapshot()
	if len(entries) == 0 {
		fmt.Println("No audit entries recorded.")
		return false
	}

	// Count by source.
	sourceCounts := make(map[string]int)
	actionCounts := make(map[string]int)
	for _, e := range entries {
		sourceCounts[e.Source]++
		actionCounts[e.Action]++
	}

	fmt.Printf("Audit statistics (%d total entries):\n", len(entries))

	fmt.Println("  By source:")
	sortedSources := make([]string, 0, len(sourceCounts))
	for s := range sourceCounts {
		sortedSources = append(sortedSources, s)
	}
	sort.Strings(sortedSources)
	for _, s := range sortedSources {
		fmt.Printf("    %s: %d\n", s, sourceCounts[s])
	}

	fmt.Println("  By action:")
	sortedActions := make([]string, 0, len(actionCounts))
	for a := range actionCounts {
		sortedActions = append(sortedActions, a)
	}
	sort.Strings(sortedActions)
	for _, a := range sortedActions {
		fmt.Printf("    %s: %d\n", a, actionCounts[a])
	}

	return false
}

// ---------------------------------------------------------------------------
// /cost command
// ---------------------------------------------------------------------------

func cmdCost(_ context.Context, _ string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig, rt *AgentRuntime) bool {
	stats, err := session.LoadUsage(rt.SessionID, rt.Config.DataDir)
	if err != nil {
		fmt.Printf("Error loading usage: %v\n", err)
		return false
	}

	if stats.RecordCount == 0 {
		fmt.Println("No usage records for this session.")
		return false
	}

	fmt.Printf("Session cost summary:\n")
	fmt.Printf("  Total prompt tokens:      %d\n", stats.TotalPromptTokens)
	fmt.Printf("  Total completion tokens:  %d\n", stats.TotalCompletionTokens)
	fmt.Printf("  Total cached tokens:      %d\n", stats.TotalCachedTokens)
	fmt.Printf("  Total reasoning tokens:   %d\n", stats.TotalReasoningTokens)
	fmt.Printf("  Total tokens:             %d\n", stats.TotalTokens)
	fmt.Printf("  Total cost:               %.6f %s\n", stats.TotalCost, stats.Currency)
	fmt.Printf("  Record count:             %d\n", stats.RecordCount)

	return false
}

// ---------------------------------------------------------------------------
// /cache-stats command
// ---------------------------------------------------------------------------

func cmdCacheStats(_ context.Context, _ string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig, rt *AgentRuntime) bool {
	stats, err := session.LoadUsage(rt.SessionID, rt.Config.DataDir)
	if err != nil {
		fmt.Printf("Error loading usage: %v\n", err)
		return false
	}

	if stats.RecordCount == 0 {
		fmt.Println("No usage records for this session.")
		return false
	}

	// Build a report for the current session using all records.
	gen := session.NewReportGenerator(rt.Config.DataDir)
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	report, err := gen.Generate(context.Background(), from, to, "")
	if err != nil {
		fmt.Printf("Error generating cache report: %v\n", err)
		return false
	}

	overall := report.Overall
	hitRate := overall.CacheHitRate * 100

	fmt.Printf("Cache statistics:\n")
	fmt.Printf("  Total prompt tokens:  %d\n", overall.TotalPromptTokens)
	fmt.Printf("  Total cached tokens:  %d\n", overall.TotalCachedTokens)
	fmt.Printf("  Cache hit rate:       %.2f%%\n", hitRate)
	fmt.Printf("  Total cost (w/o cache): %.6f %s\n", overall.TotalCost, overall.Currency)
	fmt.Printf("  Actual cost:            %.6f %s\n", overall.ActualCost, overall.Currency)
	fmt.Printf("  Savings:                %.6f %s\n", overall.Savings, overall.Currency)

	if len(report.ByModel) > 0 {
		fmt.Println("  By model:")
		for _, m := range report.ByModel {
			mHitRate := m.CacheHitRate * 100
			fmt.Printf("    %s:\n", m.Model)
			fmt.Printf("      prompt tokens:   %d\n", m.TotalPromptTokens)
			fmt.Printf("      cached tokens:   %d\n", m.TotalCachedTokens)
			fmt.Printf("      cache hit rate:  %.2f%%\n", mHitRate)
			fmt.Printf("      savings:         %.6f %s\n", m.Savings, m.Currency)
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// /cache-report command
// ---------------------------------------------------------------------------

func cmdCacheReport(_ context.Context, args string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig, rt *AgentRuntime) bool {
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	var model string

	parts := strings.Fields(args)
	for _, arg := range parts {
		if strings.HasPrefix(arg, "--from=") {
			val := strings.TrimPrefix(arg, "--from=")
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				fmt.Printf("Invalid --from value %q: expected RFC3339 format (e.g. 2026-01-01T00:00:00Z)\n", val)
				return false
			}
			from = t
		} else if strings.HasPrefix(arg, "--to=") {
			val := strings.TrimPrefix(arg, "--to=")
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				fmt.Printf("Invalid --to value %q: expected RFC3339 format (e.g. 2026-01-01T00:00:00Z)\n", val)
				return false
			}
			to = t
		} else if strings.HasPrefix(arg, "--model=") {
			model = strings.TrimPrefix(arg, "--model=")
		} else {
			fmt.Printf("Unknown flag: %s\n", arg)
			fmt.Println("Usage: /cache-report [--from=RFC3339] [--to=RFC3339] [--model=MODEL]")
			return false
		}
	}

	gen := session.NewReportGenerator(rt.Config.DataDir)
	report, err := gen.Generate(context.Background(), from, to, model)
	if err != nil {
		fmt.Printf("Error generating cache report: %v\n", err)
		return false
	}

	overall := report.Overall
	hitRate := overall.CacheHitRate * 100

	fmt.Printf("Cross-session cache efficiency report:\n")
	fmt.Printf("  Period:                %s – %s\n", from.Format(time.RFC3339), to.Format(time.RFC3339))
	fmt.Printf("  Sessions:              %d\n", overall.SessionCount)
	fmt.Printf("  Total prompt tokens:   %d\n", overall.TotalPromptTokens)
	fmt.Printf("  Total cached tokens:   %d\n", overall.TotalCachedTokens)
	fmt.Printf("  Cache hit rate:        %.2f%%\n", hitRate)
	fmt.Printf("  Total cost (w/o cache): %.6f %s\n", overall.TotalCost, overall.Currency)
	fmt.Printf("  Actual cost:             %.6f %s\n", overall.ActualCost, overall.Currency)
	fmt.Printf("  Savings:                 %.6f %s\n", overall.Savings, overall.Currency)

	if len(report.ByModel) > 0 {
		fmt.Println("  By model:")
		for _, m := range report.ByModel {
			mHitRate := m.CacheHitRate * 100
			fmt.Printf("    %s:\n", m.Model)
			fmt.Printf("      prompt tokens:   %d\n", m.TotalPromptTokens)
			fmt.Printf("      cached tokens:   %d\n", m.TotalCachedTokens)
			fmt.Printf("      cache hit rate:  %.2f%%\n", mHitRate)
			fmt.Printf("      savings:         %.6f %s\n", m.Savings, m.Currency)
		}
	}

	return false
}
