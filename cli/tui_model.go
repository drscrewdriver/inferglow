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
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/inferglow/orchestrator/agent"
)

// tuiState represents the TUI interaction state.
type tuiState int

const (
	tuiIdle    tuiState = iota
	tuiRunning          // waiting for agent response
)

// agentEventMsg wraps an agent.AgentEvent for Bubble Tea's message system.
type agentEventMsg agent.AgentEvent

// tuiShutdownMsg signals the TUI to shut down gracefully.
type tuiShutdownMsg struct{}

// elapsedTickMsg fires once per second during a running turn.
type elapsedTickMsg struct{}

// chatTUI is the Bubble Tea model for the full-screen TUI mode.
type chatTUI struct {
	// Core references.
	agent      *agent.Agent
	bridge     *MemoryBridge
	cfg        CLIConfig
	sessionID  string
	modelLabel string

	// Terminal dimensions.
	width  int
	height int

	// State machine.
	state tuiState

	// UI components.
	input    textarea.Model
	spinner  spinner.Model
	viewport viewport.Model

	// Transcript — ALL content lives here. The viewport renders this.
	transcript      []transcriptBlock
	transcriptDirty bool

	// Streaming buffers.
	pending       *strings.Builder
	reasoning     *strings.Builder
	showReasoning bool

	// answerIdx tracks which transcript block is the live streaming answer.
	// -1 means no active streaming block.
	answerIdx     int
	answerFlushed int // bytes of pending already rendered into transcript

	// Run timing.
	runStart   time.Time
	elapsed    int
	turnTokens int

	// Event channel.
	eventCh   <-chan agent.AgentEvent
	closeSink func()
	// eventChClosed is set when the event channel is closed, so we stop
	// re-registering waitForAgentEvent (closed channel returns zero-value
	// events immediately, causing an infinite loop).
	eventChClosed bool

	// Input history.
	submittedInputs []string
	submittedCursor int

	// Paste folding.
	pastedBlocks []pastedBlock
	nextPasteID  int
	
	// Pending approval.
	pendingApproval *approvalCard
	
	// Turn receipt tracking.
	receipt       turnReceipt
	sessionTokensIn  int
	sessionTokensOut int
	
	// Scrollback mode.
	scrollbackMode   bool
	scrollbackOffset int
	
	// Selection mode.
	selectionMode bool
	selectionStart int
	selectionEnd   int

	// OT-14: slash command registry.
	cmdRegistry *SlashRegistry

	// Quit control.
	lastCtrlCAt time.Time
	quit        bool
}

// waitForAgentEvent returns a tea.Cmd that blocks until an agent event arrives.
// Returns nil if the channel is already closed (prevents zero-value event loop).
func (m *chatTUI) waitForAgentEvent() tea.Cmd {
	if m.eventChClosed {
		return nil
	}
	return func() tea.Msg {
		e, ok := <-m.eventCh
		if !ok {
			// Channel closed: mark it so we stop re-registering.
			return channelClosedMsg{}
		}
		return agentEventMsg(e)
	}
}

// channelClosedMsg signals that the event channel has been closed.
type channelClosedMsg struct{}

// elapsedTick returns a tea.Cmd that fires after one second.
func elapsedTick() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg { return elapsedTickMsg{} })
}

// RunTUI starts the full-screen TUI mode.
func RunTUI(ctx context.Context, cfg CLIConfig, resumeID string) error {
	// Suppress agent debug logs (log.Printf) which would corrupt the alt-screen.
	log.SetOutput(io.Discard)

	rt, err := BuildRuntime(cfg, resumeID)
	if err != nil {
		return err
	}
	defer rt.Close(ctx)

	m := newChatTUI(rt.Agent, rt.Bridge, cfg, rt.SessionID)
	p := tea.NewProgram(&m)

	go func() {
		<-ctx.Done()
		p.Send(tuiShutdownMsg{})
	}()

	_, err = p.Run()
	return err
}

