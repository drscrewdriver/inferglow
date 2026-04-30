package actionruntime

import (
	"context"
	"sync"

	"github.com/inferglow/action"
)

// ActionDispatcher executes a list of ActionCalls using the ActionRegistry.
type ActionDispatcher struct {
	registry *action.ActionRegistry
}

// NewActionDispatcher creates a dispatcher for the given registry.
func NewActionDispatcher(r *action.ActionRegistry) *ActionDispatcher {
	return &ActionDispatcher{registry: r}
}

// Execute runs all ActionCalls concurrently and returns results in order.
func (d *ActionDispatcher) Execute(ctx context.Context, calls []ActionCall) []*action.ActionResult {
	results := make([]*action.ActionResult, len(calls))
	var wg sync.WaitGroup
	wg.Add(len(calls))

	for i, call := range calls {
		go func(idx int, c ActionCall) {
			defer wg.Done()
			result, err := d.registry.Execute(ctx, c.Name, c.Params)
			if err != nil {
				results[idx] = &action.ActionResult{
					OK:     false,
					Status: "error",
					Error:  err.Error(),
				}
				return
			}
			results[idx] = result
		}(i, call)
	}

	wg.Wait()
	return results
}
