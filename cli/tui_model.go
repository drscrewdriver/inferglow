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
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/inferglow/cli/composer"
	"github.com/inferglow/model"
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

	// RF-1: effective model route + per-run requester (/model switching).
	route     ModelRoute
	requester model.ModelRequester // nil → agent's construction-time requester
	// RF-2: reasoning effort level ("", "auto", or a scale level name).
	effort string
	// RF-2: per-model effort scales (config overrides + built-in defaults).
	effortScales []EffortScale
	// RF-1: interactive model picker state.
	picker modelPicker

	// RF-9: startup welcome page state.
	welcome tuiWelcome

	// RF-10: API health checker.
	health healthChecker

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
	// RF-6: reasoning/tool timing windows.
	thinkingStart time.Time
	toolStart     map[string]time.Time
	// RF-7: TPS tracker.
	tps tpsTracker
	
	// Scrollback mode.
	scrollbackMode   bool
	scrollbackOffset int
	
	// Selection mode.
	selectionMode bool
	selectionStart int
	selectionEnd   int

	// OT-14: slash command registry.
	cmdRegistry *SlashRegistry

	// SC-2: IME-style "/" prefix autocomplete popup.
	completion completionPopup

	// SC-3: right-side task list panel.
	taskPanel       TaskPanel
	taskPanelUserSet bool // user toggled the panel via /tasks; disables auto-show

	// SC-4: history message action menu (selection mode + actions).
	messageActions MessageActionsMenu

	// SC-5: workspace directory switching.
	workspace WorkspaceSwitch

	// Quit control.
	lastCtrlCAt time.Time
	quit        bool
	// restartResumeID, when set by /resume <id>, instructs RunTUI to relaunch
	// the TUI against that persisted session (real conversation switch).
	restartResumeID string

	// Rich input composer (deterministic paste/enter decision state machine).
	composer *composer.Composer
	// pendingImage holds a clipboard image staged for attachment (P1).
	pendingImage *pendingImageAttachment
	// renderRaw toggles P2 rich/raw rendering (false = rich/ANSI, true = raw).
	renderRaw bool
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

// composerTickMsg drives the composer flush tick.
type composerTickMsg struct{}

// composerTickCmd returns a tea.Cmd that re-arms the composer flush tick;
// returns nil while the composer is idle.
func (m *chatTUI) composerTickCmd() tea.Cmd {
	if m.composer == nil || m.composer.Mode() == composer.ModeIdle {
		return nil
	}
	return tea.Tick(m.composer.TickInterval(), func(_ time.Time) tea.Msg { return composerTickMsg{} })
}

// applyComposerActions applies composer actions to the textarea input.
// Submit/BufferDiscard are handled by the caller.
func (m *chatTUI) applyComposerActions(acts []composer.Action) {
	for _, a := range acts {
		switch a.Kind {
		case composer.ActionTyped, composer.ActionPaste:
			m.input.InsertString(a.Text)
		case composer.ActionInsertNewline:
			m.input.InsertString("\n")
		}
	}
}

