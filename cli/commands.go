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
	"strings"

	"github.com/inferglow/orchestrator/agent"
)

// CommandFunc is a slash command handler. Returns true if the REPL should quit.
type CommandFunc func(ctx context.Context, args string, ag *agent.Agent, bridge *MemoryBridge, cfg CLIConfig) bool

// commandTable maps command names to handlers.
var commandTable = map[string]CommandFunc{
	"help":    cmdHelp,
	"memory":  cmdMemory,
	"compact": cmdCompact,
	"quit":    cmdQuit,
	"exit":    cmdQuit,
}

// dispatchCommand parses and dispatches a slash command.
// Returns true if the REPL should quit.
func dispatchCommand(ctx context.Context, line string, ag *agent.Agent, bridge *MemoryBridge, cfg CLIConfig) bool {
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

	return handler(ctx, args, ag, bridge, cfg)
}

func cmdHelp(_ context.Context, _ string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig) bool {
	fmt.Println(`Available commands:
  /help              Show this help message
  /memory search <q> Search memory for a query
  /memory stats      Show memory statistics
  /compact           Manually trigger context compression
  /quit              End session and exit`)
	return false
}

func cmdMemory(ctx context.Context, args string, _ *agent.Agent, bridge *MemoryBridge, _ CLIConfig) bool {
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

func cmdCompact(ctx context.Context, _ string, _ *agent.Agent, bridge *MemoryBridge, _ CLIConfig) bool {
	if err := bridge.Compact(ctx); err != nil {
		fmt.Printf("Compression error: %v\n", err)
	} else {
		fmt.Println("Compression triggered.")
	}
	return false
}

func cmdQuit(_ context.Context, _ string, _ *agent.Agent, _ *MemoryBridge, _ CLIConfig) bool {
	fmt.Println("Goodbye!")
	return true
}
