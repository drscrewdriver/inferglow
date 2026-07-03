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
	"fmt"
	"strings"

	"github.com/inferglow/action"
	"github.com/inferglow/context/retrieval"
	"github.com/inferglow/memory"
)

// MemoryRecallActionID is the registered Action name for searching/reading memories.
const MemoryRecallActionID = "memory"

const (
	defaultRecallLimit = 8
	maxRecallLimit     = 20
)

// MemoryRecallConfig holds the store for the memory recall action.
type MemoryRecallConfig struct {
	Store memory.Store
}

// recallExecutor is the ActionExecutor for searching/reading memories.
type recallExecutor struct {
	store memory.Store
}

// NewMemoryRecallAction builds a read-only Action for searching, reading,
// and listing saved memories.
func NewMemoryRecallAction(cfg MemoryRecallConfig) *action.Action {
	return &action.Action{
		Name:        MemoryRecallActionID,
		Description: "Search, list, and read saved background memories for this project, including explicitly global facts. Use this before saving a new memory to avoid duplicates, and when a saved memory from the index looks relevant but needs its full body. This tool is read-only; use remember to save or update a memory, and forget to archive one.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{"type": "string", "enum": []string{"search", "read", "list"}, "description": "search ranks saved memories; read returns one full memory by name; list returns the saved-memory index."},
				"query":     map[string]any{"type": "string", "description": "Search query for operation=search."},
				"name":      map[string]any{"type": "string", "description": "Memory name for operation=read."},
				"type":      map[string]any{"type": "string", "enum": []string{"user", "feedback", "project", "reference"}, "description": "Optional memory type filter."},
				"scope":     map[string]any{"type": "string", "enum": []string{"project", "global"}, "description": "Optional scope filter."},
				"limit":     map[string]any{"type": "integer", "description": "Maximum results to return, default 8, max 20."},
			},
			"required": []string{"operation"},
		},
		Executor: &recallExecutor{store: cfg.Store},
		Tags:     []string{"memory", "read", "builtin"},
	}
}

// Execute dispatches to search, read, or list.
func (e *recallExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	operation, _ := input["operation"].(string)
	typeFilter, _ := input["type"].(string)
	scopeFilter, _ := input["scope"].(string)
	limit := clampLimit(input["limit"])

	switch strings.TrimSpace(operation) {
	case "search":
		query, _ := input["query"].(string)
		if query == "" {
			return &action.ActionResult{OK: false, Status: "error", Error: "memory: query is required for search"}, nil
		}
		return e.search(query, typeFilter, scopeFilter, limit)
	case "read":
		name, _ := input["name"].(string)
		if name == "" {
			return &action.ActionResult{OK: false, Status: "error", Error: "memory: name is required for read"}, nil
		}
		return e.read(name)
	case "list":
		return e.list(typeFilter, scopeFilter, limit)
	default:
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("memory: unknown operation %q (use search, read, or list)", operation)}, nil
	}
}

func (e *recallExecutor) search(query, typeFilter, scopeFilter string, limit int) (*action.ActionResult, error) {
	all := e.store.List()

	// Filter by type/scope first.
	var filtered []memory.Memory
	for _, m := range all {
		if matchesFilter(m, typeFilter, scopeFilter) {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return &action.ActionResult{OK: true, Status: "no_results", Result: "No memories matched the query."}, nil
	}

	// BM25 ranked search over filtered memories.
	idx := retrieval.NewBM25Index()
	for i, m := range filtered {
		idx.Add(i, m.Name+" "+m.Title+" "+m.Description+" "+m.Body)
	}
	results, _ := idx.Search(context.Background(), query, limit)

	if len(results) == 0 {
		// Fallback to substring match for very short queries where BM25
		// IDF is unreliable.
		return e.substringSearch(query, filtered, limit)
	}

	var hits []string
	for _, r := range results {
		hits = append(hits, formatHit(filtered[r.StepID]))
	}
	return &action.ActionResult{OK: true, Status: "ok", Result: strings.Join(hits, "\n")}, nil
}

// substringSearch is the fallback search when BM25 returns no results.
func (e *recallExecutor) substringSearch(query string, filtered []memory.Memory, limit int) (*action.ActionResult, error) {
	q := strings.ToLower(query)
	var hits []string
	for _, m := range filtered {
		if strings.Contains(strings.ToLower(m.Name), q) ||
			strings.Contains(strings.ToLower(m.Title), q) ||
			strings.Contains(strings.ToLower(m.Description), q) ||
			strings.Contains(strings.ToLower(m.Body), q) {
			hits = append(hits, formatHit(m))
			if len(hits) >= limit {
				break
			}
		}
	}
	if len(hits) == 0 {
		return &action.ActionResult{OK: true, Status: "no_results", Result: "No memories matched the query."}, nil
	}
	return &action.ActionResult{OK: true, Status: "ok", Result: strings.Join(hits, "\n")}, nil
}

func (e *recallExecutor) read(name string) (*action.ActionResult, error) {
	m, ok := e.store.Load(name)
	if !ok {
		return &action.ActionResult{OK: false, Status: "not_found", Error: fmt.Sprintf("memory: %q not found", name)}, nil
	}
	content := fmt.Sprintf("# %s\n\n- **ID:** %s\n- **Revision:** %d\n- **Type:** %s\n- **Scope:** %s\n- **Description:** %s\n\n---\n\n%s",
		m.Title, m.ID, m.Revision, m.Type, m.Scope, m.Description, m.Body)
	return &action.ActionResult{OK: true, Status: "ok", Result: content}, nil
}

func (e *recallExecutor) list(typeFilter, scopeFilter string, limit int) (*action.ActionResult, error) {
	all := e.store.List()
	var lines []string
	for _, m := range all {
		if !matchesFilter(m, typeFilter, scopeFilter) {
			continue
		}
		lines = append(lines, formatHit(m))
		if len(lines) >= limit {
			break
		}
	}
	if len(lines) == 0 {
		return &action.ActionResult{OK: true, Status: "empty", Result: "No memories stored yet."}, nil
	}
	return &action.ActionResult{OK: true, Status: "ok", Result: strings.Join(lines, "\n")}, nil
}

// --- helpers ---

func matchesFilter(m memory.Memory, typeFilter, scopeFilter string) bool {
	if typeFilter != "" && string(m.Type) != typeFilter {
		return false
	}
	if scopeFilter != "" && string(m.Scope) != scopeFilter {
		return false
	}
	return true
}

func formatHit(m memory.Memory) string {
	scope := ""
	if m.Scope == memory.FactScopeGlobal {
		scope = " [global]"
	}
	return fmt.Sprintf("- [%s] %s — %s (%s, rev %d)%s", m.Name, m.Title, m.Description, m.Type, m.Revision, scope)
}

func clampLimit(v any) int {
	limit := defaultRecallLimit
	if f, ok := v.(float64); ok && int(f) > 0 {
		limit = int(f)
	}
	if limit > maxRecallLimit {
		limit = maxRecallLimit
	}
	return limit
}
