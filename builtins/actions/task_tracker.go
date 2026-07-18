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

package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/inferglow/action"
)

// ---------------------------------------------------------------------------
// Task — a single trackable unit of work.
// ---------------------------------------------------------------------------

// Task represents a tracked item with lifecycle states.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"` // pending | in_progress | done | cancelled
	Description string `json:"description,omitempty"`
	Priority    int    `json:"priority"` // 0 = normal, 1 = high
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// TaskStore — concurrency-safe, JSON-persisted task collection.
// ---------------------------------------------------------------------------

// TaskStore manages a set of tasks with JSON file persistence.
type TaskStore struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	filePath string
	seq      int
}

// NewTaskStore creates a TaskStore and attempts to load existing data from
// filePath. If the file does not exist or is corrupt, the store starts empty.
func NewTaskStore(filePath string) *TaskStore {
	ts := &TaskStore{
		tasks:    make(map[string]*Task),
		filePath: filePath,
	}
	_ = ts.load()
	return ts
}

// Add creates a new task and persists the change.
func (ts *TaskStore) Add(title, description string, priority int) *Task {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.seq++
	t := &Task{
		ID:          fmt.Sprintf("t-%d", ts.seq),
		Title:       title,
		Status:      "pending",
		Description: description,
		Priority:    priority,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	ts.tasks[t.ID] = t
	_ = ts.save()
	return t
}

// Update modifies an existing task. Only non-zero fields in changes are applied.
func (ts *TaskStore) Update(id string, changes map[string]any) (*Task, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	t, ok := ts.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}

	if s, _ := changes["status"].(string); s != "" {
		t.Status = s
	}
	if title, _ := changes["title"].(string); title != "" {
		t.Title = title
	}
	if desc, _ := changes["description"].(string); desc != "" {
		t.Description = desc
	}
	if p, ok := changes["priority"].(float64); ok {
		t.Priority = int(p)
	}
	t.UpdatedAt = time.Now().Unix()
	_ = ts.save()
	return t, nil
}

// Delete removes a task by ID.
func (ts *TaskStore) Delete(id string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if _, ok := ts.tasks[id]; !ok {
		return fmt.Errorf("task %q not found", id)
	}
	delete(ts.tasks, id)
	_ = ts.save()
	return nil
}

// List returns tasks filtered by status. If statusFilter is empty, all tasks
// are returned sorted by creation time.
func (ts *TaskStore) List(statusFilter string) []*Task {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []*Task
	for _, t := range ts.tasks {
		if statusFilter != "" && t.Status != statusFilter {
			continue
		}
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt < result[j].CreatedAt
	})
	return result
}

// Summary returns a human-readable progress summary for context injection.
func (ts *TaskStore) Summary() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if len(ts.tasks) == 0 {
		return ""
	}

	var total, done, inProgress, pending, cancelled int
	for _, t := range ts.tasks {
		total++
		switch t.Status {
		case "done":
			done++
		case "in_progress":
			inProgress++
		case "cancelled":
			cancelled++
		default:
			pending++
		}
	}

	pct := 0.0
	if total > 0 {
		pct = float64(done) / float64(total) * 100
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Total: %d | Done: %d | In Progress: %d | Pending: %d", total, done, inProgress, pending)
	if cancelled > 0 {
		fmt.Fprintf(&sb, " | Cancelled: %d", cancelled)
	}
	fmt.Fprintf(&sb, " | Progress: %.0f%%", pct)

	// List active tasks (pending + in_progress) for LLM awareness.
	var active []*Task
	for _, t := range ts.tasks {
		if t.Status == "pending" || t.Status == "in_progress" {
			active = append(active, t)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Priority != active[j].Priority {
			return active[i].Priority > active[j].Priority
		}
		return active[i].CreatedAt < active[j].CreatedAt
	})
	if len(active) > 0 {
		sb.WriteString("\nActive tasks:")
		for _, t := range active {
			marker := "-"
			if t.Status == "in_progress" {
				marker = ">"
			}
			fmt.Fprintf(&sb, "\n  %s [%s] %s", marker, t.ID, t.Title)
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

type taskSnapshot struct {
	Seq   int     `json:"seq"`
	Tasks []*Task `json:"tasks"`
}

func (ts *TaskStore) save() error {
	snap := taskSnapshot{Seq: ts.seq, Tasks: make([]*Task, 0, len(ts.tasks))}
	for _, t := range ts.tasks {
		snap.Tasks = append(snap.Tasks, t)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: write to temp file then rename.
	tmp := ts.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ts.filePath)
}

func (ts *TaskStore) load() error {
	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		return err
	}
	var snap taskSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	ts.seq = snap.Seq
	ts.tasks = make(map[string]*Task, len(snap.Tasks))
	for _, t := range snap.Tasks {
		ts.tasks[t.ID] = t
	}
	return nil
}

// ---------------------------------------------------------------------------
// Action definitions — 4 LLM-callable tools.
// ---------------------------------------------------------------------------

// TaskTrackerConfig holds shared dependencies for all task actions.
type TaskTrackerConfig struct {
	Store *TaskStore
}

// --- task_add ---------------------------------------------------------------

const TaskAddActionID = "task_add"

type taskAddExecutor struct{ store *TaskStore }

// NewTaskAddAction creates the task_add action.
func NewTaskAddAction(cfg TaskTrackerConfig) *action.Action {
	return &action.Action{
		Name:        TaskAddActionID,
		Description: "Add a new task to the progress tracker. Use this to track work items, todos, or goals that need to be completed. Returns the new task ID.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":       map[string]any{"type": "string", "description": "Short title for the task."},
				"description": map[string]any{"type": "string", "description": "Detailed description (optional)."},
				"priority":    map[string]any{"type": "integer", "description": "Priority: 0=normal (default), 1=high."},
			},
			"required": []string{"title"},
		},
		Executor: &taskAddExecutor{store: cfg.Store},
		Tags:     []string{"task", "write", "builtin"},
	}
}