// newChatTUI creates an initialized chatTUI model.
func newChatTUI(ag *agent.Agent, bridge *MemoryBridge, cfg CLIConfig, sessionID string) chatTUI {
	ti := textarea.New()
	ti.Placeholder = "Message InferGlow… (Ctrl+D to quit)"
	ti.ShowLineNumbers = false
	ti.DynamicHeight = true
	ti.MinHeight = 1
	ti.MaxHeight = 8
	ti.CharLimit = 16384
	ti.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j", "shift+enter"))
	applyTextareaTheme(&ti)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = workingStyle

	vp := viewport.New()

	modelLabel := cfg.LLM.Model
	if modelLabel == "" {
		modelLabel = "default"
	}

	return chatTUI{
		agent:       ag,
		bridge:      bridge,
		cfg:         cfg,
		sessionID:   sessionID,
		modelLabel:  modelLabel,
		state:       tuiIdle,
		input:       ti,
		spinner:     sp,
		viewport:    vp,
		pending:     &strings.Builder{},
		reasoning:   &strings.Builder{},
		answerIdx:   -1,
		nextPasteID: 1,
		cmdRegistry: buildSlashRegistry(cfg),
	}
}

// applyTextareaTheme configures the textarea with the active theme.
func applyTextareaTheme(ti *textarea.Model) {
	styles := ti.Styles()
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.subtle.hex))
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.subtle2.hex))
	ti.SetStyles(styles)
}

// Init implements tea.Model.
func (m *chatTUI) Init() tea.Cmd {
	focusCmd := m.input.Focus()
	return tea.Batch(
		focusCmd,
		textarea.Blink,
		m.waitForAgentEvent(),
	)
}

// ---------------------------------------------------------------------------
// Update — outer wrapper handles viewport sizing (mirrors Reasonix pattern).
// ---------------------------------------------------------------------------

func (m *chatTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	wasAtBottom := m.viewport.AtBottom()
	prevLines := len(m.transcript)
	prevWidth := m.width
	prevYOff := m.viewport.YOffset()

	// Delegate to inner update for message-specific logic.
	next, cmd := m.innerUpdate(msg)
	cm := next.(*chatTUI)

	// Post-update: sync viewport dimensions to match current layout.
	contentW := max(cm.width-1, 1)
	cm.viewport.SetWidth(contentW)
	cm.viewport.SetHeight(cm.transcriptHeight())

	if cm.width != prevWidth {
		cm.reflowTranscript(cm.width)
	}

	// Re-feed viewport content when transcript changed or width changed.
	if len(cm.transcript) != prevLines || cm.width != prevWidth || cm.transcriptDirty {
		cm.viewport.SetContent(cm.renderTranscript())
		if wasAtBottom {
			cm.viewport.GotoBottom()
		}
	}
	cm.transcriptDirty = false

	// Some terminals (Warp) mishandle scroll/insert-line optimization.
	// Force a full clear+redraw when the viewport offset actually moved.
	if cm.viewport.YOffset() != prevYOff {
		return cm, tea.Batch(tea.ClearScreen, cmd)
	}

	return cm, cmd
}

// ---------------------------------------------------------------------------
// innerUpdate — message dispatch (key handling + event ingestion).
// ---------------------------------------------------------------------------

