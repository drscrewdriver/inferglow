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
	"path/filepath"

	"github.com/inferglow/builtins/actions"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/session"
)

// buildAgent assembles a fully wired Agent with memory-bridged actions.
func buildAgent(cfg CLIConfig, bridge *MemoryBridge, sessionID string) (*agent.Agent, error) {
	// 1. Session.
	sess := session.NewSession(sessionID, cfg.WindowTokens)

	// 2. Model.
	modelReq, err := buildModelRequester(cfg)
	if err != nil {
		return nil, fmt.Errorf("build model: %w", err)
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

	// 4. Callbacks for auto-ingest of assistant responses.
	callbacks := &agent.AgentCallbacks{
		OnRunEnd: func(ctx context.Context, response string, err error) {
			if err == nil && response != "" {
				bridge.IngestAssistant(response)
			}
		},
	}

	// 5. Construct agent.
	ag := agent.New(sess, actExt, modelReq, agent.WithCallbacks(callbacks))
	return ag, nil
}
