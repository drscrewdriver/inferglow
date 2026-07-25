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

// TeamConfig defines a team of agents for coordinated execution.
type TeamConfig struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Members   []TeamMemberConfig `json:"members"`
	MaxRounds int                `json:"max_rounds,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

// TeamMemberConfig defines a single member within a team.
type TeamMemberConfig struct {
	AgentID string   `json:"agent_id"`
	Role    string   `json:"role"`
	Handoff []string `json:"handoff,omitempty"`
}

// TeamStore is an in-memory store for team definitions.
// The backing KV storage is provided by the generic storage.Map primitive.
type TeamStore struct {
	*storage.Map[string, *TeamConfig]
	metaMu sync.RWMutex // guards nextID + the id-assembly critical section
	nextID int
}

// NewTeamStore creates an empty TeamStore.
func NewTeamStore() *TeamStore {
	return &TeamStore{
		Map: storage.NewMap[string, *TeamConfig](),
	}
}

// Create adds a new team and returns its ID.
func (ts *TeamStore) Create(cfg TeamConfig) (string, error) {
	if cfg.Name == "" {
		return "", fmt.Errorf("team name is required")
	}
	if len(cfg.Members) == 0 {
		return "", fmt.Errorf("at least one member is required")
	}

	ts.metaMu.Lock()
	defer ts.metaMu.Unlock()

	ts.nextID++
	id := fmt.Sprintf("team-%d", ts.nextID)
	cfg.ID = id
	cfg.CreatedAt = time.Now()
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 3
	}

	ts.Map.Set(id, &cfg)
	return id, nil
}

// Get returns a team by ID, or nil if not found.
func (ts *TeamStore) Get(id string) *TeamConfig {
	v, _ := ts.Map.Get(id)
	return v
}

// List returns all team definitions.
func (ts *TeamStore) List() []*TeamConfig {
	return ts.Map.Values()
}

// Delete removes a team by ID.
func (ts *TeamStore) Delete(id string) error {
	if _, ok := ts.Map.Get(id); !ok {
		return fmt.Errorf("team %q not found", id)
	}
	ts.Map.Delete(id)
	return nil
}