func (m *chatTUI) innerUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(max(msg.Width-4, 1))
		return m, tea.Batch(cmds...)

	case tuiShutdownMsg:
		m.quit = true
		return m, tea.Quit

	case tea.PasteMsg:
		return m.handlePaste(msg)

	// Mouse wheel: scroll the transcript viewport.
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.viewport.ScrollUp(3)
		case tea.MouseWheelDown:
			m.viewport.ScrollDown(3)
		}
		return m, nil

	case channelClosedMsg:
		// Event channel was closed by the agent goroutine. Stop waiting.
		m.eventChClosed = true
		if m.state == tuiRunning {
			m.state = tuiIdle
			m.answerIdx = -1
		}
		return m, nil

	case agentEventMsg:
		e := agent.AgentEvent(msg)
		m.ingestEvent(e)
		turnDone := e.Kind == agent.EventRunEnd

		// Batch drain: coalesce burst events (up to 256 per update).
		for drained := 0; drained < 256; drained++ {
			select {
			case e2 := <-m.eventCh:
				m.ingestEvent(e2)
				if e2.Kind == agent.EventRunEnd {
					turnDone = true
				}
			default:
				goto doneDrain
			}
		}
	doneDrain:

		cmds = append(cmds, m.waitForAgentEvent())
		if turnDone {
			m.state = tuiIdle
			m.answerIdx = -1
		}
		return m, tea.Batch(cmds...)

	case elapsedTickMsg:
		if m.state == tuiRunning {
			m.elapsed = int(time.Since(m.runStart).Seconds())
			cmds = append(cmds, elapsedTick())
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		sp, cmd := m.spinner.Update(msg)
		m.spinner = sp
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// ---- Key handling: intercept special keys BEFORE textarea ----
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		// Scrollback mode intercepts ALL keys first.
		if m.scrollbackMode {
			if handled, _, _ := m.handleScrollbackKey(keyMsg); handled {
				return m, tea.Batch(cmds...)
			}
		}

		// Approval pending: intercept y/n/escape.
		if m.pendingApproval != nil && !m.pendingApproval.resolved {
			switch keyMsg.String() {
			case "y":
				m.pendingApproval.resolved = true
				m.pendingApproval.approved = true
				m.commitSystemNote(successText("Approved: " + m.pendingApproval.toolName))
				m.pendingApproval = nil
				return m, tea.Batch(cmds...)
			case "n":
				m.pendingApproval.resolved = true
				m.pendingApproval.approved = false
				m.commitSystemNote(errorText("Denied: " + m.pendingApproval.toolName))
				m.pendingApproval = nil
				return m, tea.Batch(cmds...)
			case "esc":
				m.pendingApproval = nil
				m.commitSystemNote(dim("Approval dismissed."))
				return m, tea.Batch(cmds...)
			}
		}

		switch keyMsg.String() {
		case "ctrl+c":
			if m.state == tuiRunning {
				m.state = tuiIdle
				m.answerIdx = -1
				m.commitLine("")
				m.commitLine(warnText("[turn cancelled]"))
				return m, tea.Batch(cmds...)
			}
			if time.Since(m.lastCtrlCAt) < 1500*time.Millisecond {
				return m, tea.Quit
			}
			m.lastCtrlCAt = time.Now()
			m.commitLine(dim("Press Ctrl+C again to quit."))
			return m, tea.Batch(cmds...)

		case "ctrl+d":
			// Always quit: if input has content, clear it first; otherwise quit.
			if strings.TrimSpace(m.input.Value()) != "" {
				m.input.SetValue("")
				m.input.Blur()
				m.input.Focus()
				return m, nil
			}
			return m, tea.Quit

		case "ctrl+o":
			m.showReasoning = !m.showReasoning
			return m, tea.Batch(cmds...)

		case "ctrl+z":
			return m, tea.Suspend

		case "ctrl+s":
			if !m.scrollbackMode {
				m.enterScrollbackMode()
				return m, tea.Batch(cmds...)
			}

		case "v":
			if m.state == tuiIdle && !m.selectionMode {
				m.enterSelectionMode()
				m.commitSystemNote(dim("Visual selection mode (v again to copy, esc to cancel)"))
				return m, tea.Batch(cmds...)
			}
			if m.selectionMode {
				copied := m.copySelection()
				m.exitSelectionMode()
				if copied != "" {
					m.commitSystemNote(successText("Copied to clipboard."))
				}
				return m, tea.Batch(cmds...)
			}

		case "esc":
			if m.selectionMode {
				m.exitSelectionMode()
				m.commitSystemNote(dim("Selection cancelled."))
				return m, tea.Batch(cmds...)
			}

		case "ctrl+home":
			m.viewport.GotoTop()
			return m, tea.Batch(cmds...)

		case "ctrl+end":
			m.viewport.GotoBottom()
			return m, tea.Batch(cmds...)

		case "tab":
			// OT-14: slash command auto-completion.
			if m.cmdRegistry != nil && m.state == tuiIdle {
				val := m.input.Value()
				if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
					prefix := strings.TrimPrefix(val, "/")
					matches := m.cmdRegistry.Complete(prefix)
					if len(matches) == 1 {
						// Single match: auto-complete.
						m.input.SetValue("/" + matches[0] + " ")
						return m, tea.Batch(cmds...)
					} else if len(matches) > 1 {
						// Multiple matches: show candidates.
						m.commitLine("")
						m.commitLine(dim("  Completions: /" + strings.Join(matches, " /")))
						return m, tea.Batch(cmds...)
					}
				}
			}

		case "enter":
			if m.state != tuiIdle {
				return m, tea.Batch(cmds...)
			}
			val := strings.TrimSpace(m.input.Value())
			if val == "" {
				return m, tea.Batch(cmds...)
			}
			m.input.Reset()
			return m.handleSubmit(val, cmds)

		case "up":
			if m.state == tuiIdle && strings.TrimSpace(m.input.Value()) == "" && len(m.submittedInputs) > 0 {
				if m.submittedCursor < len(m.submittedInputs) {
					m.submittedCursor++
				}
				idx := len(m.submittedInputs) - m.submittedCursor
				if idx >= 0 && idx < len(m.submittedInputs) {
					m.input.SetValue(m.submittedInputs[idx])
				}
				return m, tea.Batch(cmds...)
			}

		case "down":
			if m.state == tuiIdle && m.submittedCursor > 0 {
				m.submittedCursor--
				if m.submittedCursor == 0 {
					m.input.SetValue("")
				} else {
					idx := len(m.submittedInputs) - m.submittedCursor
					if idx >= 0 && idx < len(m.submittedInputs) {
						m.input.SetValue(m.submittedInputs[idx])
					}
				}
				return m, tea.Batch(cmds...)
			}

		case "pgup":
			m.viewport.PageUp()
			return m, tea.Batch(cmds...)

		case "pgdown":
			m.viewport.PageDown()
			return m, tea.Batch(cmds...)
		}
	}

	// ---- All other messages (including regular keystrokes) → textarea ----
	ta, taCmd := m.input.Update(msg)
	m.input = ta
	cmds = append(cmds, taCmd)

	// Also update viewport for scroll messages.
	vp, vpCmd := m.viewport.Update(msg)
	m.viewport = vp
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// handleSubmit processes a confirmed user input (after Enter).
func (m *chatTUI) handleSubmit(val string, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(val, "/") {
		cmd, quit := m.tuiDispatchCommand(val)
		if quit {
			return m, tea.Quit
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	sentLine := m.expandPastedBlocks(val)
	m.submitTurn(sentLine)
	cmds = append(cmds, m.waitForAgentEvent())
	return m, tea.Batch(cmds...)
}

// mergeCallbacks merges two AgentCallbacks. When both original and override
// have a non-nil hook, the merged hook calls BOTH in order (original first,
// then override). This ensures side-effect callbacks like IngestAssistant
// are not lost when the TUI adds its own event-sink callbacks.
func mergeCallbacks(original, override *agent.AgentCallbacks) *agent.AgentCallbacks {
	if original == nil {
		return override
	}
	if override == nil {
		return original
	}
	return &agent.AgentCallbacks{
		OnRunStart: func(ctx context.Context, userMessage string) {
			if original.OnRunStart != nil {
				original.OnRunStart(ctx, userMessage)
			}
			if override.OnRunStart != nil {
				override.OnRunStart(ctx, userMessage)
			}
		},
		OnRunEnd: func(ctx context.Context, response string, err error) {
			if original.OnRunEnd != nil {
				original.OnRunEnd(ctx, response, err)
			}
			if override.OnRunEnd != nil {
				override.OnRunEnd(ctx, response, err)
			}
		},
		OnLLMCallStart: func(ctx context.Context, round int) {
			if original.OnLLMCallStart != nil {
				original.OnLLMCallStart(ctx, round)
			}
			if override.OnLLMCallStart != nil {
				override.OnLLMCallStart(ctx, round)
			}
		},
		OnLLMCallEnd: func(ctx context.Context, round int, tokens int) {
			if original.OnLLMCallEnd != nil {
				original.OnLLMCallEnd(ctx, round, tokens)
			}
			if override.OnLLMCallEnd != nil {
				override.OnLLMCallEnd(ctx, round, tokens)
			}
		},
		OnToolCallStart: func(ctx context.Context, toolName string) {
			if original.OnToolCallStart != nil {
				original.OnToolCallStart(ctx, toolName)
			}
			if override.OnToolCallStart != nil {
				override.OnToolCallStart(ctx, toolName)
			}
		},
		OnToolCallEnd: func(ctx context.Context, toolName string, err error) {
			if original.OnToolCallEnd != nil {
				original.OnToolCallEnd(ctx, toolName, err)
			}
			if override.OnToolCallEnd != nil {
				override.OnToolCallEnd(ctx, toolName, err)
			}
		},
		OnToken: func(ctx context.Context, delta string) {
			if original.OnToken != nil {
				original.OnToken(ctx, delta)
			}
			if override.OnToken != nil {
				override.OnToken(ctx, delta)
			}
		},
		OnReasoning: func(ctx context.Context, delta string) {
			if original.OnReasoning != nil {
				original.OnReasoning(ctx, delta)
			}
			if override.OnReasoning != nil {
				override.OnReasoning(ctx, delta)
			}
		},
		OnApprovalRequired: func(ctx context.Context, toolName, recordID string) {
			if original.OnApprovalRequired != nil {
				original.OnApprovalRequired(ctx, toolName, recordID)
			}
			if override.OnApprovalRequired != nil {
				override.OnApprovalRequired(ctx, toolName, recordID)
			}
		},
		OnCompression: func(ctx context.Context, stepsCompressed int) {
			if original.OnCompression != nil {
				original.OnCompression(ctx, stepsCompressed)
			}
			if override.OnCompression != nil {
				override.OnCompression(ctx, stepsCompressed)
			}
		},
	}
}

// submitTurn starts an agent turn with the given user message.
func (m *chatTUI) submitTurn(message string) {
	m.submittedInputs = append(m.submittedInputs, message)
	m.submittedCursor = 0

	m.commitUserBubble(message)
	m.bridge.IngestUser(message)

	sysPrompt := m.bridge.BuildSystemPrompt(baseSystemPrompt, message)

	sink, events, closeSink := agent.NewChannelSink(1024)
	m.eventCh = events
	m.closeSink = closeSink
	m.eventChClosed = false // Reset for new turn

	// Merge sink callbacks with agent's persisted callbacks (e.g. OnRunEnd for IngestAssistant).
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
		_, _ = m.agent.Run(context.Background(), message,
			agent.WithSystemPrompt(sysPrompt),
			agent.WithCallbacks(mergedCB),
		)

		// CM-2: auto-trigger rebackground if Zone 1 is still empty
		// after the agent turn completes.
		if m.bridge.CheckAutoBackground() {
			m.tuiHandleRebackground("")
		}
	}()
}

// ---------------------------------------------------------------------------
// Event ingestion — streaming content goes INTO transcript immediately.
// ---------------------------------------------------------------------------

func (m *chatTUI) ingestEvent(e agent.AgentEvent) {
	switch e.Kind {
	case agent.EventRunStart:
		m.state = tuiRunning
		m.runStart = time.Now()
		m.pending.Reset()
		m.reasoning.Reset()
		m.answerIdx = -1
		m.answerFlushed = 0

	case agent.EventToken:
		m.pending.WriteString(e.Text)
		m.streamAnswer()

	case agent.EventReasoning:
		m.reasoning.WriteString(e.Text)
		if m.showReasoning {
			m.flushStreamingReasoning()
		}

	case agent.EventToolStart:
		m.finalizeStreamingAnswer()
		sandboxMode := ""
		sideEffect := ""
		if e.Metadata != nil {
			sandboxMode = e.Metadata["sandboxMode"]
			sideEffect = e.Metadata["sideEffectLevel"]
		}
		m.commitToolCardEx(e.ToolName, "running", sandboxMode, sideEffect, "")

	case agent.EventToolEnd:
		status := "done"
		if e.Err != nil {
			status = "error"
		}
		if e.Status == "blocked" {
			status = "blocked"
		}
		sandboxMode := ""
		sideEffect := ""
		if e.Metadata != nil {
			sandboxMode = e.Metadata["sandboxMode"]
			sideEffect = e.Metadata["sideEffectLevel"]
		}
		m.commitToolCardEx(e.ToolName, status, sandboxMode, sideEffect, "")
		// T5: show task progress on task tool completion.
		if isTaskTool(e.ToolName) && e.Err == nil {
			if summary := m.bridge.TaskSummary(); summary != "" {
				m.commitSystemNote("task progress: " + summary)
			}
		}

	case agent.EventApproval:
		recordID := ""
		if e.Metadata != nil {
			recordID = e.Metadata["recordID"]
		}
		m.commitApprovalCard(e.ToolName, recordID)
		m.pendingApproval = &approvalCard{
			recordID: recordID,
			toolName: e.ToolName,
		}

	case agent.EventCompression:
		steps := "?"
		if e.Metadata != nil {
			steps = e.Metadata["stepsCompressed"]
		}
		m.commitSystemNote(compressionNote("compressed " + steps + " steps"))

	case agent.EventRunEnd:
		m.state = tuiIdle
		m.elapsed = int(time.Since(m.runStart).Seconds())
		m.finalizeStreamingAnswer()
		// Emit turn receipt.
		m.receipt.duration = m.elapsed
		m.commitReceipt(fmt.Sprintf("Turn · %ds · ↓%s ↑%s · %d tools",
			m.receipt.duration,
			shortTokens(m.sessionTokensIn),
			shortTokens(m.sessionTokensOut),
			m.receipt.toolCalls))
		if e.Err != nil {
			m.commitLine("")
			m.commitLine(errorText(fmt.Sprintf("Error: %v", e.Err)))
		}
		// CM-5: detect plan completion and inherit into Zone 1.
		if e.Metadata != nil {
			if planTitle := e.Metadata["plan_title"]; planTitle != "" {
				planSummary := e.Metadata["plan_summary"]
				m.bridge.AppendPlanToHeadBuffer(planTitle, planSummary)
				m.commitSystemNote("plan inherited to background: " + planTitle)
			}
		}

	case agent.EventError:
		if e.Err != nil {
			m.commitLine("")
			m.commitLine(errorText(fmt.Sprintf("Error: %v", e.Err)))
		}

	case agent.EventLLMEnd:
		// Accumulate completion tokens from each LLM round.
		if e.Tokens > 0 {
			m.turnTokens += e.Tokens
			m.sessionTokensIn += e.Tokens
		}
		m.receipt.llmRounds++

	case agent.EventLLMStart:
		// Optional: could show round indicator.
	}
}

// ---------------------------------------------------------------------------
// Streaming answer — paragraph-level incremental rendering (like Reasonix).
// ---------------------------------------------------------------------------

// streamAnswer flushes completed markdown paragraphs into the transcript.
// Uses flushableMarkdownPrefix to avoid rendering half-written code blocks.
func (m *chatTUI) streamAnswer() {
	prefix := flushableMarkdownPrefix(m.pending.String())
	if len(prefix) <= m.answerFlushed {
		return // no new completed paragraph
	}
	m.answerFlushed = len(prefix)
	if m.answerIdx < 0 {
		// First content: create a new transcript block.
		m.commitLine("")
		m.answerIdx = len(m.transcript)
		m.commitLine(prefix)
	} else {
		// Update the existing streaming block in-place.
		m.transcript[m.answerIdx] = transcriptBlock{Kind: blockAssistant, Raw: prefix, Source: prefix}
	}
	m.transcriptDirty = true
}

// flushableMarkdownPrefix returns the longest prefix of buf made of complete
// markdown blocks — text up to the last blank line outside any open fenced
// code block. A half-written code block stays buffered until it closes.
func flushableMarkdownPrefix(buf string) string {
	lines := strings.Split(buf, "\n")
	inFence := false
	boundary := -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && t == "" && i > 0 {
			boundary = i
		}
	}
	if inFence {
		// Inside an unclosed fence: return nothing beyond what was already flushed.
		return ""
	}
	if boundary < 0 {
		return ""
	}
	return strings.Join(lines[:boundary+1], "\n")
}

