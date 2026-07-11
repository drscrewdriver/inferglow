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
	"io"
	"log"
	"strings"

	"github.com/inferglow/orchestrator/agent"
)

// RunOneShot executes a single prompt and prints only the final response to
// stdout. Designed for scripts and pipes: no banner, no spinner, no tool
// previews, no session_id line. Tools, memory, and rules are loaded as
// normal; approvals are auto-bypassed.
//
// Mirrors the behavior of `hermes -z PROMPT` / `opencode --print`.
// The process exits after the single response is written.
func RunOneShot(ctx context.Context, cfg CLIConfig, prompt string) error {
	// Non-interactive: auto-approve all tool executions.
	cfg.UnsafeMode = true

	// Suppress agent debug logs (log.Printf) which would corrupt stdout.
	log.SetOutput(io.Discard)

	rt, err := BuildRuntime(cfg, "")
	if err != nil {
		return err
	}
	defer rt.Close(ctx)

	// Build system prompt with all memory layers (same as REPL/TUI).
	rt.Bridge.IngestUser(prompt)
	sysPrompt := rt.Bridge.BuildSystemPrompt(baseSystemPrompt, prompt)

	// Run agent without streaming callbacks — collect the final response only.
	// No EventSink is wired so tool calls, reasoning, and intermediate tokens
	// are completely silent.
	resp, err := rt.Agent.Run(ctx, prompt,
		agent.WithSystemPrompt(sysPrompt),
	)
	if err != nil {
		return fmt.Errorf("agent run: %w", err)
	}

	// Print ONLY the final response to stdout.
	if resp != "" {
		fmt.Print(resp)
		if !strings.HasSuffix(resp, "\n") {
			fmt.Println()
		}
	}

	return nil
}
