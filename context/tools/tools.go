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

// Package tools implements the context management tools exposed to the LLM.
//
// Tools:
//   - context_search: semantic search across history steps (§8B.2)
//   - context_expand: expand a step to its original content (§8B.3)
//   - context_surround: view context around a step (§8B.4)
//   - memory_search: search long-term memory (§8B.8)
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/inferglow/context"
)

// Registry is the interface for tool registration.
type Registry interface {
	Register(tool Tool)
}

// Tool is the interface for context management tools.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// RegisterContextTools registers all context management tools (§8B.1).
// Tools are only registered when mode != passthrough.
func RegisterContextTools(reg Registry, mgr contextmgr.ContextManager) {
	RegisterContextToolsWithStore(reg, mgr, nil)
}

// StepStoreProvider is the interface for accessing the step store directly.
// Implemented by HybridManager when it wraps a StepStoreLike.
type StepStoreProvider interface {
	StepStore() contextmgr.StepStoreLike
}

// RegisterContextToolsWithStore registers all context tools including
// context_trace when a StepStoreLike is available.
func RegisterContextToolsWithStore(reg Registry, mgr contextmgr.ContextManager, store contextmgr.StepStoreLike) {
	if mgr.Mode() == contextmgr.ModePassthrough {
		return
	}

	reg.Register(&ContextSearchTool{mgr: mgr})
	reg.Register(&ContextExpandTool{mgr: mgr})
	reg.Register(&ContextSurroundTool{mgr: mgr, store: store})

	// memory_search only if longmem is enabled (check via stats)
	stats := mgr.Stats()
	if stats.LongMemCount >= 0 { // always register if mode != passthrough
		reg.Register(&MemorySearchTool{mgr: mgr})
	}

	// context_reorganize: only if the manager supports Reorganize (hybrid mode)
	if reorgMgr, ok := mgr.(ReorganizeProvider); ok {
		reg.Register(&ContextReorganizeTool{mgr: reorgMgr})
	}

	// context_trace: only if we have a step store
	if store != nil {
		reg.Register(&ContextTraceTool{store: store})
	} else if sp, ok := mgr.(StepStoreProvider); ok {
		reg.Register(&ContextTraceTool{store: sp.StepStore()})
	}
}

// NewContextTools returns initialized context tool instances for external
// registration (e.g. into the agent ActionExtension). Returns nil if the
// context manager is in passthrough mode.
func NewContextTools(mgr contextmgr.ContextManager, store contextmgr.StepStoreLike) []Tool {
	if mgr.Mode() == contextmgr.ModePassthrough {
		return nil
	}

	// Resolve store from manager if not provided
	if store == nil {
		if sp, ok := mgr.(StepStoreProvider); ok {
			store = sp.StepStore()
		}
	}

	toolList := []Tool{
		&ContextSearchTool{mgr: mgr},
		&ContextExpandTool{mgr: mgr},
		&ContextSurroundTool{mgr: mgr, store: store},
		&MemorySearchTool{mgr: mgr},
	}

	// context_trace requires a store
	if store != nil {
		toolList = append(toolList, &ContextTraceTool{store: store})
	}

	// context_reorganize: only if supported
	if reorgMgr, ok := mgr.(ReorganizeProvider); ok {
		toolList = append(toolList, &ContextReorganizeTool{mgr: reorgMgr})
	}

	return toolList
}

// ReorganizeProvider is the interface a ContextManager must implement
// to support the context_reorganize tool.
type ReorganizeProvider interface {
	Reorganize(ctx context.Context, engine contextmgr.CompressEngine, focus string) (*contextmgr.ReorganizeResult, error)
}

// estimatePressure estimates the current window pressure from manager stats.
func estimatePressure(mgr contextmgr.ContextManager) float64 {
	stats := mgr.Stats()
	if stats.WindowPressure > 0 {
		return stats.WindowPressure
	}
	// Fallback: estimate from tokens
	if stats.TotalTokens > 0 {
		return float64(stats.CompressedTokens) / float64(stats.TotalTokens)
	}
	return 0
}

// --- context_search (§8B.2) ---

