package schema

import (
	"testing"
)

// Check 2.3.1: ParsePath 正确解析 dot-path
func TestParsePathDotPath(t *testing.T) {
	parts := ParsePath("final.steps")

	if len(parts) != 2 {
		t.Fatalf("parts length = %d, want 2", len(parts))
	}

	if parts[0].Key != "final" {
		t.Errorf("parts[0].Key = %q, want %q", parts[0].Key, "final")
	}

	if parts[1].Key != "steps" {
		t.Errorf("parts[1].Key = %q, want %q", parts[1].Key, "steps")
	}
}

// Check 2.3.1b: ParsePath 解析单段
func TestParsePathSingleKey(t *testing.T) {
	parts := ParsePath("name")

	if len(parts) != 1 {
		t.Fatalf("parts length = %d, want 1", len(parts))
	}

	if parts[0].Key != "name" {
		t.Errorf("parts[0].Key = %q, want %q", parts[0].Key, "name")
	}
}

// Check 2.3.1c: ParsePath 解析三段
func TestParsePathThreeSegments(t *testing.T) {
	parts := ParsePath("a.b.c")

	if len(parts) != 3 {
		t.Fatalf("parts length = %d, want 3", len(parts))
	}

	if parts[0].Key != "a" || parts[1].Key != "b" || parts[2].Key != "c" {
		t.Errorf("keys = [%q, %q, %q], want [a, b, c]",
			parts[0].Key, parts[1].Key, parts[2].Key)
	}
}

// Check 2.3.2: ParsePath 正确解析数组通配符
func TestParsePathWildcard(t *testing.T) {
	parts := ParsePath("resources[*].title")

	if len(parts) != 3 {
		t.Fatalf("parts length = %d, want 3", len(parts))
	}

	if parts[0].Key != "resources" {
		t.Errorf("parts[0].Key = %q, want %q", parts[0].Key, "resources")
	}

	if !parts[1].Wild {
		t.Error("parts[1] should be wildcard")
	}

	if parts[2].Key != "title" {
		t.Errorf("parts[2].Key = %q, want %q", parts[2].Key, "title")
	}
}

// Check 2.3.3: ParsePath 正确解析数组索引
func TestParsePathIndex(t *testing.T) {
	parts := ParsePath("tasks[0].id")

	if len(parts) != 3 {
		t.Fatalf("parts length = %d, want 3", len(parts))
	}

	if parts[0].Key != "tasks" {
		t.Errorf("parts[0].Key = %q, want %q", parts[0].Key, "tasks")
	}

	if parts[1].Index != 0 {
		t.Errorf("parts[1].Index = %d, want 0", parts[1].Index)
	}

	if parts[2].Key != "id" {
		t.Errorf("parts[2].Key = %q, want %q", parts[2].Key, "id")
	}
}

// Check 2.3.4: LocatePathInDict 正确定位嵌套字段
func TestLocatePathInDictSimple(t *testing.T) {
	data := map[string]any{
		"final": map[string]any{
			"steps": []any{1, 2, 3},
		},
	}

	result, ok := LocatePathInDict(data, "final.steps")
	if !ok {
		t.Fatal("LocatePathInDict should find final.steps")
	}

	steps, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}

	if len(steps) != 3 {
		t.Errorf("len(steps) = %d, want 3", len(steps))
	}
}

// Check 2.3.5: 通配符 [*] 正确遍历数组并收集结果
func TestLocatePathInDictWildcard(t *testing.T) {
	data := map[string]any{
		"resources": []any{
			map[string]any{"title": "Resource 1"},
			map[string]any{"title": "Resource 2"},
			map[string]any{"title": "Resource 3"},
		},
	}

	result, ok := LocatePathInDict(data, "resources[*].title")
	if !ok {
		t.Fatal("LocatePathInDict should find resources[*].title")
	}

	items, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}

	if len(items) != 3 {
		t.Errorf("len(items) = %d, want 3", len(items))
	}
}

// Test LocatePathInDict with index
func TestLocatePathInDictIndex(t *testing.T) {
	data := map[string]any{
		"tasks": []any{
			map[string]any{"id": "task-1", "title": "First"},
			map[string]any{"id": "task-2", "title": "Second"},
		},
	}

	result, ok := LocatePathInDict(data, "tasks[1].id")
	if !ok {
		t.Fatal("LocatePathInDict should find tasks[1].id")
	}

	id, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}

	if id != "task-2" {
		t.Errorf("id = %q, want %q", id, "task-2")
	}
}

// Test LocatePathInDict with missing path
func TestLocatePathInDictMissing(t *testing.T) {
	data := map[string]any{
		"name": "test",
	}

	_, ok := LocatePathInDict(data, "missing.field")
	if ok {
		t.Error("LocatePathInDict should not find missing.path")
	}
}

// Test LocatePathInDict with empty data
func TestLocatePathInDictEmptyData(t *testing.T) {
	_, ok := LocatePathInDict(nil, "any.path")
	if ok {
		t.Error("LocatePathInDict(nil) should not find anything")
	}
}

// Test LocatePathInDict with empty path
func TestLocatePathInDictEmptyPath(t *testing.T) {
	data := map[string]any{"key": "value"}
	result, ok := LocatePathInDict(data, "")
	if !ok {
		t.Error("LocatePathInDict with empty path should return data")
	}
	// Empty path should return the same data pointer
	if _, ok := result.(map[string]any); !ok {
		t.Error("LocatePathInDict with empty path should return a map")
	}
}

// Test LocatePathInDict with nested wildcard in array of dicts
func TestLocatePathInDictNestedWildcard(t *testing.T) {
	data := map[string]any{
		"users": []any{
			map[string]any{
				"name": "Alice",
				"tags": []any{"admin", "user"},
			},
			map[string]any{
				"name": "Bob",
				"tags": []any{"user"},
			},
		},
	}

	result, ok := LocatePathInDict(data, "users[*].name")
	if !ok {
		t.Fatal("should find users[*].name")
	}

	names, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}

	if len(names) != 2 {
		t.Errorf("len(names) = %d, want 2", len(names))
	}
}
