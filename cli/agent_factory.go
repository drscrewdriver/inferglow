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
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
	"github.com/inferglow/builtins/actions"
	contextmgr "github.com/inferglow/context"
	"github.com/inferglow/context/tools"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/session"
)

// buildAgent assembles a fully wired Agent with memory-bridged actions.
// When ac is non-nil (and enabled), the agent engine uses
// NewEngineWithAudit so decision audit entries flow through the chain.
// Returns the agent and the audit chain (or nil if audit is disabled).
func buildAgent(cfg CLIConfig, bridge *MemoryBridge, sessionID string, ac *audit.AuditChain) (*agent.Agent, *audit.AuditChain, error) {
	// 1. Session.
	sess := session.NewSession(sessionID, cfg.WindowTokens)

	// 2. Model.
	modelReq, err := buildModelRequester(cfg)
	if err != nil {
		return nil, ac, fmt.Errorf("build model: %w", err)
	}

	// 3. Action extension — register builtins with ingest wrapping.
	actExt := agent.NewActionExtension()

	absWorkspace, _ := filepath.Abs(cfg.WorkspaceDir)
	allowedDirs := []string{absWorkspace}

	// file_read
	fileRead := actions.NewFileReadAction(actions.FileReadConfig{AllowedDirs: allowedDirs})
	actExt.Register(wrapWithIngest(fileRead, bridge))

	// file_write
	fileWrite := actions.NewFileWriteAction(actions.FileWriteConfig{AllowedDirs: allowedDirs})
	actExt.Register(wrapWithIngest(fileWrite, bridge))

	// list_dir
	listDir := actions.NewListDirAction(actions.ListDirConfig{AllowedDirs: allowedDirs})
	actExt.Register(wrapWithIngest(listDir, bridge))

	// bash_executor — uses local bash runner.
	bashRunner := &localBashRunner{workdir: absWorkspace, unsafe: cfg.UnsafeMode}
	bashAct := actions.NewBashExecutorAction(bashRunner)
	actExt.Register(wrapWithIngest(bashAct, bridge))

	// grep — uses local grep runner.
	grepRunner := &localGrepRunner{workdir: absWorkspace}
	grepAct := actions.NewGrepAction(grepRunner)
	actExt.Register(wrapWithIngest(grepAct, bridge))

	// url_fetch
	urlFetch := actions.NewURLFetchAction(actions.URLFetchConfig{})
	actExt.Register(wrapWithIngest(urlFetch, bridge))

	// Memory tools (Phase 2): remember / memory / forget
	memStore := bridge.MemStore()
	remember := actions.NewMemoryRememberAction(actions.MemoryRememberConfig{Store: memStore})
	actExt.Register(wrapWithIngest(remember, bridge))

	recall := actions.NewMemoryRecallAction(actions.MemoryRecallConfig{Store: memStore})
	actExt.Register(recall) // read-only, no ingest wrapping

	forget := actions.NewMemoryForgetAction(actions.MemoryForgetConfig{Store: memStore})
	actExt.Register(wrapWithIngest(forget, bridge))

	// Sub-agent tool (Phase 3): spawn_agent
	subAgent := actions.NewSubAgentAction(actions.SubAgentConfig{MaxDepth: 3, MaxRounds: 15})
	actExt.Register(wrapWithIngest(subAgent, bridge))

	// Skill tool: run_skill (procedural memory)
	runSkill := actions.NewRunSkillAction(actions.RunSkillConfig{
		Store:     bridge.SkillStore(),
		MaxRounds: 15,
	})
	actExt.Register(wrapWithIngest(runSkill, bridge))

	// Task tracker tools (T1-T4)
	ts := bridge.TaskStore()
	taskCfg := actions.TaskTrackerConfig{Store: ts}
	taskAdd := actions.NewTaskAddAction(taskCfg)
	actExt.Register(wrapWithIngest(taskAdd, bridge))
	taskUpdate := actions.NewTaskUpdateAction(taskCfg)
	actExt.Register(wrapWithIngest(taskUpdate, bridge))
	taskList := actions.NewTaskListAction(taskCfg)
	actExt.Register(taskList) // read-only, no ingest wrapping
	taskDelete := actions.NewTaskDeleteAction(taskCfg)
	actExt.Register(wrapWithIngest(taskDelete, bridge))

	// Context tools: bridge context tools to agent tool system so LLM can see them.
	registerContextTools(actExt, bridge)

	// 4. Callbacks for auto-ingest of assistant responses.
	callbacks := &agent.AgentCallbacks{
		OnRunEnd: func(ctx context.Context, response string, err error) {
			if err == nil && response != "" {
				bridge.IngestAssistant(response)
			}
		},
	}

	// 5. Construct agent.
	var agentOpts []agent.RunOption
	agentOpts = append(agentOpts, agent.WithCallbacks(callbacks))
	if ac != nil {
		agentOpts = append(agentOpts, agent.WithAuditHook(ac))
	}
	ag := agent.New(sess, actExt, modelReq, agentOpts...)
	return ag, ac, nil
}

// registerContextTools bridges context tools (context_search, context_expand,
// context_surround, memory_search, context_trace) to the agent tool system
// so the LLM can see and invoke them.
func registerContextTools(actExt *agent.ActionExtension, bridge *MemoryBridge) {
	mgr := bridge.ContextManager()
	if mgr == nil || mgr.Mode() == contextmgr.ModePassthrough {
		return
	}

	// Create initialized context tools via factory function
	ctxTools := tools.NewContextTools(mgr, nil)

	// Register each tool as an action
	for _, t := range ctxTools {
		act := wrapContextToolAsAction(t, bridge)
		_ = actExt.Register(act)
	}
}

// contextToolAdapter adapts a context tools.Tool to action.ActionExecutor.
type contextToolAdapter struct {
	tool   tools.Tool
	bridge *MemoryBridge
}

func (a *contextToolAdapter) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	// Convert map input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("context tool: marshal input: %v", err),
		}, nil
	}

	// Execute the context tool
	outputJSON, err := a.tool.Execute(ctx, inputJSON)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  err.Error(),
		}, nil
	}

	return &action.ActionResult{
		OK:     true,
		Status: "ok",
		Result: string(outputJSON),
	}, nil
}

// wrapContextToolAsAction wraps a context Tool as an *action.Action.
func wrapContextToolAsAction(t tools.Tool, bridge *MemoryBridge) *action.Action {
	schema := t.InputSchema()
	var schemaMap map[string]any
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		schemaMap = map[string]any{"type": "object", "properties": map[string]any{}}
	}

	return &action.Action{
		Name:        t.Name(),
		Description: t.Description(),
		Schema:      schemaMap,
		Executor:    &contextToolAdapter{tool: t, bridge: bridge},
		Tags:        []string{"context", "builtin"},
	}
}