// ContextSearchInput is the input schema for context_search.
type ContextSearchInput struct {
	Query     string `json:"query" jsonschema:"description=检索关键词或语义描述"`
	LevelMax  int    `json:"level_max,omitempty" jsonschema:"description=最高检索级别(0-3),默认3"`
	TaskGroup int    `json:"task_group,omitempty" jsonschema:"description=限定任务组"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=返回条数,默认5"`
}

// ContextSearchOutput is the output for context_search.
type ContextSearchOutput struct {
	Hits []SearchHitOutput `json:"hits"`
}

// SearchHitOutput is a single search result.
type SearchHitOutput struct {
	StepID  int     `json:"step_id"`
	Level   int     `json:"level"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
	Type    string  `json:"type"`
}

// ContextSearchTool implements context_search.
type ContextSearchTool struct {
	mgr contextmgr.ContextManager
}

func (t *ContextSearchTool) Name() string        { return "context_search" }
func (t *ContextSearchTool) Description() string  { return "检索历史 step（当 L3 掩码不够用时）" }
func (t *ContextSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"level_max":{"type":"integer"},"task_group":{"type":"integer"},"limit":{"type":"integer"}},"required":["query"]}`)
}

func (t *ContextSearchTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in ContextSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("context_search: invalid input: %w", err)
	}

	if in.Limit <= 0 {
		in.Limit = 5
	}

	hits, err := t.mgr.Search(ctx, contextmgr.SearchQuery{
		Query:     in.Query,
		LevelMax:  in.LevelMax,
		TaskGroup: in.TaskGroup,
		Limit:     in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("context_search: %w", err)
	}

	out := ContextSearchOutput{}
	for _, h := range hits {
		out.Hits = append(out.Hits, SearchHitOutput{
			StepID:  h.StepID,
			Level:   h.Level,
			Score:   h.Score,
			Snippet: h.Snippet,
			Type:    h.Type,
		})
	}

	if len(out.Hits) == 0 {
		return json.Marshal(map[string]string{"hint": "无匹配，尝试 context_expand 直接查看"})
	}
	return json.Marshal(out)
}

// --- context_expand (§8B.3) ---

// ContextExpandInput is the input schema for context_expand.
type ContextExpandInput struct {
	StepID int  `json:"step_id" jsonschema:"description=要展开的step编号"`
	Full   bool `json:"full,omitempty" jsonschema:"description=true=完整原文,false=仅L1"`
}

// ContextExpandOutput is the output for context_expand.
type ContextExpandOutput struct {
	StepID  int    `json:"step_id"`
	Level   int    `json:"current_level"`
	Content string `json:"content"`
	Tokens  int    `json:"token_count"`
	Warning string `json:"warning,omitempty"`
}

// ContextExpandTool implements context_expand.
type ContextExpandTool struct {
	mgr contextmgr.ContextManager
}

func (t *ContextExpandTool) Name() string        { return "context_expand" }
func (t *ContextExpandTool) Description() string  { return "展开某个 step 的原文（从 L0 恢复）" }
func (t *ContextExpandTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"step_id":{"type":"integer"},"full":{"type":"boolean"}},"required":["step_id"]}`)
}

func (t *ContextExpandTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in ContextExpandInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("context_expand: invalid input: %w", err)
	}

	result, err := t.mgr.Expand(in.StepID)
	if err != nil {
		return nil, fmt.Errorf("context_expand: %w", err)
	}

	// Append context_tool hint feedback (§8B.5)
	windowPressure := estimatePressure(t.mgr)
	hint := fmt.Sprintf("[context_tool] expand §%d → 原文 %d tokens | 当前窗口压力: %.0f%%", in.StepID, result.Tokens, windowPressure*100)

	return json.Marshal(map[string]interface{}{
		"step_id":       result.StepID,
		"current_level": result.Level,
		"content":       result.Content,
		"token_count":   result.Tokens,
		"warning":       result.Warning,
		"hint":          hint,
	})
}

// --- context_surround (§8B.4) ---

// ContextSurroundInput is the input schema for context_surround.
type ContextSurroundInput struct {
	StepID    int    `json:"step_id" jsonschema:"description=中心step编号"`
	Before    int    `json:"before,omitempty" jsonschema:"description=向前看N步,默认2"`
	After     int    `json:"after,omitempty" jsonschema:"description=向后看N步,默认2"`
	Causal    bool   `json:"causal,omitempty" jsonschema:"description=因果模式:沿文件依赖链追踪"`
	TaskGroup string `json:"task_group,omitempty" jsonschema:"description=按任务组聚合"`
}

// ContextSurroundOutput is the output for context_surround.
type ContextSurroundOutput struct {
	Steps []SurroundStepOutput `json:"steps"`
}

// SurroundStepOutput is a single step in the surround output.
type SurroundStepOutput struct {
	StepID   int    `json:"step_id"`
	Type     string `json:"type"`
	Level    int    `json:"level"`
	Content  string `json:"content"`
	IsCenter bool   `json:"is_center"`
}

// stepTypeRe extracts type from the ⟨§N·type·Lx⟩ marker in content.
var stepTypeRe = regexp.MustCompile(`\x{27E8}\x{00A7}\d+\x{00B7}(\w+)\x{00B7}L\d\x{27E9}`)

// ContextSurroundTool implements context_surround.
type ContextSurroundTool struct {
	mgr   contextmgr.ContextManager
	store contextmgr.StepStoreLike // optional, for causal mode
}

func (t *ContextSurroundTool) Name() string        { return "context_surround" }
func (t *ContextSurroundTool) Description() string  { return "查看某 step 前后的上下文" }
func (t *ContextSurroundTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"step_id":{"type":"integer"},"before":{"type":"integer"},"after":{"type":"integer"},"causal":{"type":"boolean"},"task_group":{"type":"string"}},"required":["step_id"]}`)
}

