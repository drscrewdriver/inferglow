package policies

import (
	"github.com/inferglow/action"
	"github.com/inferglow/builtins/actions"
)

// RestrictivePolicy returns an ActionRegistry populated only with Actions
// whose SideEffectLevel is "none" or "read". Write and exec Actions are
// excluded entirely so the registry is safe to expose in untrusted
// contexts.
func RestrictivePolicy() *action.ActionRegistry {
	r := action.NewRegistry()
	acts := []*action.Action{
		actions.NewCalculatorAction(),
		actions.NewWebSearchAction(nil),
		actions.NewURLFetchAction(actions.URLFetchConfig{}),
		actions.NewFileReadAction(actions.FileReadConfig{}),
		actions.NewJSONProcessorAction(),
	}
	for _, a := range acts {
		if err := r.Register(a); err != nil {
			panic(err)
		}
	}
	return r
}
