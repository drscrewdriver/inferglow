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

package server

import (
	"fmt"
	"time"
)

// RunQueueTier mirrors dsh-input-traffic's three-level planning tiers.
type RunQueueTier string

const (
	RunQueueTierLater RunQueueTier = "later" // 🟢 等当前 turn 自然完成
	RunQueueTierNext  RunQueueTier = "next"  // 🟡 当前 action 结束后插入
	RunQueueTierNow   RunQueueTier = "now"   // 🔴 打断当前运行，立即发送
)

// RunQueueItem is a single queued input for a run.
type RunQueueItem struct {
	ID        string      `json:"id"`
	RunID     string      `json:"run_id"`
	Tier      RunQueueTier `json:"tier"`
	Text      string      `json:"text"`
	SessionID string      `json:"session_id,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// RunQueue is the ordered list of queued inputs for a single run.
type RunQueue struct {
	items []*RunQueueItem
	seq   int
}

// RunJob is a background sub-task tracked against a run (mirrors dsh-input-traffic JobList).
type RunJob struct {
	ID          string     `json:"id"`
	RunID       string     `json:"run_id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"` // ongoing | stopping | completed | killed | failed
	Description string     `json:"description,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Duration    string     `json:"duration,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// queues is keyed by run ID. Access is guarded by rm.mu.

// QueueList returns the queue items for a run, oldest (highest priority drain
// order) first — the order in which they will be consumed.
func (rm *RunManager) QueueList(id string) ([]*RunQueueItem, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if _, ok := rm.runs[id]; !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	q, ok := rm.queues[id]
	if !ok || q == nil {
		return []*RunQueueItem{}, nil
	}
	return append([]*RunQueueItem{}, q.items...), nil
}

// QueuePush appends a new item to the run's queue at the given tier.
// A "now" tier item is inserted at the front (interrupt) so it drains next.
func (rm *RunManager) QueuePush(id string, tier RunQueueTier, text string, sessionID string) (*RunQueueItem, error) {
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, ok := rm.runs[id]; !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	q, ok := rm.queues[id]
	if !ok {
		q = &RunQueue{}
		rm.queues[id] = q
	}
	q.seq++
	item := &RunQueueItem{
		ID:        fmt.Sprintf("%s-q%d", id, q.seq),
		RunID:     id,
		Tier:      tier,
		Text:      text,
		SessionID: sessionID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if tier == RunQueueTierNow {
		q.items = append([]*RunQueueItem{item}, q.items...)
	} else {
		q.items = append(q.items, item)
	}
	return item, nil
}

// QueueEdit updates the tier and/or text of an existing queue item.
func (rm *RunManager) QueueEdit(id, itemID string, tier RunQueueTier, text string) (*RunQueueItem, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, ok := rm.runs[id]; !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	q, ok := rm.queues[id]
	if !ok {
		return nil, fmt.Errorf("queue item %q not found", itemID)
	}
	for _, it := range q.items {
		if it.ID == itemID {
			if tier != "" {
				it.Tier = tier
			}
			if text != "" {
				it.Text = text
			}
			it.UpdatedAt = time.Now()
			return it, nil
		}
	}
	return nil, fmt.Errorf("queue item %q not found", itemID)
}

// QueueRemove deletes a queue item by ID.
func (rm *RunManager) QueueRemove(id, itemID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, ok := rm.runs[id]; !ok {
		return fmt.Errorf("run %q not found", id)
	}
	q, ok := rm.queues[id]
	if !ok {
		return fmt.Errorf("queue item %q not found", itemID)
	}
	for i, it := range q.items {
		if it.ID == itemID {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("queue item %q not found", itemID)
}

// QueueSteer recolors (and optionally promotes) an item: raising to "now" moves
// it to the front; toFront forces a full reorder by moving the item to the front.
func (rm *RunManager) QueueSteer(id, itemID string, tier RunQueueTier, toFront bool) (*RunQueueItem, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, ok := rm.runs[id]; !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	q, ok := rm.queues[id]
	if !ok {
		return nil, fmt.Errorf("queue item %q not found", itemID)
	}
	var item *RunQueueItem
	idx := -1
	for i, it := range q.items {
		if it.ID == itemID {
			item = it
			idx = i
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("queue item %q not found", itemID)
	}
	if tier != "" {
		item.Tier = tier
	}
	item.UpdatedAt = time.Now()
	if toFront || item.Tier == RunQueueTierNow {
		if idx > 0 {
			q.items = append(q.items[:idx], q.items[idx+1:]...)
			q.items = append([]*RunQueueItem{item}, q.items...)
		}
	}
	return item, nil
}

// QueueClear empties the run's queue.
func (rm *RunManager) QueueClear(id string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, ok := rm.runs[id]; !ok {
		return fmt.Errorf("run %q not found", id)
	}
	delete(rm.queues, id)
	return nil
}

// Jobs returns the background jobs tracked against a run, newest first.
func (rm *RunManager) Jobs(id string) ([]*RunJob, error) {
	rm.mu.RLock()
	h, ok := rm.runs[id]
	rm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*RunJob, 0, len(h.Jobs))
	for i := len(h.Jobs) - 1; i >= 0; i-- {
		out = append(out, h.Jobs[i])
	}
	return out, nil
}

// TrackJob records a new job against a run and emits a job_started event.
func (rm *RunManager) TrackJob(id, kind, description string) (*RunJob, error) {
	rm.mu.RLock()
	h, ok := rm.runs[id]
	rm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	job := &RunJob{
		ID:          fmt.Sprintf("job-%d", time.Now().UnixNano()),
		RunID:       id,
		Kind:        kind,
		Status:      "ongoing",
		Description: description,
		StartedAt:   time.Now(),
	}
	h.mu.Lock()
	h.Jobs = append(h.Jobs, job)
	h.emit(RunEvent{Type: "job_started", Timestamp: time.Now(), Data: job})
	h.mu.Unlock()
	return job, nil
}

// AllJobs returns every background job tracked across all runs, newest first.
// It powers the global GET /v1/jobs listing (Spec B).
func (rm *RunManager) AllJobs() []*RunJob {
	rm.mu.RLock()
	handles := make([]*RunHandle, 0, len(rm.runs))
	for _, h := range rm.runs {
		handles = append(handles, h)
	}
	rm.mu.RUnlock()

	out := make([]*RunJob, 0)
	for _, h := range handles {
		h.mu.Lock()
		for i := len(h.Jobs) - 1; i >= 0; i-- {
			out = append(out, h.Jobs[i])
		}
		h.mu.Unlock()
	}
	return out
}

// FindJob looks up a single background job by ID across all runs.
func (rm *RunManager) FindJob(jobID string) (*RunJob, bool) {
	rm.mu.RLock()
	handles := make([]*RunHandle, 0, len(rm.runs))
	for _, h := range rm.runs {
		handles = append(handles, h)
	}
	rm.mu.RUnlock()

	for _, h := range handles {
		h.mu.Lock()
		for _, j := range h.Jobs {
			if j.ID == jobID {
				h.mu.Unlock()
				return j, true
			}
		}
		h.mu.Unlock()
	}
	return nil, false
}

// UpdateJob mutates the status/error of a tracked job, sets FinishedAt when
// terminal, computes duration, and emits a job_done event.
func (rm *RunManager) UpdateJob(id, jobID, status, errMsg string) (*RunJob, error) {
	rm.mu.RLock()
	h, ok := rm.runs[id]
	rm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var target *RunJob
	for _, j := range h.Jobs {
		if j.ID == jobID {
			target = j
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("job %q not found", jobID)
	}
	if status != "" {
		target.Status = status
	}
	if errMsg != "" {
		target.Error = errMsg
	}
	switch target.Status {
	case "completed", "killed", "failed":
		now := time.Now()
		target.FinishedAt = &now
		target.Duration = now.Sub(target.StartedAt).Round(time.Millisecond).String()
	}
	h.emit(RunEvent{Type: "job_done", Timestamp: time.Now(), Data: target})
	return target, nil
}