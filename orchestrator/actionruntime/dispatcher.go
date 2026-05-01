package actionruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
)

// ActionDispatcher executes a list of ActionCalls using the ActionRegistry.
type ActionDispatcher struct {
	registry  *action.ActionRegistry
	auditHook audit.AuditHook
}

// NewActionDispatcher creates a dispatcher for the given registry. It is
// equivalent to NewActionDispatcherWithAudit with a NoOpHook so existing
// callers keep their pre-audit behavior and zero overhead.
func NewActionDispatcher(r *action.ActionRegistry) *ActionDispatcher {
	return NewActionDispatcherWithAudit(r, &audit.NoOpHook{})
}

// NewActionDispatcherWithAudit creates a dispatcher that appends an audit
// entry to hook after every registry.Execute call (success or failure). A
// nil hook disables auditing entirely; pass &audit.NoOpHook{} for the
// zero-overhead default.
func NewActionDispatcherWithAudit(r *action.ActionRegistry, hook audit.AuditHook) *ActionDispatcher {
	return &ActionDispatcher{registry: r, auditHook: hook}
}

// Execute runs all ActionCalls concurrently and returns results in order.
// Each call's audit entry is appended regardless of whether registry.Execute
// succeeded or errored; audit Append return values are intentionally ignored
// so an audit failure cannot break action execution.
func (d *ActionDispatcher) Execute(ctx context.Context, calls []ActionCall) []*action.ActionResult {
	results := make([]*action.ActionResult, len(calls))
	var wg sync.WaitGroup
	wg.Add(len(calls))

	for i, call := range calls {
		go func(idx int, c ActionCall) {
			defer wg.Done()
			start := time.Now()

			// recover from executor panics: without this, results[idx]
			// stays nil and upstream consumers hit a nil-pointer panic.
			// On panic we synthesize a "panic" ActionResult and append an
			// audit entry so the failure is observable.
			defer func() {
				if r := recover(); r != nil {
					results[idx] = &action.ActionResult{
						OK:     false,
						Status: "panic",
						Error:  fmt.Sprintf("panic: %v", r),
					}
					if d.auditHook != nil {
						entry := &audit.AuditEntry{
							Timestamp: start,
							Source:    "action",
							Action:    "execute",
							Input:     c,
							Output:    results[idx],
							Duration:  time.Since(start),
							Metadata:  map[string]string{"action_name": c.Name},
							Error:     fmt.Sprintf("panic: %v", r),
						}
						_, _ = d.auditHook.Append(entry)
					}
				}
			}()

			result, err := d.registry.Execute(ctx, c.Name, c.Params)
			duration := time.Since(start)
			if err != nil {
				results[idx] = &action.ActionResult{
					OK:     false,
					Status: "error",
					Error:  err.Error(),
				}
			} else {
				results[idx] = result
			}
			if d.auditHook != nil {
				entry := &audit.AuditEntry{
					Timestamp: start,
					Source:    "action",
					Action:    "execute",
					Input:     c,
					Output:    results[idx],
					Duration:  duration,
					Metadata:  map[string]string{"action_name": c.Name},
				}
				if err != nil {
					entry.Error = err.Error()
				}
				// Intentionally ignore return values: audit failures must
				// not break action execution.
				_, _ = d.auditHook.Append(entry)
			}
		}(i, call)
	}

	wg.Wait()
	return results
}
