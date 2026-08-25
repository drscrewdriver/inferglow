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
	"strings"

	"github.com/inferglow/builtins/actions"
)

const (
	// defaultTaskPanelWidth is the task panel column width (SC-3).
	defaultTaskPanelWidth = 30
	// taskPanelAutoShowMinWidth: terminals wider than this auto-show the panel.
	taskPanelAutoShowMinWidth = 120
	// taskPanelMinTranscriptWidth guarantees the transcript never shrinks below
	// this many columns when the panel is open.
	taskPanelMinTranscriptWidth = 40
)

// TaskStatus is the panel display status of a task.
type TaskStatus int

const (
	TaskPending TaskStatus = iota // [ ] 待处理
	TaskInProgress                // [•] 进行中
	TaskCompleted                 // [✓] 已完成
	TaskCancelled                 // [✕] 已取消
)

// TaskItem is a single task row in the panel (read-only view of TaskStore).
type TaskItem struct {
	ID          string
	Title       string
	Status      TaskStatus
	CreatedAt   int64
	CompletedAt *int64
}

// TaskPanel is the right-side task list panel (SC-3). It is a read-only view
// over the AI-driven TaskTracker store.
type TaskPanel struct {
	active bool
	tasks  []*TaskItem
	width  int
}

// Toggle switches the panel between hidden and visible.
func (p *TaskPanel) Toggle() {
	p.active = !p.active
}

// Active reports whether the panel is visible.
func (p *TaskPanel) Active() bool { return p.active }

// HasTasks reports whether the panel currently has real todo content. The
// panel is only rendered (and only reserves transcript width) when it has
// tasks — an empty panel neither shows nor occupies space (SC-3).
func (p *TaskPanel) HasTasks() bool { return len(p.tasks) > 0 }

// Width returns the panel column width (defaulted when unset).
func (p *TaskPanel) Width() int {
	if p.width <= 0 {
		return defaultTaskPanelWidth
	}
	return p.width
}

// SetWidth overrides the panel column width.
func (p *TaskPanel) SetWidth(w int) {
	p.width = w
}

// Sync refreshes the panel's task list from the task tracker store.
func (p *TaskPanel) Sync(store *actions.TaskStore) {
	if store == nil {
		p.tasks = nil
		return
	}
	list := store.List("")
	p.tasks = make([]*TaskItem, 0, len(list))
	for _, t := range list {
		item := &TaskItem{ID: t.ID, Title: t.Title, CreatedAt: t.CreatedAt}
		switch t.Status {
		case "done":
			item.Status = TaskCompleted
			item.CompletedAt = &t.UpdatedAt
		case "in_progress":
			item.Status = TaskInProgress
		case "cancelled":
			item.Status = TaskCancelled
		default:
			item.Status = TaskPending
		}
		p.tasks = append(p.tasks, item)
	}
}

// Render draws the panel as a column of exactly height rows, each padded to
// width. Returns "" when the panel is hidden OR has no real todo content —
// an empty panel renders nothing and reserves no space.
func (p *TaskPanel) Render(width, height int) string {
	if !p.active || len(p.tasks) == 0 {
		return ""
	}
	if width <= 0 {
		width = p.Width()
	}
	if height < 1 {
		height = 1
	}
	var rows []string
	rows = append(rows, accent("Todo"))
	rows = append(rows, dim(strings.Repeat("─", max(width-1, 1))))
	for _, t := range p.tasks {
		rows = append(rows, p.renderTask(t, width))
	}
	// Pad or truncate to exactly height rows.
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		if i < len(rows) {
			out = append(out, padToWidth(rows[i], width))
		} else {
			out = append(out, strings.Repeat(" ", width))
		}
	}
	return strings.Join(out, "\n")
}

// renderTask renders one task row: "  [✓] title" (truncated to width).
func (p *TaskPanel) renderTask(t *TaskItem, width int) string {
	var icon string
	switch t.Status {
	case TaskCompleted:
		icon = successText("[✓]")
	case TaskInProgress:
		icon = warnText("[•]")
	case TaskCancelled:
		icon = dim("[✕]")
	default:
		icon = dim("[ ]")
	}
	title := truncateToWidth(t.Title, max(width-6, 1))
	return "  " + icon + " " + title
}

// padToWidth pads a string with trailing spaces to exactly width columns
// (approximating width by rune count after stripping ANSI codes).
func padToWidth(s string, width int) string {
	runes := []rune(stripAnsi(s))
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// sideBySide merges a main column and a side column row by row, producing
// exactly rows lines. Missing lines are padded with spaces.
func sideBySide(main, side string, rows int) string {
	mainLines := strings.Split(main, "\n")
	sideLines := strings.Split(side, "\n")
	out := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		ml := ""
		if i < len(mainLines) {
			ml = mainLines[i]
		}
		sl := ""
		if i < len(sideLines) {
			sl = sideLines[i]
		}
		out = append(out, ml+sl)
	}
	return strings.Join(out, "\n")
}
