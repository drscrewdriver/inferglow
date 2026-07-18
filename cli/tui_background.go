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
	"time"

	contextmgr "github.com/inferglow/context"
	"github.com/inferglow/orchestrator/agent"
)

// tuiHandleShowBackground displays the current project background context
// including the system prompt, constitutional zone, and memory status.
func (m *chatTUI) tuiHandleShowBackground(args string) {
	m.commitLine("")
	m.commitLine(accent("Project Background"))
	m.commitLine(dim("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

	// Section 1: Base System Prompt
	m.commitLine("")
	m.commitLine(accent("System Prompt (base):"))
	for _, line := range strings.Split(baseSystemPrompt, "\n") {
		m.commitLine(dim("  " + line))
	}

	// Section 2: Constitutional Zone
	m.commitLine("")
	m.commitLine(accent("Constitutional Zone:"))
	cm := m.bridge.ContextManager()
	if cm != nil {
		// Try to get constitutional entries from the hybrid manager.
		// The current API doesn't expose a direct getter, so we show
		// the config path and status instead.
		constitutionalFile := m.cfg.Constitutional
		if constitutionalFile != "" {
			m.commitLine(dim("  Source:  ") + footerInfo(constitutionalFile))
		}
		if m.cfg.Features.Constitutional {
			m.commitLine(dim("  Status:  ") + successText("enabled"))
		} else {
			m.commitLine(dim("  Status:  ") + infoText("disabled"))
		}
	} else {
		m.commitLine(dim("  (no context manager)"))
	}

	// Section 3: Project Instructions (from skill store)
	m.commitLine("")
	m.commitLine(accent("Project Instructions:"))
	skillStore := m.bridge.SkillStore()
	if skillStore != nil {
		instructions := skillStore.ProjectInstructions(m.bridge.ProjectRoot())
		if instructions != "" {
			m.commitLine(dim("  Project root: ") + footerInfo(m.bridge.ProjectRoot()))
			m.commitLine("")
			for _, line := range strings.Split(instructions, "\n") {
				m.commitLine(dim("  " + line))
			}
		} else {
			m.commitLine(dim("  (none)"))
		}
	} else {
		m.commitLine(dim("  (no skill store)"))
	}

	// Section 4: Memory Index
	m.commitLine("")
	m.commitLine(accent("Memory:"))
	idx := m.bridge.MemoryIndex()
	if idx != "" {
		m.commitLine(dim("  Standing facts:"))
		for _, line := range strings.Split(idx, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				m.commitLine(dim("    " + trimmed))
			}
		}
	} else {
		m.commitLine(dim("  (no memory stored)"))
	}

	m.commitLine("")
	m.commitLine(dim("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
	m.commitLine(dim("Use /rebackground to have the AI analyze and rewrite this background."))
}

// tuiHandleRebackground triggers the AI agent to analyze the project
// context and generate/update a concise background summary. The result
// is stored in Zone 1 (head buffer) as persistent project background.
func (m *chatTUI) tuiHandleRebackground(args string) {
	m.commitLine("")
	m.commitLine(accent("Rebackgrounding..."))
	m.commitLine(dim("  The AI will analyze your project and generate a background summary."))
	m.commitLine(dim("  This will be stored in Zone 1 (head buffer) as persistent context."))
	m.commitLine("")

	// Build a prompt that asks the agent to analyze the project.
	rebackgroundPrompt := `You are performing a project background analysis. Please:
1. Review the current workspace context and project files
2. Identify the project's purpose, tech stack, architecture, and conventions
3. Write a concise background summary (2-4 paragraphs) that captures:
   - What this project does
   - Key technologies and patterns used
   - Important conventions and constraints
   - Current development focus or goals
4. Format the output as a clean, readable summary

Current workspace: ` + m.cfg.WorkspaceDir

	// Build the system prompt with full memory context.
	sysPrompt := m.bridge.BuildSystemPrompt(baseSystemPrompt, rebackgroundPrompt)

	// Create a sink and run the agent in a goroutine.
	sink, events, closeSink := agent.NewChannelSink(1024)
	m.eventCh = events
	m.closeSink = closeSink
	m.eventChClosed = false

	sinkCB := agent.CallbacksFromSink(sink)
	mergedCB := mergeCallbacks(m.agent.Callbacks(), sinkCB)

	m.state = tuiRunning
	m.runStart = time.Now()
	m.elapsed = 0
	m.turnTokens = 0
	m.pending.Reset()
	m.reasoning.Reset()
	m.answerIdx = -1
	m.answerFlushed = 0
	m.transcriptDirty = true

	go func() {
		defer closeSink()
		result, err := m.agent.Run(context.Background(), rebackgroundPrompt,
			agent.WithSystemPrompt(sysPrompt),
			agent.WithCallbacks(mergedCB),
		)
		if err != nil {
			m.commitLine(errorText(fmt.Sprintf("Rebackground error: %v", err)))
			return
		}

		// Extract the final response text.
		background := strings.TrimSpace(result)
		if background == "" {
			m.commitLine(errorText("AI returned empty background. Try again."))
			return
		}

		// Write the background to Zone 1 (head buffer) instead of Zone 0.5.
		// Zone 1 = Background (task background) + Skill lightweight index.
		version := fmt.Sprintf("rebg-%d", time.Now().Unix())
		var zone1Blocks []contextmgr.RenderedBlock
		zone1Blocks = append(zone1Blocks, contextmgr.RenderedBlock{
			StepID:  -4, // background pseudo-step
			Level:   0,
			Content: "## Project Background\n\n" + background,
		})
		// Append skill lightweight index if available.
		if m.bridge.SkillStore() != nil {
			if idx := m.bridge.SkillStore().IndexBlock(); idx != "" {
				zone1Blocks = append(zone1Blocks, contextmgr.RenderedBlock{
					StepID:  -5, // skill index pseudo-step
					Level:   0,
					Content: idx,
				})
			}
		}
		m.bridge.RewriteHeadBuffer(zone1Blocks, version)

		m.commitLine("")
		m.commitLine(successText("Background updated and saved to Zone 1 (head buffer)."))
		m.commitLine(dim("Use /showbackground to view the current background."))
	}()
}