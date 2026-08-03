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

package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/memory"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestStore creates a memory.Store backed by a temporary directory
// and saves the given memories into it. Cleanup is automatic via t.TempDir.
// This helper is shared by memory tests (memory_forget_test.go, memory_recall_test.go).
func newTestStore(t *testing.T, memories ...memory.Memory) memory.Store {
	t.Helper()
	store := memory.Store{Dir: t.TempDir()}
	for _, m := range memories {
		if _, err := store.Save(m); err != nil {
			t.Fatalf("failed to save memory %q: %v", m.Name, err)
		}
	}
	return store
}

// newTaskTestStore creates a TaskStore backed by a temp file.
func newTaskTestStore(t *testing.T) *TaskStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	return NewTaskStore(path)
}

// addTask is a convenience helper to add a task and return it.
func addTask(t *testing.T, ts *TaskStore, title, desc string, priority int) *Task {
	t.Helper()
	return ts.Add(title, desc, priority)
}

// ---------------------------------------------------------------------------
// TaskStore.Add
// ---------------------------------------------------------------------------

func TestTaskStore_Add(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task one", "first task", 0)
	if t1.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if t1.Title != "task one" {
		t.Errorf("Title = %q, want %q", t1.Title, "task one")
	}
	if t1.Description != "first task" {
		t.Errorf("Description = %q, want %q", t1.Description, "first task")
	}
	if t1.Status != "pending" {
		t.Errorf("Status = %q, want pending", t1.Status)
	}
	if t1.Priority != 0 {
		t.Errorf("Priority = %d, want 0", t1.Priority)
	}
	if t1.CreatedAt == 0 {
		t.Error("CreatedAt should not be zero")
	}
	if t1.UpdatedAt == 0 {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestTaskStore_Add_IDAutoIncrement(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task one", "", 0)
	t2 := ts.Add("task two", "", 0)
	t3 := ts.Add("task three", "", 0)

	if t1.ID != "t-1" {
		t.Errorf("first ID = %q, want t-1", t1.ID)
	}
	if t2.ID != "t-2" {
		t.Errorf("second ID = %q, want t-2", t2.ID)
	}
	if t3.ID != "t-3" {
		t.Errorf("third ID = %q, want t-3", t3.ID)
	}
}

func TestTaskStore_Add_HighPriority(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("high prio", "", 1)
	if t1.Priority != 1 {
		t.Errorf("Priority = %d, want 1", t1.Priority)
	}
}

// ---------------------------------------------------------------------------
// TaskStore.Update
// ---------------------------------------------------------------------------

func TestTaskStore_Update_Status(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task", "", 0)
	updated, err := ts.Update(t1.ID, map[string]any{"status": "in_progress"})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated.Status != "in_progress" {
		t.Errorf("Status = %q, want in_progress", updated.Status)
	}
	if updated.ID != t1.ID {
		t.Errorf("ID changed: %q -> %q", t1.ID, updated.ID)
	}
}

func TestTaskStore_Update_Title(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("old title", "", 0)
	updated, err := ts.Update(t1.ID, map[string]any{"title": "new title"})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated.Title != "new title" {
		t.Errorf("Title = %q, want %q", updated.Title, "new title")
	}
}

func TestTaskStore_Update_Description(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task", "old desc", 0)
	updated, err := ts.Update(t1.ID, map[string]any{"description": "new desc"})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated.Description != "new desc" {
		t.Errorf("Description = %q, want %q", updated.Description, "new desc")
	}
}

func TestTaskStore_Update_Priority(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task", "", 0)
	updated, err := ts.Update(t1.ID, map[string]any{"priority": float64(1)})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated.Priority != 1 {
		t.Errorf("Priority = %d, want 1", updated.Priority)
	}
}

func TestTaskStore_Update_NotFound(t *testing.T) {
	ts := newTaskTestStore(t)
	_, err := ts.Update("t-999", map[string]any{"status": "done"})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestTaskStore_Update_EmptyStringFieldsIgnored(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task", "desc", 0)
	// Empty string fields should not overwrite existing values
	_, err := ts.Update(t1.ID, map[string]any{"title": "", "description": ""})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	// Verify original values are preserved
	tasks := ts.List("")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "task" {
		t.Errorf("Title = %q, want %q", tasks[0].Title, "task")
	}
	if tasks[0].Description != "desc" {
		t.Errorf("Description = %q, want %q", tasks[0].Description, "desc")
	}
}

// ---------------------------------------------------------------------------
// TaskStore.Delete
// ---------------------------------------------------------------------------

func TestTaskStore_Delete(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task", "", 0)
	if err := ts.Delete(t1.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	tasks := ts.List("")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestTaskStore_Delete_NotFound(t *testing.T) {
	ts := newTaskTestStore(t)
	err := ts.Delete("t-999")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TaskStore.List
// ---------------------------------------------------------------------------

func TestTaskStore_List_All(t *testing.T) {
	ts := newTaskTestStore(t)
	ts.Add("task one", "", 0)
	ts.Add("task two", "", 0)
	ts.Add("task three", "", 0)

	tasks := ts.List("")
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestTaskStore_List_ByStatus(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task one", "", 0)
	t2 := ts.Add("task two", "", 0)
	ts.Update(t1.ID, map[string]any{"status": "done"})
	ts.Update(t2.ID, map[string]any{"status": "in_progress"})

	done := ts.List("done")
	if len(done) != 1 {
		t.Errorf("expected 1 done task, got %d", len(done))
	}
	if len(done) > 0 && done[0].ID != t1.ID {
		t.Errorf("expected done task ID = %q, got %q", t1.ID, done[0].ID)
	}

	inProgress := ts.List("in_progress")
	if len(inProgress) != 1 {
		t.Errorf("expected 1 in_progress task, got %d", len(inProgress))
	}

	pending := ts.List("pending")
	if len(pending) != 0 {
		t.Errorf("expected 0 pending tasks, got %d", len(pending))
	}
}

func TestTaskStore_List_EmptyFilterReturnsAll(t *testing.T) {
	ts := newTaskTestStore(t)
	ts.Add("task one", "", 0)
	ts.Add("task two", "", 0)

	all := ts.List("")
	if len(all) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(all))
	}
}

func TestTaskStore_List_EmptyStore(t *testing.T) {
	ts := newTaskTestStore(t)
	tasks := ts.List("")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
	tasks = ts.List("pending")
	if len(tasks) != 0 {
		t.Errorf("expected 0 pending tasks, got %d", len(tasks))
	}
}

func TestTaskStore_List_SortedByCreatedAt(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("first", "", 0)
	t2 := ts.Add("second", "", 0)
	t3 := ts.Add("third", "", 0)

	tasks := ts.List("")
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != t1.ID {
		t.Errorf("tasks[0] ID = %q, want %q", tasks[0].ID, t1.ID)
	}
	if tasks[1].ID != t2.ID {
		t.Errorf("tasks[1] ID = %q, want %q", tasks[1].ID, t2.ID)
	}
	if tasks[2].ID != t3.ID {
		t.Errorf("tasks[2] ID = %q, want %q", tasks[2].ID, t3.ID)
	}
}

// ---------------------------------------------------------------------------
// TaskStore.Summary
// ---------------------------------------------------------------------------

func TestTaskStore_Summary_Empty(t *testing.T) {
	ts := newTaskTestStore(t)
	s := ts.Summary()
	if s != "" {
		t.Errorf("expected empty summary, got %q", s)
	}
}

func TestTaskStore_Summary_AllStatuses(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task one", "", 0)
	t2 := ts.Add("task two", "", 0)
	t3 := ts.Add("task three", "", 0)
	ts.Add("task four", "", 0) // remains pending

	ts.Update(t1.ID, map[string]any{"status": "done"})
	ts.Update(t2.ID, map[string]any{"status": "in_progress"})
	ts.Update(t3.ID, map[string]any{"status": "cancelled"})

	s := ts.Summary()
	if s == "" {
		t.Fatal("expected non-empty summary")
	}
	if !strings.Contains(s, "Total: 4") {
		t.Errorf("summary missing 'Total: 4', got: %s", s)
	}
	if !strings.Contains(s, "Done: 1") {
		t.Errorf("summary missing 'Done: 1', got: %s", s)
	}
	if !strings.Contains(s, "In Progress: 1") {
		t.Errorf("summary missing 'In Progress: 1', got: %s", s)
	}
	if !strings.Contains(s, "Pending: 1") {
		t.Errorf("summary missing 'Pending: 1', got: %s", s)
	}
	if !strings.Contains(s, "Cancelled: 1") {
		t.Errorf("summary missing 'Cancelled: 1', got: %s", s)
	}
	if !strings.Contains(s, "Progress: 25%") {
		t.Errorf("summary missing 'Progress: 25%%', got: %s", s)
	}
}

func TestTaskStore_Summary_ActiveTasks(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("high priority", "urgent", 1)
	t2 := ts.Add("normal priority", "", 0)
	ts.Update(t2.ID, map[string]any{"status": "in_progress"})

	s := ts.Summary()
	if !strings.Contains(s, "Active tasks:") {
		t.Errorf("summary missing 'Active tasks:', got: %s", s)
	}
	// High priority task should appear first
	if !strings.Contains(s, t1.ID) {
		t.Errorf("summary missing high priority task ID %s, got: %s", t1.ID, s)
	}
	if !strings.Contains(s, t2.ID) {
		t.Errorf("summary missing in_progress task ID %s, got: %s", t2.ID, s)
	}
}

// ---------------------------------------------------------------------------
// TaskStore Persistence
// ---------------------------------------------------------------------------

func TestTaskStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	// Create store and add tasks
	ts1 := NewTaskStore(path)
	ts1.Add("task one", "", 0)
	ts1.Add("task two", "", 1)

	// Create a new store pointing to the same file
	ts2 := NewTaskStore(path)
	tasks := ts2.List("")
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks after reload, got %d", len(tasks))
	}
	// Collect titles for order-independent comparison
	titles := make(map[string]bool)
	priorities := make(map[string]int)
	for _, t := range tasks {
		titles[t.Title] = true
		priorities[t.Title] = t.Priority
	}
	if !titles["task one"] {
		t.Errorf("expected 'task one' in reloaded tasks, got %v", titles)
	}
	if !titles["task two"] {
		t.Errorf("expected 'task two' in reloaded tasks, got %v", titles)
	}
	if priorities["task two"] != 1 {
		t.Errorf("task two priority = %d, want 1", priorities["task two"])
	}
}

func TestTaskStore_Persistence_IDSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	ts1 := NewTaskStore(path)
	t1 := ts1.Add("task one", "", 0)
	if t1.ID != "t-1" {
		t.Errorf("first ID = %q, want t-1", t1.ID)
	}

	// Reload and add another task
	ts2 := NewTaskStore(path)
	t2 := ts2.Add("task two", "", 0)
	if t2.ID != "t-2" {
		t.Errorf("second ID = %q, want t-2", t2.ID)
	}
}

func TestTaskStore_Load_NonExistentFile(t *testing.T) {
	// NewTaskStore should not fail when file doesn't exist
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	ts := NewTaskStore(path)
	if ts == nil {
		t.Fatal("NewTaskStore returned nil")
	}
	tasks := ts.List("")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestTaskStore_Load_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Should not fail on corrupt file; starts empty
	ts := NewTaskStore(path)
	if ts == nil {
		t.Fatal("NewTaskStore returned nil")
	}
	tasks := ts.List("")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

// ---------------------------------------------------------------------------
// taskAddExecutor
// ---------------------------------------------------------------------------

func TestTaskAddExecutor_TitleRequired(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskAddAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when title is empty")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.Error != "task_add: title is required" {
		t.Errorf("Error = %q, want %q", res.Error, "task_add: title is required")
	}
}

func TestTaskAddExecutor_Success(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskAddAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"title": "my task",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "created" {
		t.Errorf("Status = %q, want created", res.Status)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	if result["title"] != "my task" {
		t.Errorf("title = %q, want %q", result["title"], "my task")
	}
	if result["status"] != "pending" {
		t.Errorf("status = %q, want pending", result["status"])
	}
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Errorf("id should be non-empty string, got %v", result["id"])
	}
}

func TestTaskAddExecutor_WithDescriptionAndPriority(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskAddAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"title":       "my task",
		"description": "a detailed description",
		"priority":    float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	if result["title"] != "my task" {
		t.Errorf("title = %q, want %q", result["title"], "my task")
	}
	// Verify the task was actually stored
	tasks := ts.List("")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in store, got %d", len(tasks))
	}
	if tasks[0].Description != "a detailed description" {
		t.Errorf("Description = %q, want %q", tasks[0].Description, "a detailed description")
	}
	if tasks[0].Priority != 1 {
		t.Errorf("Priority = %d, want 1", tasks[0].Priority)
	}
}

func TestTaskAddExecutor_ActionMetadata(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskAddAction(TaskTrackerConfig{Store: ts})
	if a.Name != TaskAddActionID {
		t.Errorf("Name = %q, want %q", a.Name, TaskAddActionID)
	}
	if a.Description == "" {
		t.Error("Description should not be empty")
	}
	if a.Executor == nil {
		t.Error("Executor should not be nil")
	}
}

// ---------------------------------------------------------------------------
// taskUpdateExecutor
// ---------------------------------------------------------------------------

func TestTaskUpdateExecutor_TaskIDRequired(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskUpdateAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when task_id is empty")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.Error != "task_update: task_id is required" {
		t.Errorf("Error = %q, want %q", res.Error, "task_update: task_id is required")
	}
}

