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

	"github.com/inferglow/action"
	"github.com/inferglow/memory"
)

// MemoryRememberActionID is the registered Action name for saving memories.
const MemoryRememberActionID = "remember"

// MemoryRememberConfig holds the store for the remember action.
type MemoryRememberConfig struct {
	Store memory.Store
}

// rememberExecutor is the ActionExecutor for saving memories.
type rememberExecutor struct {
	store memory.Store
}

// NewMemoryRememberAction builds an Action that saves a durable background
// fact to the file-based memory store.
func NewMemoryRememberAction(cfg MemoryRememberConfig) *action.Action {
	return &action.Action{
		Name:        MemoryRememberActionID,
		Description: "Save a durable background fact so it survives across sessions. Use for things worth remembering long-term: who the user is and their preferences (type \"user\"); guidance on how to work, including the why (type \"feedback\"); ongoing goals or constraints not derivable from the code (type \"project\"); or pointers to external resources (type \"reference\"). Before saving, check the loaded memory index for an entry that already covers this — reuse that name to update it rather than create a near-duplicate.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "Stable kebab-case slug for the memory. Reusing an existing name updates that memory."},
				"title":       map[string]any{"type": "string", "description": "Short human-readable label shown in the memory index."},
				"description": map[string]any{"type": "string", "description": "One-line hook shown in the index — the phrase a future session reads to decide whether to open this memory."},
				"type":        map[string]any{"type": "string", "enum": []string{"user", "feedback", "project", "reference"}, "description": "Category of the fact."},
				"scope":       map[string]any{"type": "string", "enum": []string{"project", "global"}, "description": "Where the fact applies. Default: project."},
				"body":        map[string]any{"type": "string", "description": "The fact itself (Markdown). For feedback/project, include a \"**Why:**\" line and a \"**How to apply:**\" line."},
			},
			"required": []string{"description", "body"},
		},
		Executor: &rememberExecutor{store: cfg.Store},
		Tags:     []string{"memory", "write", "builtin"},
	}
}

// Execute saves or updates a memory.
func (e *rememberExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	description, _ := input["description"].(string)
	body, _ := input["body"].(string)
	if description == "" || body == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "remember: description and body are required"}, nil
	}

	name, _ := input["name"].(string)
	title, _ := input["title"].(string)
	typeStr, _ := input["type"].(string)
	scopeStr, _ := input["scope"].(string)

	m := memory.Memory{
		Name:        name,
		Title:       title,
		Description: description,
		Type:        memory.NormalizeType(typeStr),
		Scope:       memory.NormalizeFactScope(scopeStr),
		Body:        body,
	}

	path, err := e.store.Save(m)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("remember: save failed: %v", err)}, nil
	}

	// Re-load to get the assigned ID and revision.
	saved, _ := e.store.Load(m.Name)
	result := map[string]any{
		"path": path,
		"name": m.Name,
	}
	if saved.ID != "" {
		result["id"] = saved.ID
		result["revision"] = saved.Revision
	}

	return &action.ActionResult{
		OK:     true,
		Status: "saved",
		Result: result,
	}, nil
}