func (t *ContextSurroundTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in ContextSurroundInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("context_surround: invalid input: %w", err)
	}

	// Causal mode: trace file dependencies instead of contiguous window
	if in.Causal && t.store != nil {
		return t.executeCausal(ctx, in)
	}

	// TaskGroup mode: show all steps in the same task group
	if in.TaskGroup != "" && t.store != nil {
		return t.executeTaskGroup(ctx, in)
	}

	if in.Before <= 0 {
		in.Before = 2
	}
	if in.After <= 0 {
		in.After = 2
	}

	blocks, err := t.mgr.Surround(in.StepID, in.Before, in.After)
	if err != nil {
		return nil, fmt.Errorf("context_surround: %w", err)
	}

	out := ContextSurroundOutput{}
	for _, b := range blocks {
		stepType := extractStepType(b.Content)
		out.Steps = append(out.Steps, SurroundStepOutput{
			StepID:   b.StepID,
			Type:     stepType,
			Level:    b.Level,
			Content:  b.Content,
			IsCenter: b.StepID == in.StepID,
		})
	}
	return json.Marshal(out)
}

// executeCausal runs context_surround in causal mode.
func (t *ContextSurroundTool) executeCausal(_ context.Context, in ContextSurroundInput) (json.RawMessage, error) {
	chain, err := contextmgr.TraceChain(t.store, in.StepID)
	if err != nil {
		return nil, fmt.Errorf("context_surround: causal trace: %w", err)
	}

	out := ContextSurroundOutput{}
	for _, s := range chain.Steps {
		out.Steps = append(out.Steps, SurroundStepOutput{
			StepID:   s.StepID,
			Type:     s.Type,
			Level:    0,
			Content:  truncateContent(s.Content, 500),
			IsCenter: s.StepID == in.StepID,
		})
	}
	if len(out.Steps) == 0 {
		return json.Marshal(map[string]string{"hint": "因果链为空，该步骤无文件依赖"})
	}
	return json.Marshal(out)
}

// executeTaskGroup runs context_surround in task group mode.
func (t *ContextSurroundTool) executeTaskGroup(_ context.Context, in ContextSurroundInput) (json.RawMessage, error) {
	steps, err := contextmgr.TraceTaskGroup(t.store, in.TaskGroup)
	if err != nil {
		return nil, fmt.Errorf("context_surround: trace task group: %w", err)
	}

	out := ContextSurroundOutput{}
	for _, s := range steps {
		out.Steps = append(out.Steps, SurroundStepOutput{
			StepID:  s.StepID,
			Type:    s.Type,
			Level:   0,
			Content: truncateContent(s.Content, 500),
		})
	}
	if len(out.Steps) == 0 {
		return json.Marshal(map[string]string{"hint": fmt.Sprintf("任务组 '%s' 无步骤", in.TaskGroup)})
	}
	return json.Marshal(out)
}