func TestTaskUpdateExecutor_NotFound(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskUpdateAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"task_id": "t-999",
		"status":  "done",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for non-existent task")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if !strings.Contains(res.Error, "not found") {
		t.Errorf("Error = %q, want 'not found'", res.Error)
	}
}

func TestTaskUpdateExecutor_Success(t *testing.T) {
	ts := newTaskTestStore(t)
	task := ts.Add("my task", "description", 0)
	a := NewTaskUpdateAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"task_id": task.ID,
		"status":  "done",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "updated" {
		t.Errorf("Status = %q, want updated", res.Status)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	if result["status"] != "done" {
		t.Errorf("status = %q, want done", result["status"])
	}
}

func TestTaskUpdateExecutor_UpdateMultipleFields(t *testing.T) {
	ts := newTaskTestStore(t)
	task := ts.Add("old title", "old desc", 0)
	a := NewTaskUpdateAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"task_id":     task.ID,
		"title":       "new title",
		"description": "new desc",
		"status":      "in_progress",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	if result["title"] != "new title" {
		t.Errorf("title = %q, want %q", result["title"], "new title")
	}
	if result["status"] != "in_progress" {
		t.Errorf("status = %q, want in_progress", result["status"])
	}
}

func TestTaskUpdateExecutor_ActionMetadata(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskUpdateAction(TaskTrackerConfig{Store: ts})
	if a.Name != TaskUpdateActionID {
		t.Errorf("Name = %q, want %q", a.Name, TaskUpdateActionID)
	}
}

// ---------------------------------------------------------------------------
// taskListExecutor
// ---------------------------------------------------------------------------

func TestTaskListExecutor_NoFilter(t *testing.T) {
	ts := newTaskTestStore(t)
	ts.Add("task one", "", 0)
	ts.Add("task two", "", 0)
	a := NewTaskListAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "ok" {
		t.Errorf("Status = %q, want ok", res.Status)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	count, ok := result["count"].(int)
	if !ok || count != 2 {
		t.Errorf("count = %v, want 2", result["count"])
	}
}

func TestTaskListExecutor_WithStatusFilter(t *testing.T) {
	ts := newTaskTestStore(t)
	t1 := ts.Add("task one", "", 0)
	t2 := ts.Add("task two", "", 0)
	ts.Update(t1.ID, map[string]any{"status": "done"})
	ts.Update(t2.ID, map[string]any{"status": "in_progress"})

	a := NewTaskListAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"status_filter": "done",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	count, ok := result["count"].(int)
	if !ok || count != 1 {
		t.Errorf("count = %v, want 1", result["count"])
	}
	tasks, ok := result["tasks"].([]map[string]any)
	if !ok {
		t.Fatalf("tasks is not []map[string]any: %T", result["tasks"])
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["id"] != t1.ID {
		t.Errorf("task id = %v, want %q", tasks[0]["id"], t1.ID)
	}
	if tasks[0]["status"] != "done" {
		t.Errorf("task status = %v, want done", tasks[0]["status"])
	}
}

func TestTaskListExecutor_EmptyStore(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskListAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	count, ok := result["count"].(int)
	if !ok || count != 0 {
		t.Errorf("count = %v, want 0", result["count"])
	}
}

func TestTaskListExecutor_ActionMetadata(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskListAction(TaskTrackerConfig{Store: ts})
	if a.Name != TaskListActionID {
		t.Errorf("Name = %q, want %q", a.Name, TaskListActionID)
	}
}

// ---------------------------------------------------------------------------
// taskDeleteExecutor
// ---------------------------------------------------------------------------

func TestTaskDeleteExecutor_TaskIDRequired(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskDeleteAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when task_id is empty")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.Error != "task_delete: task_id is required" {
		t.Errorf("Error = %q, want %q", res.Error, "task_delete: task_id is required")
	}
}

func TestTaskDeleteExecutor_NotFound(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskDeleteAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"task_id": "t-999",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for non-existent task")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if !strings.Contains(res.Error, "not found") {
		t.Errorf("Error = %q, want 'not found'", res.Error)
	}
}

func TestTaskDeleteExecutor_Success(t *testing.T) {
	ts := newTaskTestStore(t)
	task := ts.Add("my task", "", 0)
	a := NewTaskDeleteAction(TaskTrackerConfig{Store: ts})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"task_id": task.ID,
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "deleted" {
		t.Errorf("Status = %q, want deleted", res.Status)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	if result["id"] != task.ID {
		t.Errorf("id = %v, want %q", result["id"], task.ID)
	}
	// Verify the task is actually deleted
	tasks := ts.List("")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestTaskDeleteExecutor_ActionMetadata(t *testing.T) {
	ts := newTaskTestStore(t)
	a := NewTaskDeleteAction(TaskTrackerConfig{Store: ts})
	if a.Name != TaskDeleteActionID {
		t.Errorf("Name = %q, want %q", a.Name, TaskDeleteActionID)
	}
}

// ---------------------------------------------------------------------------
// Action Registration
// ---------------------------------------------------------------------------

func TestTaskActions_Registration(t *testing.T) {
	ts := newTaskTestStore(t)
	cfg := TaskTrackerConfig{Store: ts}
	r := action.NewRegistry()

	actions := []*action.Action{
		NewTaskAddAction(cfg),
		NewTaskUpdateAction(cfg),
		NewTaskListAction(cfg),
		NewTaskDeleteAction(cfg),
	}

	for _, a := range actions {
		if err := r.Register(a); err != nil {
			t.Fatalf("Register %q error: %v", a.Name, err)
		}
	}

	// Verify all can be retrieved
	names := []string{TaskAddActionID, TaskUpdateActionID, TaskListActionID, TaskDeleteActionID}
	for _, n := range names {
		got, err := r.Get(n)
		if err != nil {
			t.Errorf("Get %q error: %v", n, err)
			continue
		}
		if got.Name != n {
			t.Errorf("Name = %q, want %q", got.Name, n)
		}
	}
}