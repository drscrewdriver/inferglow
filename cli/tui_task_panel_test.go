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
	"path/filepath"
	"strings"
	"testing"

	"github.com/inferglow/builtins/actions"
)

func newTestStore(t *testing.T) *actions.TaskStore {
	t.Helper()
	return actions.NewTaskStore(filepath.Join(t.TempDir(), "tasks.json"))
}

func TestTaskPanelToggle(t *testing.T) {
	var p TaskPanel
	if p.Active() {
		t.Fatal("panel should start hidden")
	}
	p.Toggle()
	if !p.Active() {
		t.Fatal("Toggle() should show the panel")
	}
	p.Toggle()
	if p.Active() {
		t.Fatal("second Toggle() should hide the panel")
	}
}

func TestTaskPanelRenderStates(t *testing.T) {
	store := newTestStore(t)
	done := store.Add("任务完成", "", 0)
	store.Update(done.ID, map[string]any{"status": "done"})
	store.Add("任务进行中", "", 0)
	inProgress := store.Add("任务待处理", "", 0)
	store.Update(inProgress.ID, map[string]any{"status": "in_progress"})
	store.Add("任务取消", "", 0)
	cancelled := store.Add("任务取消2", "", 0)
	store.Update(cancelled.ID, map[string]any{"status": "cancelled"})

	var p TaskPanel
	p.active = true
	p.Sync(store)
	if len(p.tasks) != 5 {
		t.Fatalf("Sync() tasks = %d, want 5", len(p.tasks))
	}

	out := p.Render(30, 12)
	if !strings.Contains(out, "Todo") {
		t.Fatalf("Render missing title:\n%s", out)
	}
	if !strings.Contains(out, "[✓]") {
		t.Fatalf("Render missing completed icon:\n%s", out)
	}
	if !strings.Contains(out, "[•]") {
		t.Fatalf("Render missing in-progress icon:\n%s", out)
	}
	if !strings.Contains(out, "[ ]") {
		t.Fatalf("Render missing pending icon:\n%s", out)
	}
	if !strings.Contains(out, "[✕]") {
		t.Fatalf("Render missing cancelled icon:\n%s", out)
	}
	// Height contract: exactly height rows.
	if got := strings.Count(out, "\n") + 1; got != 12 {
		t.Fatalf("Render rows = %d, want 12", got)
	}
}

func TestTaskPanelRenderHidden(t *testing.T) {
	var p TaskPanel
	if out := p.Render(30, 10); out != "" {
		t.Fatalf("hidden panel should render empty, got %q", out)
	}
}

func TestTaskPanelSyncEmpty(t *testing.T) {
	var p TaskPanel
	p.Sync(newTestStore(t))
	if len(p.tasks) != 0 {
		t.Fatalf("Sync(empty store) tasks = %d, want 0", len(p.tasks))
	}
	p.active = true
	if p.HasTasks() {
		t.Fatal("empty panel must not report tasks")
	}
	// SC-3: an active panel with no real todo content renders nothing and
	// reserves no space (no empty shell / placeholder).
	if out := p.Render(30, 8); out != "" {
		t.Fatalf("empty panel should render empty, got %q", out)
	}
}

func TestTaskPanelHasTasks(t *testing.T) {
	var p TaskPanel
	if p.HasTasks() {
		t.Fatal("fresh panel must not report tasks")
	}
	store := newTestStore(t)
	store.Add("一个任务", "", 0)
	p.Sync(store)
	if !p.HasTasks() {
		t.Fatal("panel with synced tasks must report tasks")
	}
	p.active = true
	if out := p.Render(30, 8); out == "" {
		t.Fatal("panel with tasks must render")
	}
}

func TestTaskPanelWidth(t *testing.T) {
	var p TaskPanel
	if p.Width() != defaultTaskPanelWidth {
		t.Fatalf("default width = %d, want %d", p.Width(), defaultTaskPanelWidth)
	}
	p.SetWidth(42)
	if p.Width() != 42 {
		t.Fatalf("SetWidth(42) width = %d", p.Width())
	}
}

func TestSideBySide(t *testing.T) {
	main := "a\nbb\nccc"
	side := "1\n2\n3\n4"
	out := sideBySide(main, side, 4)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("sideBySide rows = %d, want 4", len(lines))
	}
	if !strings.HasPrefix(lines[0], "a1") || !strings.HasPrefix(lines[2], "ccc3") {
		t.Fatalf("sideBySide merge wrong:\n%q", out)
	}
	// Main column shorter than rows: missing main lines padded.
	if !strings.HasPrefix(lines[3], "4") {
		t.Fatalf("sideBySide tail row wrong:\n%q", out)
	}
}
