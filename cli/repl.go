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
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/inferglow/orchestrator/agent"
)

const baseSystemPrompt = `You are InferGlow CLI, a helpful AI assistant with access to tools.
You can read/write files, execute commands, search code, and fetch URLs.
You have persistent memory across conversations — relevant memories are
automatically injected into the conversation context.`

// RunREPL starts the interactive read-eval-print loop.
func RunREPL(ctx context.Context, cfg CLIConfig, resumeID string) error {
	// Determine session ID.
	sessionID := resumeID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Build memory bridge.
	bridge, err := NewMemoryBridge(cfg, sessionID)
	if err != nil {
		return fmt.Errorf("init memory bridge: %w", err)
	}
	defer bridge.OnSessionEnd(ctx)

	// Load constitutional entries if configured.
	if cfg.Features.Constitutional && cfg.Constitutional != "" {
		entries, err := loadConstitutional(cfg.Constitutional)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load constitutional: %v\n", err)
		} else {
			bridge.AppendConstitutional(entries)
		}
	}

	// Build agent.
	ag, err := buildAgent(cfg, bridge, sessionID)
	if err != nil {
		return fmt.Errorf("init agent: %w", err)
	}

	fmt.Printf("InferGlow CLI Agent (session: %s)\n", sessionID[:8])
	fmt.Println("Type /help for commands, /quit to exit.")

	// Proactive recall on session start.
	if cfg.Features.ProactiveRecall && resumeID != "" {
		memText, _ := bridge.Recall(ctx, "recent conversation topics")
		if memText != "" {
			fmt.Println("[Recalled from previous session]")
		}
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nSession ended.")
			return nil
		default:
		}

		fmt.Print(">>> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Slash commands.
		if strings.HasPrefix(line, "/") {
			if shouldQuit := dispatchCommand(ctx, line, ag, bridge, cfg); shouldQuit {
				return nil
			}
			continue
		}

		// Regular chat.
		if err := chatOnce(ctx, ag, bridge, cfg, line); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}

	return nil
}

// chatOnce handles a single user message through the agent.
func chatOnce(ctx context.Context, ag *agent.Agent, bridge *MemoryBridge, cfg CLIConfig, message string) error {
	// Ingest user message.
	bridge.IngestUser(message)

	// Recall relevant memories.
	var sysPrompt string
	if cfg.Features.MemoryInjection {
		memText, _ := bridge.Recall(ctx, message)
		sysPrompt = baseSystemPrompt
		if memText != "" {
			sysPrompt += "\n\n" + memText
		}
	} else {
		sysPrompt = baseSystemPrompt
	}

	// Create an event sink for real-time token display.
	sink, events, closeSink := agent.NewChannelSink(256)
	defer closeSink()

	// Start a goroutine to consume events and print tokens in real-time.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range events {
			switch ev.Kind {
			case agent.EventToken:
				fmt.Print(ev.Text)
			case agent.EventToolStart:
				fmt.Printf("\n[tool: %s]\n", ev.ToolName)
			case agent.EventToolEnd:
				if ev.Err != nil {
					fmt.Printf("[tool error: %v]\n", ev.Err)
				}
			}
		}
	}()

	// Run agent with event callbacks.
	cb := agent.CallbacksFromSink(sink)
	resp, err := ag.Run(ctx, message,
		agent.WithSystemPrompt(sysPrompt),
		agent.WithCallbacks(cb),
	)
	closeSink() // Signal the consumer goroutine to exit.
	<-done

	if err != nil {
		return err
	}

	// Print final response (tokens were already streamed, so just add newline).
	fmt.Println()
	_ = resp // Response already streamed via EventToken.

	return nil
}