// RunTUI starts the full-screen TUI mode.
func RunTUI(ctx context.Context, cfg CLIConfig, resumeID string) error {
	// Suppress agent debug logs (log.Printf) which would corrupt the alt-screen.
	log.SetOutput(io.Discard)

	// Resume loop: /resume <id> arms a model.restartResumeID and requests a
	// quit; on exit we tear down the current runtime and relaunch against the
	// new session, so the TUI truly switches conversation context.
	for {
		rt, err := BuildRuntime(cfg, resumeID)
		if err != nil {
			return err
		}
		closed := false
		closeRT := func() {
			if !closed {
				rt.Close(ctx)
				closed = true
			}
		}

		m := newChatTUI(rt.Agent, rt.Bridge, cfg, rt.SessionID)
		p := tea.NewProgram(&m)

		go func(p *tea.Program) {
			<-ctx.Done()
			p.Send(tuiShutdownMsg{})
		}(p)

		_, runErr := p.Run()
		closeRT()

		if m.restartResumeID == "" || runErr != nil {
			return runErr
		}
		// Relaunch against the requested persisted session.
		resumeID = m.restartResumeID
	}
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

	// RF-1: resolve the effective model route (config / persisted pref) and
	// build the initial per-run requester. Requester build failures are
	// non-fatal: m.requester stays nil and the agent's construction-time
	// requester is used.
	route := resolveModelRoute(cfg, readModelPref())
	modelLabel := route.Provider + "/" + route.Model
	var requester model.ModelRequester
	if route.Endpoint != "" {
		if req, err := buildModelRequester(route.routeConfig()); err == nil {
			requester = req
		}
	}

	// RF-2: effort level from persisted pref, falling back to config. The
	// scales (config overrides + built-ins) are built once; the level is
	// re-validated against the route's scale on /model switches.
	effort := readEffortPref()
	if effort == "" {
		effort = cfg.TUI.ReasoningEffort
	}
	effortScales := buildEffortScales(cfg.TUI.EffortScales)
	if scale, ok := resolveEffortScale(route.Provider, route.Model, effortScales); !ok || !effortLevelValid(scale, effort) {
		effort = ""
	}

	m := chatTUI{
		agent:       ag,
		bridge:      bridge,
		cfg:         cfg,
		sessionID:   sessionID,
		modelLabel:  modelLabel,
		route:       route,
		requester:   requester,
		effort:      effort,
		effortScales: effortScales,
		state:       tuiIdle,
		composer:    composer.New(composer.DefaultConfig()),
		input:       ti,
		spinner:     sp,
		viewport:    vp,
		pending:     &strings.Builder{},
		reasoning:   &strings.Builder{},
		answerIdx:   -1,
		nextPasteID: 1,
		cmdRegistry: buildSlashRegistry(cfg),
		workspace:   *newWorkspaceSwitch(),
		health:      newHealthChecker(cfg),
	}
	// RF-5: load persisted input history for ↑ recall after restart.
	if cfg.Features.InputHistory {
		m.submittedInputs = loadInputHistory()
	}

	// RF-3: restore the persisted theme (best-effort).
	if cfg.Features.ThemeSwitch {
		if theme := readThemePref(); theme != "" {
			_ = applyTheme(theme)
			applyTextareaTheme(&ti)
		}
	}

	// RF-9: show the welcome page on first run (features.welcome gate).
	if cfg.Features.Welcome && !welcomeSeenFrom(welcomeSeenPath()) {
		m.welcome.visible = true
		markWelcomeSeen()
	}
	// SC-6: load ~/.agents/skills into the registry so every installed skill
	// is summonable as /<skill>. Runs after buildSlashRegistry so native
	// commands keep priority over same-named skills.
	registerSkillCommands(m.cmdRegistry, cfg)
	return m
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
	cmds := []tea.Cmd{
		focusCmd,
		textarea.Blink,
		m.waitForAgentEvent(),
	}
	// RF-10: arm the periodic API health check.
	if hc := m.healthTickCmd(); hc != nil {
		cmds = append(cmds, hc)
	}
	return tea.Batch(cmds...)
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
	if cm.taskPanel.Active() && cm.taskPanel.HasTasks() {
		contentW = max(contentW-cm.taskPanel.Width(), taskPanelMinTranscriptWidth)
	}
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
		// SC-3: auto-show the task panel on wide terminals unless the user
		// explicitly toggled it via /tasks.
		if m.cfg.Features.TaskPanel && !m.taskPanelUserSet && msg.Width >= taskPanelAutoShowMinWidth && !m.taskPanel.Active() {
			m.taskPanel.active = true
			m.taskPanel.Sync(m.bridge.TaskStore())
		}
		return m, tea.Batch(cmds...)

	case tuiShutdownMsg:
		m.quit = true
		return m, tea.Quit

	case tea.PasteMsg:
		m.composer.Reset()
		return m.handlePaste(msg)

	case composerTickMsg:
		acts := m.composer.Feed(composer.Event{Kind: composer.EventTick, Now: time.Now()})
		m.applyComposerActions(acts)
		return m, tea.Batch(append(cmds, m.composerTickCmd())...)

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

	case healthTickMsg:
		// RF-10: probe only while idle (never steal bandwidth mid-turn).
		if m.state == tuiIdle {
			m.checkAll()
		}
		// Always re-arm the periodic tick.
		return m, tea.Batch(append(cmds, m.healthTickCmd())...)
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

		// RF-9: welcome page intercepts esc/q (close) and tab (page).
		if m.welcome.visible && m.state == tuiIdle && !m.picker.active {
			switch keyMsg.String() {
			case "esc", "q":
				m.welcome.visible = false
				m.transcriptDirty = true
				return m, tea.Batch(cmds...)
			case "tab":
				tips := tipsForGroup(m.welcome.group)
				pages := (len(tips) + welcomePageSize - 1) / welcomePageSize
				if pages < 1 {
					pages = 1
				}
				m.welcome.page = (m.welcome.page + 1) % pages
				m.transcriptDirty = true
				return m, tea.Batch(cmds...)
			}
		}

		// RF-1: model picker intercepts navigation keys while active.
		if m.picker.active && m.state == tuiIdle {
			if m.handlePickerKey(keyMsg.String()) {
				return m, tea.Batch(cmds...)
			}
		}

		// SC-4: message selection mode + action menu intercept keys first.
		if m.messageActions.active {
			switch keyMsg.String() {
			case "esc":
				if m.messageActions.MenuVisible() {
					m.messageActions.CloseMenu()
				} else {
					m.messageActions.Exit()
					m.commitSystemNote(dim("Message actions cancelled."))
				}
				m.transcriptDirty = true
				return m, tea.Batch(cmds...)
			case "up":
				if m.messageActions.MenuVisible() {
					m.messageActions.MoveMenu(-1)
				} else {
					m.messageActions.Move(-1)
				}
				m.transcriptDirty = true
				return m, tea.Batch(cmds...)
			case "down":
				if m.messageActions.MenuVisible() {
					m.messageActions.MoveMenu(+1)
				} else {
					m.messageActions.Move(+1)
				}
				m.transcriptDirty = true
				return m, tea.Batch(cmds...)
			case "o", "enter":
				if !m.messageActions.MenuVisible() {
					m.messageActions.OpenMenu()
					m.transcriptDirty = true
					return m, tea.Batch(cmds...)
				}
				m.executeMessageAction()
				return m, tea.Batch(cmds...)
			default:
				// Swallow other keys while in selection mode.
				return m, tea.Batch(cmds...)
			}
		}
		// SC-4: enter message selection mode on m/a when idle, no popup, no
		// pending approval and an empty input box (typing m/a stays normal).
		if (keyMsg.String() == "m" || keyMsg.String() == "a") &&
			m.cfg.Features.MessageActions && m.state == tuiIdle &&
			!m.completion.active && m.pendingApproval == nil &&
			strings.TrimSpace(m.input.Value()) == "" {
			m.messageActions.EnterSelectionMode(m.collectUserMessages())
			if m.messageActions.Active() {
				m.commitSystemNote(dim("Message selection mode: [↑↓] select, [o] menu, [Esc] exit"))
				m.transcriptDirty = true
				return m, tea.Batch(cmds...)
			}
		}

		// Composer: printable characters route through the input state
		// machine; special keys (except Enter) flush pending state first.
		// "v" is reserved for visual selection and must reach the switch.
		if keyMsg.Text != "" && keyMsg.String() != "v" {
			var kind composer.EventKind
			rs := []rune(keyMsg.Text)
			if len(rs) == 1 && rs[0] < utf8.RuneSelf {
				kind = composer.EventPlainChar
			} else {
				kind = composer.EventPlainCharNoHold
			}
			acts := m.composer.Feed(composer.Event{Kind: kind, Text: keyMsg.Text, Now: time.Now()})
			m.applyComposerActions(acts)
			// SC-2: refresh the completion popup after the character lands
			// in the input.
			if m.cfg.Features.SlashPopup {
				if m.completion.wantsOpen(m.input.Value(), m.state) {
					m.completion.Refresh(strings.TrimPrefix(m.input.Value(), "/"), m.cmdRegistry)
				} else if !m.completion.isCycling(m.input.Value()) && m.completion.active {
					m.completion.Close()
				}
			}
			return m, tea.Batch(append(cmds, m.composerTickCmd())...)
		}
		if keyMsg.String() != "enter" {
			acts := m.composer.Feed(composer.Event{Kind: composer.EventModifiedInput, Now: time.Now()})
			m.applyComposerActions(acts)
		}

		// SC-2: keep the popup in sync with the input context for all other
		// keys (tab/enter/esc/up/down handled inside the switch below).
		if m.cfg.Features.SlashPopup {
			if m.completion.wantsOpen(m.input.Value(), m.state) {
				m.completion.Refresh(strings.TrimPrefix(m.input.Value(), "/"), m.cmdRegistry)
			} else if !m.completion.isCycling(m.input.Value()) && m.completion.active {
				m.completion.Close()
			}
		}

		switch keyMsg.String() {
		case "ctrl+v", "ctrl+y":
			// P1 clipboard bridge: paste image or text from the system
			// clipboard. Ctrl+Y is a fallback for Windows terminals that
			// intercept Ctrl+V.
			if m.state != tuiIdle {
				return m, tea.Batch(cmds...)
			}
			// Flush any pending burst buffer (IN-5 semantics) before
			// reading the clipboard.
			acts := m.composer.Feed(composer.Event{Kind: composer.EventModifiedInput, Now: time.Now()})
			m.applyComposerActions(acts)

			if img, err := ReadClipboardImagePNG(); err == nil && len(img) > 0 {
				path, werr := writeTempPNG(img, "clipboard")
				if werr != nil {
					m.commitSystemNote(warnText("Could not save image attachment."))
					return m, tea.Batch(cmds...)
				}
				w, h := 0, 0
				if w, h, err = imageSizeOf(img); err != nil {
					w, h = 0, 0
				}
				m.pendingImage = &pendingImageAttachment{Path: path, MIMEType: "image/png", Width: w, Height: h}
				m.commitSystemNote(successText(fmt.Sprintf("Image attached: %s (%dx%d)", filepath.Base(path), w, h)))
				return m, tea.Batch(cmds...)
			}

			txt, terr := ReadClipboardText()
			if terr == nil && strings.TrimSpace(txt) != "" {
				// Explicit paste: insert the full text (including internal
				// newlines) in one shot.
				m.input.InsertString(txt)
				m.commitSystemNote(successText("Pasted clipboard text."))
				return m, tea.Batch(cmds...)
			}
			if errors.Is(terr, ErrClipboardUnavailable) {
				m.commitSystemNote(warnText("Clipboard unavailable"))
				return m, tea.Batch(cmds...)
			}
			return m, tea.Batch(cmds...)

		case "ctrl+c":
			// In selection mode Ctrl+C copies the selection to the system
			// clipboard (matching "v" copy semantics).
			if m.selectionMode {
				copied := m.copySelection()
				m.exitSelectionMode()
				if copied != "" {
					m.commitSystemNote(successText("Copied to clipboard."))
				}
				return m, tea.Batch(cmds...)
			}
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

		case "ctrl+r":
			// P2 render toggle: switch between rich (ANSI/markdown) and raw
			// (plain-text source) transcript rendering. Input state unaffected.
			m.renderRaw = !m.renderRaw
			m.transcriptDirty = true
			m.commitSystemNote(dim(fmt.Sprintf("Render mode: %s", map[bool]string{false: "rich", true: "raw"}[m.renderRaw])))
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
			if m.completion.active {
				m.completion.Close()
				return m, tea.Batch(cmds...)
			}
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
			// SC-2: popup Tab — single candidate completes; multiple align
			// to the longest common prefix first, then cycle through the
			// candidates committing each name into the input (placeholder
			// semantics: nothing executes until Enter).
			if m.completion.active {
				switch len(m.completion.items) {
				case 1:
					m.input.SetValue("/" + m.completion.items[0].Name + " ")
					m.completion.Close()
					return m, tea.Batch(cmds...)
				case 0:
					m.completion.Close()
					return m, tea.Batch(cmds...)
				default:
					if newInput := m.completion.Cycle(m.input.Value()); newInput != "" {
						m.input.SetValue(newInput)
					}
					return m, tea.Batch(cmds...)
				}
			}
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
			// SC-2: popup Enter — commit the selected candidate. Skill
			// candidates are placeholders (SC-6): the command name lands in
			// the input box and is only activated when the user confirms
			// with a second Enter. All other candidates dispatch directly.
			if m.completion.active {
				if sel := m.completion.Selected(); sel != nil {
					cmd, quit := m.commitPopupSelection(sel)
					if quit {
						return m, tea.Quit
					}
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
					return m, tea.Batch(cmds...)
				}
				// No selection: fall through to the normal submit path.
			}
			if m.state != tuiIdle {
				return m, tea.Batch(cmds...)
			}
			acts := m.composer.Feed(composer.Event{Kind: composer.EventEnter, Now: time.Now()})
			submit := false
			for _, a := range acts {
				switch a.Kind {
				case composer.ActionTyped:
					m.input.InsertString(a.Text)
				case composer.ActionInsertNewline:
					m.input.InsertString("\n")
				case composer.ActionSubmit:
					submit = true
				}
			}
			if !submit {
				return m, tea.Batch(append(cmds, m.composerTickCmd())...)
			}
			// P1 vision precheck (PRD §7 双层门控 TUI 侧): if a clipboard image
			// is staged and the active model is KNOWN not to support vision,
			// surface a readable warning and drop the attachment so we never
			// silently send bytes a non-vision model cannot accept. Unknown /
			// vision models keep the attachment (model-layer gateMultimodal
			// still guards the wire path).
			if m.pendingImage != nil {
				cap, found := model.LookupModelCapability(m.route.Model)
				if found && !cap.Vision {
					m.commitSystemNote(warnText(fmt.Sprintf(
						"Non-vision model %s: dropped image attachment (switch to a vision model to send images).",
						m.modelLabel)))
					m.pendingImage = nil
				}
			}
			val := strings.TrimSpace(m.input.Value())
			if val == "" {
				return m, tea.Batch(cmds...)
			}
			m.input.Reset()
			return m.handleSubmit(val, cmds)

		case "up":
			// SC-2: popup selection — intercept before input history.
			if m.completion.active {
				m.completion.Move(-1)
				return m, tea.Batch(cmds...)
			}
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
			// SC-2: popup selection — intercept before input history.
			if m.completion.active {
				m.completion.Move(+1)
				return m, tea.Batch(cmds...)
			}
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
		OnLLMCallEnd: func(ctx context.Context, round int, tokens int, usage *model.UsageInfo) {
			if original.OnLLMCallEnd != nil {
				original.OnLLMCallEnd(ctx, round, tokens, usage)
			}
			if override.OnLLMCallEnd != nil {
				override.OnLLMCallEnd(ctx, round, tokens, usage)
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

	// RF-5: persist the input for ↑ recall after restart (best-effort).
	if m.cfg.Features.InputHistory {
		go appendInputHistory(message)
	}

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

	// P1 image attachment: if a clipboard image was staged (and not dropped
	// by the vision pre-check), attach it as a ContentBlock so the engine
	// sends it to the model. Cleared regardless once the turn starts.
	runOpts := []agent.RunOption{
		agent.WithSystemPrompt(sysPrompt),
		agent.WithCallbacks(mergedCB),
	}
	// RF-1: per-run model route (/model runtime switching). When the route
	// requester failed to build at startup, keep the agent's default.
	if m.requester != nil {
		runOpts = append(runOpts, agent.WithModelRequester(m.requester))
	}
	// RF-2: per-run effort injection (reasoning_effort). nil = no injection.
	if opts := m.effortOptions(); opts != nil {
		runOpts = append(runOpts, agent.WithModelOptions(opts))
	}
	if m.pendingImage != nil {
		if data, err := os.ReadFile(m.pendingImage.Path); err == nil && len(data) > 0 {
			runOpts = append(runOpts, agent.WithContentBlocks([]model.ContentBlock{
				model.ImageBlock(m.pendingImage.MIMEType, data),
			}))
		}
		m.pendingImage = nil
	}

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
		_, _ = m.agent.Run(context.Background(), message, runOpts...)

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
		// RF-6/7: reset per-turn metrics.
		m.resetTurnStats(time.Now())

	case agent.EventToken:
		m.pending.WriteString(e.Text)
		m.streamAnswer()
		// RF-6/7: close the thinking window; accumulate output chars.
		m.endThinking(time.Now())
		m.receipt.totalOutputChars += len(e.Text)
		m.tps.OnToken(len(e.Text))

	case agent.EventReasoning:
		m.reasoning.WriteString(e.Text)
		if m.showReasoning {
			m.flushStreamingReasoning()
		}
		// RF-6: open (or extend) the thinking timing window.
		m.beginThinking(time.Now())

	case agent.EventToolStart:
		m.finalizeStreamingAnswer()
		sandboxMode := ""
		sideEffect := ""
		if e.Metadata != nil {
			sandboxMode = e.Metadata["sandboxMode"]
			sideEffect = e.Metadata["sideEffectLevel"]
		}
		m.commitToolCardEx(e.ToolName, "running", sandboxMode, sideEffect, "")
		// RF-6: record the tool start timestamp.
		m.endThinking(time.Now())
		m.trackToolStart(e.ToolName, time.Now())

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
		// RF-6: close the tool timing window and count the call.
		m.trackToolEnd(e.ToolName, time.Now())
		// T5: show task progress on task tool completion.
		if isTaskTool(e.ToolName) && e.Err == nil {
			if summary := m.bridge.TaskSummary(); summary != "" {
				m.commitSystemNote("task progress: " + summary)
			}
		}
		// SC-3: keep the task panel in sync with the tracker.
		if m.cfg.Features.TaskPanel && isTaskTool(e.ToolName) {
			m.taskPanel.Sync(m.bridge.TaskStore())
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
		// RF-6: close any open timing windows; RF-7: record the TPS sample.
		m.endThinking(time.Now())
		m.tps.OnRunEnd()
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
		// RF-6/8: capture provider-reported usage (reasoning tokens, cache).
		if e.Usage != nil {
			m.receipt.usage = e.Usage
			m.receipt.reasoningTokens = e.Usage.ReasoningTokens()
			if e.Usage.PromptTokens > 0 {
				m.receipt.promptTokens = e.Usage.PromptTokens
			}
			if e.Usage.CompletionTokens > 0 {
				m.receipt.completionTokens = e.Usage.CompletionTokens
			}
		}

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

	// SC-2: completion popup rows between the status bar and the input box.
	if rows := m.completion.Render(boxW); rows != "" {
		parts = append(parts, rows)
		rowsAboveBox += strings.Count(rows, "\n") + 1
	}

	// SC-4: message action menu rows (above the input box, below status).
	if rows := m.messageActions.Render(boxW); rows != "" {
		parts = append(parts, rows)
		rowsAboveBox += strings.Count(rows, "\n") + 1
	}

	// RF-1: model picker rows (above the input box, below status).
	if rows := m.picker.Render(boxW); rows != "" {
		parts = append(parts, rows)
		rowsAboveBox += strings.Count(rows, "\n") + 1
	}

	// Input box.
	inputStyle := inputBoxStyle.Width(boxW)
	inputBox := inputStyle.Render(m.input.View())
	parts = append(parts, inputBox)

	// Full frame: transcript viewport on top, bottom region beneath.
	mainArea := m.viewport.View()
	// RF-9: welcome page rendered above the transcript (first-run guide).
	if rows := m.renderWelcome(m.width); rows != "" {
		mainArea = rows + "\n" + mainArea
	}
	// SC-3: task panel rendered as a right-hand column beside the transcript
	// — only when it has real todo content; an empty panel renders nothing
	// and reserves no width.
	if m.taskPanel.Active() && m.taskPanel.HasTasks() {
		panelW := m.taskPanel.Width()
		panel := m.taskPanel.Render(panelW, m.viewport.Height())
		mainArea = sideBySide(mainArea, panel, m.viewport.Height())
	}
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
	if m.completion.active {
		rows += len(m.completion.items) // SC-2: popup rows
	}
	if m.messageActions.Active() && m.messageActions.MenuVisible() {
		rows += 2 + len(m.messageActions.menuItems) // SC-4: menu header + divider + items
	}
	if m.picker.active {
		rows += 2 // RF-1: header + hint
		n := len(m.picker.providers)
		if m.picker.level == 1 {
			n = len(m.picker.models)
		}
		if n > 15 {
			n = 15 // capped picker rows (mirrors modelPicker.Render)
		}
		rows += n
	}
	rows += m.input.Height() + 2 // input box + border
	return rows
}

func (m *chatTUI) transcriptHeight() int {
	h := m.height - m.bottomRows()
	if m.welcome.visible {
		// RF-9: the welcome panel sits above the transcript; reserve its rows.
		if rows := strings.Count(m.renderWelcome(m.width), "\n"); rows > 0 {
			h -= rows + 1
		}
	}
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
