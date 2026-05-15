package policies

import (
	"github.com/inferglow/action"
	"github.com/inferglow/builtins/actions"
)

// BalancedPolicy returns an ActionRegistry populated with the read-only
// Actions from RestrictivePolicy plus file_write. file_write is a
// SideEffectWrite Action whose ActionSpec already declares
// ApprovalRequired=true, so the runtime gates it behind an approval
// flow even though it is registered. Exec Actions remain excluded.
func BalancedPolicy() *action.ActionRegistry {
	r := action.NewRegistry()
	acts := []*action.Action{
		actions.NewCalculatorAction(),
		actions.NewWebSearchAction(nil),
		actions.NewURLFetchAction(actions.URLFetchConfig{}),
		actions.NewFileReadAction(actions.FileReadConfig{}),
		actions.NewJSONProcessorAction(),
		actions.NewFileWriteAction(actions.FileWriteConfig{}),
	}
	for _, a := range acts {
		if err := r.Register(a); err != nil {
			panic(err)
		}
	}
	return r
}
