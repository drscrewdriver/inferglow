package memory

import (
	"path/filepath"
	"testing"
)

func TestJSONGraphStore_New(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")

	gs, err := NewJSONGraphStore(path)
	if err != nil {
		t.Fatalf("NewJSONGraphStore failed: %v", err)
	}
	defer gs.Close()

	if gs == nil {
		t.Fatal("Expected non-nil JSONGraphStore")
	}

	entities := gs.Entities()
	if entities == nil {
		t.Error("Entities() should return non-nil slice (empty)")
	}
	if len(entities) != 0 {
		t.Errorf("New store should have 0 entities, got %d", len(entities))
	}
}

func TestJSONGraphStore_AddTriple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")

	gs, err := NewJSONGraphStore(path)
	if err != nil {
		t.Fatalf("NewJSONGraphStore failed: %v", err)
	}
	defer gs.Close()

	triple := Triple{
		Subject:    "Go",
		Predicate:  "is_a",
		Object:     "programming_language",
		Confidence: 0.95,
		SourceStep: 1,
	}

	if err := gs.AddRelation(triple); err != nil {
		t.Fatalf("AddRelation failed: %v", err)
	}

	// Verify entities were auto-registered
	entities := gs.Entities()
	if len(entities) == 0 {
		t.Fatal("Expected entities after adding triple")
	}

	// Check that both subject and object were registered
	entityMap := make(map[string]Entity)
	for _, e := range entities {
		entityMap[e.Name] = e
	}

	if _, ok := entityMap["Go"]; !ok {
		t.Error("Expected 'Go' to be registered as an entity")
	}
	if _, ok := entityMap["programming_language"]; !ok {
		t.Error("Expected 'programming_language' to be registered as an entity")
	}
}

func TestJSONGraphStore_GetTriples(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")

	gs, err := NewJSONGraphStore(path)
	if err != nil {
		t.Fatalf("NewJSONGraphStore failed: %v", err)
	}
	defer gs.Close()

	// Add triples
	gs.AddRelation(Triple{Subject: "Alice", Predicate: "knows", Object: "Bob"})
	gs.AddRelation(Triple{Subject: "Bob", Predicate: "knows", Object: "Charlie"})
	gs.AddRelation(Triple{Subject: "Alice", Predicate: "works_with", Object: "David"})

	// Search by subject
	results := gs.Search("Alice")
	if len(results) == 0 {
		t.Error("Search('Alice') should return at least 1 result")
	}

	// Search by object
	results = gs.Search("Charlie")
	if len(results) == 0 {
		t.Error("Search('Charlie') should return at least 1 result")
	}

	// Search by predicate
	results = gs.Search("knows")
	if len(results) != 2 {
		t.Errorf("Search('knows') should return 2 results, got %d", len(results))
	}

	// Neighbors of Alice (depth 1)
	neighbors := gs.Neighbors("Alice", 1)
	if len(neighbors) == 0 {
		t.Error("Neighbors('Alice', 1) should return at least 1 result")
	}

	// Neighbors of Alice (depth 2) should include Bob's neighbors
	neighborsDeep := gs.Neighbors("Alice", 2)
	if len(neighborsDeep) < len(neighbors) {
		t.Error("Neighbors with depth 2 should return at least as many results as depth 1")
	}
}

func TestJSONGraphStore_Persist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")

	// Create and populate graph
	gs, err := NewJSONGraphStore(path)
	if err != nil {
		t.Fatalf("NewJSONGraphStore failed: %v", err)
	}

	gs.AddRelation(Triple{Subject: "Go", Predicate: "is_a", Object: "language"})
	gs.AddRelation(Triple{Subject: "Go", Predicate: "created_by", Object: "Google"})
	gs.Close()

	// Reload from same file
	gs2, err := NewJSONGraphStore(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	defer gs2.Close()

	// Verify triples were persisted
	results := gs2.Search("Go")
	if len(results) != 2 {
		t.Errorf("Expected 2 triples after reload, got %d", len(results))
	}

	// Verify entities were persisted
	entities := gs2.Entities()
	if len(entities) == 0 {
		t.Error("Expected entities after reload")
	}
}
