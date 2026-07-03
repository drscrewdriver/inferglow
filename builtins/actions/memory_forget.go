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

// MemoryForgetActionID is the registered Action name for archiving memories.
const MemoryForgetActionID = "forget"

// MemoryForgetConfig holds the store for the forget action.
type MemoryForgetConfig struct {
	Store memory.Store
}

// forgetExecutor is the ActionExecutor for archiving memories.
type forgetExecutor struct {
	store memory.Store
}

// NewMemoryForgetAction builds an Action that archives (soft-deletes) a
// memory by name. The file is moved to .archive/ for traceability.
func NewMemoryForgetAction(cfg MemoryForgetConfig) *action.Action {
	return &action.Action{
		Name:        MemoryForgetActionID,
		Description: "Archive a saved memory by name, removing it from the active index. The file is preserved under .archive/ for traceability. Use this when a memory is now wrong or no longer relevant. Use memory (operation=list) to find the exact name first.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "The name (kebab-case slug) of the memory to archive."},
			},
			"required": []string{"name"},
		},
		Executor: &forgetExecutor{store: cfg.Store},
		Tags:     []string{"memory", "write", "builtin"},
	}
}

// Execute archives the named memory.
func (e *forgetExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "forget: name is required"}, nil
	}

	// Verify the memory exists before archiving.
	if _, ok := e.store.Load(name); !ok {
		return &action.ActionResult{OK: false, Status: "not_found", Error: fmt.Sprintf("forget: memory %q not found", name)}, nil
	}

	if err := e.store.Archive(name); err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("forget: archive failed: %v", err)}, nil
	}

	return &action.ActionResult{
		OK:     true,
		Status: "archived",
		Result: fmt.Sprintf("Memory %q has been archived. It is no longer in the active index but preserved under .archive/.", name),
	}, nil
}
