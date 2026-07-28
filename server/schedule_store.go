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
	"sync"
	"time"

	"github.com/inferglow/storage"
)

// ScheduleRecord describes a single scheduler entry (spec C-5). Schedules are
// built on the existing trigger.CronTrigger: a stateful schedule is persisted
// agent-side and rebuilt+registered on start, while a stateless one is only
// registered in memory. Both share a periodic Interval; the unused
// CronConfig.Expr remains untouched (the running loop parses only Interval).
type ScheduleRecord struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Flow      string        `json:"flow"`
	Interval  time.Duration `json:"interval"`
	Stateful  bool          `json:"stateful"`
	Enabled   bool          `json:"enabled"`
	CreatedAt time.Time     `json:"created_at"`
}

// ScheduleStore is an in-memory store for scheduler records, mirroring the
// TeamStore template (team_store.go) over the generic storage.Map primitive.
type ScheduleStore struct {
	*storage.Map[string, *ScheduleRecord]
	metaMu sync.RWMutex // guards nextID + the id-assembly critical section
	nextID int
}

// NewScheduleStore creates an empty ScheduleStore.
func NewScheduleStore() *ScheduleStore {
	return &ScheduleStore{
		Map: storage.NewMap[string, *ScheduleRecord](),
	}
}

// Create adds a new schedule and returns its ID.
func (sc *ScheduleStore) Create(rec ScheduleRecord) (string, error) {
	if rec.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if rec.Flow == "" {
		return "", fmt.Errorf("flow is required")
	}
	if rec.Interval <= 0 {
		return "", fmt.Errorf("interval must be positive")
	}

	sc.metaMu.Lock()
	defer sc.metaMu.Unlock()

	sc.nextID++
	id := fmt.Sprintf("sched-%d", sc.nextID)
	rec.ID = id
	rec.CreatedAt = time.Now()

	sc.Map.Set(id, &rec)
	return id, nil
}

// Get returns a schedule by ID, or nil if not found.
func (sc *ScheduleStore) Get(id string) *ScheduleRecord {
	v, _ := sc.Map.Get(id)
	return v
}

// List returns all schedule records.
func (sc *ScheduleStore) List() []*ScheduleRecord {
	return sc.Map.Values()
}

// Delete removes a schedule by ID.
func (sc *ScheduleStore) Delete(id string) error {
	if _, ok := sc.Map.Get(id); !ok {
		return fmt.Errorf("schedule %q not found", id)
	}
	sc.Map.Delete(id)
	return nil
}
