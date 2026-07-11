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
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/inferglow/cli"
)

// runMemory handles the "memory" subcommand.
func runMemory(ctx context.Context, cfg cli.CLIConfig, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: inferglow memory <search|list|stats|validate>")
	}

	switch args[0] {
	case "search":
		return runMemorySearch(ctx, cfg, args[1:])
	case "list":
		return runMemoryList(ctx, cfg, args[1:])
	case "stats":
		return runMemoryStats(ctx, cfg, args[1:])
	case "validate":
		return runMemoryValidate(ctx, cfg, args[1:])
	default:
		return fmt.Errorf("unknown memory subcommand: %s", args[0])
	}
}

// runMemorySearch searches long-term memory.
func runMemorySearch(ctx context.Context, cfg cli.CLIConfig, args []string) error {
	fs := flag.NewFlagSet("memory search", flag.ExitOnError)
	semantic := fs.Bool("semantic", false, "Use semantic (vector) search")
	scope := fs.String("scope", "", "Search scope: session, task_group, global")
	limit := fs.Int("limit", 10, "Maximum results")
	_ = fs.Parse(args)

	query := strings.Join(fs.Args(), " ")
	if query == "" {
		return fmt.Errorf("usage: inferglow memory search [--semantic] [--scope=global] <query>")
	}

	bridge, err := cli.NewMemoryBridge(cfg, "memory-cli")
	if err != nil {
		return fmt.Errorf("memory bridge: %w", err)
	}
	defer bridge.OnSessionEnd(ctx)

	_ = semantic // semantic search available when VectorStoreBackend is injected
	_ = scope    // scope available when external store is injected
	_ = limit

	results, err := bridge.SearchMemory(ctx, query)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	for _, r := range results {
		fmt.Printf("[%s] %s\n", r.MemID, strings.Join(r.Facts, "; "))
	}
	fmt.Printf("\n%d results\n", len(results))
	return nil
}

// runMemoryList lists memories, optionally filtered by session.
func runMemoryList(ctx context.Context, cfg cli.CLIConfig, args []string) error {
	fs := flag.NewFlagSet("memory list", flag.ExitOnError)
	sessionID := fs.String("session", "", "Filter by session ID")
	_ = fs.Parse(args)
	_ = sessionID

	bridge, err := cli.NewMemoryBridge(cfg, "memory-cli")
	if err != nil {
		return fmt.Errorf("memory bridge: %w", err)
	}
	defer bridge.OnSessionEnd(ctx)

	stats := bridge.Stats()
	out, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(out))
	return nil
}

// runMemoryStats prints memory statistics.
func runMemoryStats(ctx context.Context, cfg cli.CLIConfig, args []string) error {
	_ = args

	bridge, err := cli.NewMemoryBridge(cfg, "memory-cli")
	if err != nil {
		return fmt.Errorf("memory bridge: %w", err)
	}
	defer bridge.OnSessionEnd(ctx)

	stats := bridge.Stats()
	out, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(out))
	return nil
}

// runMemoryValidate boosts confidence for a memory by ID.
func runMemoryValidate(ctx context.Context, cfg cli.CLIConfig, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: inferglow memory validate <mem_id>")
	}

	bridge, err := cli.NewMemoryBridge(cfg, "memory-cli")
	if err != nil {
		return fmt.Errorf("memory bridge: %w", err)
	}
	defer bridge.OnSessionEnd(ctx)

	memID := args[0]
	bridge.ValidateCited([]string{memID})
	fmt.Fprintf(os.Stderr, "Validated memory: %s\n", memID)
	return nil
}