// truncateContent limits content to maxLen chars.
func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// extractStepType parses the type from a ⟨§N·type·Lx⟩ marker.
func extractStepType(content string) string {
	m := stepTypeRe.FindStringSubmatch(content)
	if len(m) >= 2 {
		return m[1]
	}
	return "unknown"
}

// --- memory_search (§8B.8) ---

// MemorySearchInput is the input schema for memory_search.
type MemorySearchInput struct {
	Query    string `json:"query" jsonschema:"description=检索关键词或语义描述"`
	Category string `json:"category,omitempty" jsonschema:"description=限定类别: config/decision/constraint/pattern"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=返回条数,默认5"`
}

// MemorySearchOutput is the output for memory_search.
type MemorySearchOutput struct {
	Hits []MemoryHitOutput `json:"hits"`
}

// MemoryHitOutput is a single long-term memory result.
type MemoryHitOutput struct {
	MemID      string   `json:"mem_id"`
	Facts      []string `json:"facts"`
	Category   string   `json:"category"`
	Confidence float64  `json:"confidence"`
	Sources    []int    `json:"source_steps"`
	Sessions   []string `json:"source_sessions"`
}

// MemorySearchTool implements memory_search.
type MemorySearchTool struct {
	mgr contextmgr.ContextManager
}

func (t *MemorySearchTool) Name() string        { return "memory_search" }
func (t *MemorySearchTool) Description() string  { return "检索跨 session 长期记忆（配置值/决策/约束等持久知识）" }
func (t *MemorySearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"category":{"type":"string"},"limit":{"type":"integer"}},"required":["query"]}`)
}

func (t *MemorySearchTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in MemorySearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("memory_search: invalid input: %w", err)
	}

	if in.Limit <= 0 {
		in.Limit = 5
	}

	records, err := t.mgr.SearchLongMem(ctx, in.Query, in.Category, in.Limit)
	if err != nil {
		return nil, fmt.Errorf("memory_search: %w", err)
	}

	out := MemorySearchOutput{}
	for _, r := range records {
		// Filter by confidence ≥ 0.5 (§8B.8)
		if r.Confidence < 0.5 {
			continue
		}
		out.Hits = append(out.Hits, MemoryHitOutput{
			MemID:      r.MemID,
			Facts:      r.Facts,
			Category:   r.Category,
			Confidence: r.Confidence,
			Sources:    r.SourceSteps,
			Sessions:   r.SourceSessions,
		})
	}

	if len(out.Hits) == 0 {
		return json.Marshal(map[string]string{"hint": "无匹配长期记忆，尝试 context_search 检索当前 session"})
	}
	return json.Marshal(out)
}

// --- context_reorganize (§0.2.6) ---

// ContextReorganizeInput is the input schema for context_reorganize.
type ContextReorganizeInput struct {
	Focus      string `json:"focus,omitempty" jsonschema:"description=重组焦点（如 'redis config migration'）"`
	Aggressive bool   `json:"aggressive,omitempty" jsonschema:"description=true=更激进压缩"`
}

// ContextReorganizeTool implements context_reorganize.
type ContextReorganizeTool struct {
	mgr ReorganizeProvider
}

func (t *ContextReorganizeTool) Name() string        { return "context_reorganize" }
func (t *ContextReorganizeTool) Description() string  { return "当上下文增长过大或任务焦点显著转移时，重组上下文（三问合一：宪法区追加+头部改写+step级别调整）" }
func (t *ContextReorganizeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"focus":{"type":"string"},"aggressive":{"type":"boolean"}}}`)
}

func (t *ContextReorganizeTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in ContextReorganizeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("context_reorganize: invalid input: %w", err)
	}

	// Note: engine is nil here — in production, the tool should receive
	// the compress engine via constructor injection. For now, return a
	// placeholder that indicates the reorganize was requested.
	result, err := t.mgr.Reorganize(ctx, nil, in.Focus)
	if err != nil {
		return nil, fmt.Errorf("context_reorganize: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"constitutional_added": result.ConstitutionalAdded,
		"head_rewritten":       result.HeadRewritten,
		"steps_adjusted":       result.StepsAdjusted,
		"hint":                 fmt.Sprintf("重组完成：宪法+%d, 头部改写=%v, step调整=%d", result.ConstitutionalAdded, result.HeadRewritten, result.StepsAdjusted),
	})
}