func (e *taskAddExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	title, _ := input["title"].(string)
	if title == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "task_add: title is required"}, nil
	}
	desc, _ := input["description"].(string)
	priority := 0
	if p, ok := input["priority"].(float64); ok {
		priority = int(p)
	}
	t := e.store.Add(title, desc, priority)
	return &action.ActionResult{
		OK:     true,
		Status: "created",
		Result: map[string]any{"id": t.ID, "title": t.Title, "status": t.Status},
	}, nil
}

// --- task_update ------------------------------------------------------------

const TaskUpdateActionID = "task_update"

type taskUpdateExecutor struct{ store *TaskStore }

// NewTaskUpdateAction creates the task_update action.
func NewTaskUpdateAction(cfg TaskTrackerConfig) *action.Action {
	return &action.Action{
		Name:        TaskUpdateActionID,
		Description: "Update an existing task's status, title, or description. Common status transitions: pending -> in_progress -> done. Use 'cancelled' to discard.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id":     map[string]any{"type": "string", "description": "ID of the task to update (e.g. t-1)."},
				"status":      map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "done", "cancelled"}, "description": "New status."},
				"title":       map[string]any{"type": "string", "description": "New title (optional)."},
				"description": map[string]any{"type": "string", "description": "New description (optional)."},
			},
			"required": []string{"task_id"},
		},
		Executor: &taskUpdateExecutor{store: cfg.Store},
		Tags:     []string{"task", "write", "builtin"},
	}
}

func (e *taskUpdateExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	id, _ := input["task_id"].(string)
	if id == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "task_update: task_id is required"}, nil
	}
	t, err := e.store.Update(id, input)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: err.Error()}, nil
	}
	return &action.ActionResult{
		OK:     true,
		Status: "updated",
		Result: map[string]any{"id": t.ID, "title": t.Title, "status": t.Status},
	}, nil
}

// --- task_list --------------------------------------------------------------

const TaskListActionID = "task_list"

type taskListExecutor struct{ store *TaskStore }

// NewTaskListAction creates the task_list action.
func NewTaskListAction(cfg TaskTrackerConfig) *action.Action {
	return &action.Action{
		Name:        TaskListActionID,
		Description: "List current tasks. Optionally filter by status (pending/in_progress/done/cancelled). Returns a formatted list of tasks.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status_filter": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "done", "cancelled"}, "description": "Filter by status (optional, omit for all)."},
			},
		},
		Executor: &taskListExecutor{store: cfg.Store},
		Tags:     []string{"task", "builtin"},
	}
}

func (e *taskListExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	filter, _ := input["status_filter"].(string)
	tasks := e.store.List(filter)
	items := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, map[string]any{
			"id":          t.ID,
			"title":       t.Title,
			"status":      t.Status,
			"priority":    t.Priority,
			"description": t.Description,
		})
	}
	return &action.ActionResult{
		OK:     true,
		Status: "ok",
		Result: map[string]any{"count": len(items), "tasks": items},
	}, nil
}

// --- task_delete ------------------------------------------------------------

const TaskDeleteActionID = "task_delete"

type taskDeleteExecutor struct{ store *TaskStore }

// NewTaskDeleteAction creates the task_delete action.
func NewTaskDeleteAction(cfg TaskTrackerConfig) *action.Action {
	return &action.Action{
		Name:        TaskDeleteActionID,
		Description: "Delete a task from the tracker. Use this to remove tasks that are no longer relevant.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string", "description": "ID of the task to delete (e.g. t-1)."},
			},
			"required": []string{"task_id"},
		},
		Executor: &taskDeleteExecutor{store: cfg.Store},
		Tags:     []string{"task", "write", "builtin"},
	}
}

func (e *taskDeleteExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	id, _ := input["task_id"].(string)
	if id == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "task_delete: task_id is required"}, nil
	}
	if err := e.store.Delete(id); err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: err.Error()}, nil
	}
	return &action.ActionResult{
		OK:     true,
		Status: "deleted",
		Result: map[string]any{"id": id},
	}, nil
}
