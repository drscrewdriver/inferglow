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

// Triple represents a subject-predicate-object knowledge graph triple (OT-12).
type Triple struct {
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	Confidence float64 `json:"confidence,omitempty"`
	SourceStep int     `json:"source_step,omitempty"`
}

// Entity represents a node in the knowledge graph.
type Entity struct {
	Name string `json:"name"`
	Type string `json:"type"` // e.g., "person", "project", "concept"
}

// KnowledgeGraphStore is the interface for structured long-term memory (OT-12).
// Implementations may range from simple JSON files to graph databases.
type KnowledgeGraphStore interface {
	// UpsertEntity adds or updates an entity in the graph.
	UpsertEntity(name, entityType string) error
	// AddRelation adds a triple (relationship) to the graph.
	AddRelation(t Triple) error
	// Neighbors returns triples involving the given entity within depth hops.
	Neighbors(entity string, depth int) []Triple
	// Search finds triples matching a keyword query.
	Search(query string) []Triple
	// Entities returns all known entities.
	Entities() []Entity
	// Close releases resources.
	Close() error
}

// NoopGraphStore is a no-op implementation for when knowledge graph is disabled.
type NoopGraphStore struct{}

func (NoopGraphStore) UpsertEntity(string, string) error { return nil }
func (NoopGraphStore) AddRelation(Triple) error          { return nil }
func (NoopGraphStore) Neighbors(string, int) []Triple    { return nil }
func (NoopGraphStore) Search(string) []Triple            { return nil }
func (NoopGraphStore) Entities() []Entity                { return nil }
func (NoopGraphStore) Close() error                      { return nil }