// flushStreamingReasoning writes reasoning content into transcript.
func (m *chatTUI) flushStreamingReasoning() {
	text := m.reasoning.String()
	if text == "" {
		return
	}
	lines := strings.Split(text, "\n")
	lastLine := lines[len(lines)-1]
	if lastLine != "" {
		reasoningLine := reasoningDim("▎ " + lastLine)
		if len(m.transcript) > 0 && strings.Contains(m.transcript[len(m.transcript)-1].Raw, "▎") {
			m.transcript[len(m.transcript)-1] = transcriptBlock{Kind: blockText, Raw: reasoningLine, Source: "▎ " + lastLine}
		} else {
			m.commitLine(reasoningLine)
		}
		m.transcriptDirty = true
	}
}

// finalizeStreamingAnswer commits the final pending content and resets.
func (m *chatTUI) finalizeStreamingAnswer() {
	if m.pending.Len() > 0 {
		text := strings.TrimRight(m.pending.String(), "\n ")
		if text != "" {
			if m.answerIdx >= 0 && m.answerIdx < len(m.transcript) {
				m.transcript[m.answerIdx] = transcriptBlock{Kind: blockAssistant, Raw: text, Source: text}
			} else {
				m.commitLine("")
				m.commitLine(text)
			}
		}
		m.pending.Reset()
		m.answerIdx = -1
		m.answerFlushed = 0
		m.transcriptDirty = true
	}
	if m.reasoning.Len() > 0 && m.showReasoning {
		text := strings.TrimRight(m.reasoning.String(), "\n ")
		if text != "" {
			m.commitLine(reasoningDim("▎ thought"))
			for _, line := range strings.Split(text, "\n") {
				m.commitLine(reasoningDim("▎ " + line))
			}
		}
		m.reasoning.Reset()
	}
}

