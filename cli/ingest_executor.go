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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// ingestExecutor is a decorator around action.ActionExecutor that
// automatically ingests successful tool results into the memory bridge.
type ingestExecutor struct {
	inner  action.ActionExecutor
	bridge *MemoryBridge
	name   string
}

// Execute delegates to the inner executor, then ingests the result.
func (e *ingestExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	result, err := e.inner.Execute(ctx, input)
	if err == nil && result != nil && result.OK {
		e.bridge.IngestTool(e.name, formatActionResult(result))
	}
	return result, err
}

// wrapWithIngest creates a shallow copy of the action with the executor
// replaced by an ingest-wrapped version. The original action is not modified.
func wrapWithIngest(act *action.Action, bridge *MemoryBridge) *action.Action {
	return &action.Action{
		Name:        act.Name,
		Description: act.Description,
		Schema:      act.Schema,
		Executor:    &ingestExecutor{inner: act.Executor, bridge: bridge, name: act.Name},
		Tags:        act.Tags,
	}
}

// formatActionResult converts an ActionResult to a human-readable string
// for storage in the memory bridge.
func formatActionResult(r *action.ActionResult) string {
	if r == nil {
		return ""
	}
	if r.Error != "" {
		return fmt.Sprintf("[%s] error: %s", r.Status, r.Error)
	}
	return fmt.Sprintf("[%s] %v", r.Status, r.Result)
}
