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

package memory

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// JSONGraphStore is a file-backed knowledge graph implementation (OT-12 MVP).
// Stores entities and triples in a single JSON file. Suitable for small graphs
// (< 10K triples). For larger graphs, replace with SQLite or a graph DB.
type JSONGraphStore struct {
	mu       sync.RWMutex
	filePath string
	entities map[string]Entity
	triples  []Triple
}

type graphData struct {
	Entities []Entity `json:"entities"`
	Triples  []Triple `json:"triples"`
}

// NewJSONGraphStore creates or loads a JSON-backed knowledge graph.
func NewJSONGraphStore(filePath string) (*JSONGraphStore, error) {
	gs := &JSONGraphStore{
		filePath: filePath,
		entities: make(map[string]Entity),
	}

	// Try to load existing data.
	data, err := os.ReadFile(filePath)
	if err == nil {
		var gd graphData
		if json.Unmarshal(data, &gd) == nil {
			for _, e := range gd.Entities {
				gs.entities[e.Name] = e
			}
			gs.triples = gd.Triples
		}
	}

	return gs, nil
}

func (gs *JSONGraphStore) UpsertEntity(name, entityType string) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.entities[name] = Entity{Name: name, Type: entityType}
	return gs.saveLocked()
}

func (gs *JSONGraphStore) AddRelation(t Triple) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.triples = append(gs.triples, t)
	// Auto-register entities from triples.
	if _, ok := gs.entities[t.Subject]; !ok {
		gs.entities[t.Subject] = Entity{Name: t.Subject, Type: "unknown"}
	}
	if _, ok := gs.entities[t.Object]; !ok {
		gs.entities[t.Object] = Entity{Name: t.Object, Type: "unknown"}
	}
	return gs.saveLocked()
}

func (gs *JSONGraphStore) Neighbors(entity string, depth int) []Triple {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3 // prevent explosion
	}

	// BFS up to depth hops.
	visited := map[string]bool{entity: true}
	frontier := []string{entity}
	var result []Triple

	for d := 0; d < depth && len(frontier) > 0; d++ {
		var nextFrontier []string
		for _, t := range gs.triples {
			for _, node := range frontier {
				if t.Subject == node && !visited[t.Object] {
					result = append(result, t)
					visited[t.Object] = true
					nextFrontier = append(nextFrontier, t.Object)
				} else if t.Object == node && !visited[t.Subject] {
					result = append(result, t)
					visited[t.Subject] = true
					nextFrontier = append(nextFrontier, t.Subject)
				} else if t.Subject == node || t.Object == node {
					result = append(result, t)
				}
			}
		}
		frontier = nextFrontier
	}

	// Limit results.
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

func (gs *JSONGraphStore) Search(query string) []Triple {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	query = strings.ToLower(query)
	var result []Triple
	for _, t := range gs.triples {
		if strings.Contains(strings.ToLower(t.Subject), query) ||
			strings.Contains(strings.ToLower(t.Predicate), query) ||
			strings.Contains(strings.ToLower(t.Object), query) {
			result = append(result, t)
		}
	}
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

func (gs *JSONGraphStore) Entities() []Entity {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	out := make([]Entity, 0, len(gs.entities))
	for _, e := range gs.entities {
		out = append(out, e)
	}
	return out
}

func (gs *JSONGraphStore) Close() error {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.saveLocked()
}

func (gs *JSONGraphStore) saveLocked() error {
	gd := graphData{
		Entities: make([]Entity, 0, len(gs.entities)),
		Triples:  gs.triples,
	}
	for _, e := range gs.entities {
		gd.Entities = append(gd.Entities, e)
	}

	data, err := json.MarshalIndent(gd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(gs.filePath, data, 0644)
}
