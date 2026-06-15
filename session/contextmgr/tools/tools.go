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

	"github.com/inferglow/session/contextmgr"
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
	if mgr.Mode() == contextmgr.ModePassthrough {
		return
	}

	reg.Register(&ContextSearchTool{mgr: mgr})
	reg.Register(&ContextExpandTool{mgr: mgr})
	reg.Register(&ContextSurroundTool{mgr: mgr})

	// memory_search only if longmem is enabled (check via stats)
	stats := mgr.Stats()
	if stats.LongMemCount >= 0 { // always register if mode != passthrough
		reg.Register(&MemorySearchTool{mgr: mgr})
	}
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
	StepID int `json:"step_id" jsonschema:"description=中心step编号"`
	Before int `json:"before,omitempty" jsonschema:"description=向前看N步,默认2"`
	After  int `json:"after,omitempty" jsonschema:"description=向后看N步,默认2"`
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
var stepTypeRe = regexp.MustCompile(`\u27E8\u00A7\d+\u00B7(\w+)\u00B7L\d\u27E9`)

// ContextSurroundTool implements context_surround.
type ContextSurroundTool struct {
	mgr contextmgr.ContextManager
}

func (t *ContextSurroundTool) Name() string        { return "context_surround" }
func (t *ContextSurroundTool) Description() string  { return "查看某 step 前后的上下文" }
func (t *ContextSurroundTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"step_id":{"type":"integer"},"before":{"type":"integer"},"after":{"type":"integer"}},"required":["step_id"]}`)
}

func (t *ContextSurroundTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in ContextSurroundInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("context_surround: invalid input: %w", err)
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
