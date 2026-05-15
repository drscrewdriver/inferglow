package policies

import (
	"github.com/inferglow/action"
	"github.com/inferglow/builtins/actions"
)

// PermissivePolicy returns an ActionRegistry populated with every
// built-in Action. The high-risk exec Actions (code_executor,
// bash_executor) are registered, but their ActionSpecs already declare
// ApprovalRequired=true and SandboxRequired=true, so the runtime still
// applies approval and sandboxing gates before dispatching them.
func PermissivePolicy() *action.ActionRegistry {
	r := action.NewRegistry()
	acts := []*action.Action{
		actions.NewCalculatorAction(),
		actions.NewWebSearchAction(nil),
		actions.NewURLFetchAction(actions.URLFetchConfig{}),
		actions.NewFileReadAction(actions.FileReadConfig{}),
		actions.NewJSONProcessorAction(),
		actions.NewFileWriteAction(actions.FileWriteConfig{}),
		actions.NewCodeExecutorAction(nil),
		actions.NewBashExecutorAction(nil),
	}
	for _, a := range acts {
		if err := r.Register(a); err != nil {
			panic(err)
		}
	}
	return r
}