// --- context_trace (B3) ---

// ContextTraceInput is the input schema for context_trace.
type ContextTraceInput struct {
	File      string `json:"file,omitempty" jsonschema:"description=追踪涉及该文件的所有步骤"`
	StepID    int    `json:"step_id,omitempty" jsonschema:"description=从该步骤出发追踪因果链"`
	TaskGroup string `json:"task_group,omitempty" jsonschema:"description=列出同组所有步骤"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=返回条数,默认10"`
}

// ContextTraceOutput is the output for context_trace.
type ContextTraceOutput struct {
	Steps []TraceStepOutput `json:"steps"`
	Files []string          `json:"files,omitempty"`
	Hint  string            `json:"hint,omitempty"`
}

// TraceStepOutput is a single step in the trace output.
type TraceStepOutput struct {
	StepID        int      `json:"step_id"`
	Type          string   `json:"type"`
	ToolName      string   `json:"tool_name,omitempty"`
	FilesRead     []string `json:"files_read,omitempty"`
	FilesModified []string `json:"files_modified,omitempty"`
	TaskGroup     string   `json:"task_group,omitempty"`
	Snippet       string   `json:"snippet"`
}

// ContextTraceTool implements context_trace.
type ContextTraceTool struct {
	store contextmgr.StepStoreLike
}

func (t *ContextTraceTool) Name() string        { return "context_trace" }
func (t *ContextTraceTool) Description() string  { return "沿文件依赖链追踪因果关系" }
func (t *ContextTraceTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"file":{"type":"string"},"step_id":{"type":"integer"},"task_group":{"type":"string"},"limit":{"type":"integer"}}}`)
}

func (t *ContextTraceTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in ContextTraceInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("context_trace: invalid input: %w", err)
	}

	if in.Limit <= 0 {
		in.Limit = 10
	}

	out := ContextTraceOutput{}

	// Mode 1: trace by file
	if in.File != "" {
		steps, err := contextmgr.TraceFiles(t.store, []string{in.File}, in.Limit)
		if err != nil {
			return nil, fmt.Errorf("context_trace: %w", err)
		}
		for _, s := range steps {
			out.Steps = append(out.Steps, stepToTraceOutput(s))
		}
		out.Files = []string{in.File}
		out.Hint = fmt.Sprintf("涉及文件 '%s' 的步骤共 %d 个", in.File, len(steps))
	}

	// Mode 2: trace causal chain from step
	if in.StepID > 0 {
		chain, err := contextmgr.TraceChain(t.store, in.StepID)
		if err != nil {
			return nil, fmt.Errorf("context_trace: %w", err)
		}
		for _, s := range chain.Steps {
			if len(out.Steps) >= in.Limit {
				break
			}
			out.Steps = append(out.Steps, stepToTraceOutput(s))
		}
		out.Files = chain.Files
		out.Hint = fmt.Sprintf("从 step %d 追踪因果链，涉及 %d 个文件", in.StepID, len(chain.Files))
	}

	// Mode 3: trace by task group
	if in.TaskGroup != "" {
		steps, err := contextmgr.TraceTaskGroup(t.store, in.TaskGroup)
		if err != nil {
			return nil, fmt.Errorf("context_trace: %w", err)
		}
		for _, s := range steps {
			if len(out.Steps) >= in.Limit {
				break
			}
			out.Steps = append(out.Steps, stepToTraceOutput(s))
		}
		out.Hint = fmt.Sprintf("任务组 '%s' 共 %d 个步骤", in.TaskGroup, len(steps))
	}

	if len(out.Steps) == 0 {
		return json.Marshal(map[string]string{"hint": "无匹配，请指定 file/step_id/task_group"})
	}
	return json.Marshal(out)
}

// stepToTraceOutput converts a StepRecord to trace output format.
func stepToTraceOutput(s contextmgr.StepRecord) TraceStepOutput {
	return TraceStepOutput{
		StepID:        s.StepID,
		Type:          s.Type,
		ToolName:      s.ToolName,
		FilesRead:     s.FilesRead,
		FilesModified: s.FilesModified,
		TaskGroup:     s.TaskGroup,
		Snippet:       truncateContent(s.Content, 200),
	}
}