// ---------------------------------------------------------------------------
// View — viewport on top, bottom region beneath. Cursor positioned correctly.
// ---------------------------------------------------------------------------

func (m *chatTUI) View() tea.View {
	if m.quit {
		return tea.NewView("")
	}

	boxW := m.width
	if boxW < 10 {
		boxW = 10
	}

	// Build bottom region.
	var parts []string
	rowsAboveBox := 0

	// Working spinner line (above input, below transcript).
	if m.state == tuiRunning {
		parts = append(parts, workingStyle.Width(boxW).MaxWidth(boxW).Render(m.renderWorkingLine()))
		rowsAboveBox++
	}

	// Status bar.
	parts = append(parts, m.renderStatusBar())

	// Input box.
	inputStyle := inputBoxStyle.Width(boxW)
	inputBox := inputStyle.Render(m.input.View())
	parts = append(parts, inputBox)

	// Full frame: transcript viewport on top, bottom region beneath.
	mainArea := m.viewport.View()
	v := tea.NewView(mainArea + "\n" + strings.Join(parts, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	// Position the terminal cursor at the textarea's insertion point.
	if cur := m.input.Cursor(); cur != nil {
		cur.X += 1 // padding left
		cur.Y += m.viewport.Height() + rowsAboveBox + 1
		v.Cursor = cur
	}

	return v
}

// renderWorkingLine shows spinner + elapsed + token count during a running turn.
func (m *chatTUI) renderWorkingLine() string {
	spinnerView := m.spinner.View()
	working := fmt.Sprintf("  %s thinking… (%ds)", spinnerView, m.elapsed)
	if m.turnTokens > 0 {
		working += " · ↓" + shortTokens(m.turnTokens)
	}
	return working
}

// ---------------------------------------------------------------------------
// Layout helpers — bottomRows pattern (mirrors Reasonix).
// ---------------------------------------------------------------------------

func (m *chatTUI) bottomRows() int {
	rows := 0
	if m.state == tuiRunning {
		rows++ // working line
	}
	rows++ // status bar
	rows += m.input.Height() + 2 // input box + border
	return rows
}

func (m *chatTUI) transcriptHeight() int {
	h := m.height - m.bottomRows()
	if h < 3 {
		h = 3
	}
	return h
}

func (m *chatTUI) reflowTranscript(width int) {
	m.transcriptDirty = true
}

// ---------------------------------------------------------------------------
// Paste handling.
// ---------------------------------------------------------------------------

func (m *chatTUI) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	content := msg.Content
	if content == "" {
		return m, nil
	}
	if shouldFoldPastedText(content) {
		m.insertFoldedPaste(content)
		return m, nil
	}
	ta, _ := m.input.Update(msg)
	m.input = ta
	return m, nil
}

func (m *chatTUI) insertFoldedPaste(s string) {
	lines := pastedLineCount(s)
	label := foldedPasteLabel(m.nextPasteID, lines)
	m.nextPasteID++
	m.pastedBlocks = append(m.pastedBlocks, pastedBlock{label: label, text: s})
	m.input.InsertString(label + " ")
}

func (m *chatTUI) expandPastedBlocks(displayed string) string {
	sent := displayed
	for _, block := range m.pastedBlocks {
		if !strings.Contains(sent, block.label) {
			continue
		}
		repl := renderFoldedPasteBlock(block)
		sent = strings.ReplaceAll(sent, block.label, repl)
	}
	m.pastedBlocks = nil
	return sent
}

type pastedBlock struct {
	label string
	text  string
}

const (
	foldedPasteMinChars = 200
	foldedPasteMinLines = 5
)

func pastedLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"), "\n") + 1
}

func foldedPasteLabel(id, lines int) string {
	return fmt.Sprintf("[Pasted text #%d · %d lines]", id, lines)
}

func shouldFoldPastedText(s string) bool {
	return len([]rune(s)) >= foldedPasteMinChars || pastedLineCount(s) >= foldedPasteMinLines
}

func renderFoldedPasteBlock(block pastedBlock) string {
	return fmt.Sprintf("%s\n\n--- Begin %s ---\n%s\n--- End %s ---", block.label, block.label, block.text, block.label)
}

// isTaskTool reports whether the tool name is a task tracker action.
func isTaskTool(name string) bool {
	return name == "task_add" || name == "task_update" || name == "task_delete"
}
