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

package contextmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ReorganizeDecision is the merged output of the three-in-one reorganize prompt.
type ReorganizeDecision struct {
	// Q1: constitutional entries to append
	ConstitutionalAppend []string `json:"q1_constitutional_append"`
	// Q2: new head summary (empty = no rewrite needed)
	NewHeadSummary string `json:"q2_new_head_summary"`
	// Q3: per-step level decisions
	StepDecisions []StepLevelDecision `json:"q3_step_decisions"`
}

// StepLevelDecision is a single step's target compression level.
type StepLevelDecision struct {
	StepID      int    `json:"step_id"`
	TargetLevel int    `json:"target_level"` // -1..4, -1=discard
	Reason      string `json:"reason"`
}

// ReorganizeResult is the outcome of a Reorganize call.
type ReorganizeResult struct {
	ConstitutionalAdded int
	HeadRewritten       bool
	StepsAdjusted       int
}

// CompressEngine is the interface needed by the reorganize engine for
// calling the compression model. It is satisfied by the existing
// compress.Engine type.
type CompressEngine interface {
	Call(ctx context.Context, prompt string) (string, error)
}

// Reorganize executes the three-in-one merged reorganization.
// It makes a single LLM call that answers Q1 (constitutional append),
// Q2 (head rewrite), and Q3 (step level decisions) simultaneously.
func (h *HybridManager) Reorganize(ctx context.Context, engine CompressEngine, focus string) (*ReorganizeResult, error) {
	prompt := h.buildMergedReorganizePrompt(focus)

	resp, err := engine.Call(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("reorganize call failed: %w", err)
	}

	decision, err := parseMergedResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("parse merged response: %w", err)
	}

	var result ReorganizeResult

	// Apply Q1: constitutional append
	if len(decision.ConstitutionalAppend) > 0 {
		h.AppendConstitutional(decision.ConstitutionalAppend)
		result.ConstitutionalAdded = len(decision.ConstitutionalAppend)
	}

	// Apply Q2: head rewrite
	if decision.NewHeadSummary != "" {
		h.RewriteHeadBuffer([]RenderedBlock{{
			StepID:  -1,
			Level:   0,
			Content: decision.NewHeadSummary,
		}}, fmt.Sprintf("reorg-%d", time.Now().Unix()))
		result.HeadRewritten = true
	}

	// Apply Q3: step level decisions
	if len(decision.StepDecisions) > 0 {
		count := 0
		for _, d := range decision.StepDecisions {
			ref, err := h.store.GetRef(d.StepID)
			if err != nil {
				continue
			}
			if d.TargetLevel == -1 {
				// Mark for discard (L4)
				ref.Level = 4
			} else if d.TargetLevel > ref.Level {
				ref.Level = d.TargetLevel
			}
			_ = h.store.UpsertRef(*ref)
			count++
		}
		result.StepsAdjusted = count
	}

	return &result, nil
}

// buildMergedReorganizePrompt constructs the merged three-question prompt.
func (h *HybridManager) buildMergedReorganizePrompt(focus string) string {
	var sb strings.Builder

	sb.WriteString("你是上下文重组管理器。基于以下信息，一次性回答三个决策问题。\n\n")

	// Constitutional zone
	sb.WriteString("## 当前状态\n")
	sb.WriteString("### 宪法区（Zone 0.5）\n")
	h.constitutionalMu.RLock()
	for _, e := range h.constitutionalEntries {
		sb.WriteString("- " + e + "\n")
	}
	if len(h.constitutionalEntries) == 0 {
		sb.WriteString("(空)\n")
	}
	h.constitutionalMu.RUnlock()

	// Head buffer
	sb.WriteString("\n### 头部简述（Zone 1）\n")
	h.mu.RLock()
	for _, b := range h.headBuffer {
		sb.WriteString(b.Content + "\n")
	}
	h.mu.RUnlock()

	// Step status table
	sb.WriteString("\n### Step 状态表\n")
	sb.WriteString(h.statusString())

	if focus != "" {
		sb.WriteString(fmt.Sprintf("\n### 重组焦点\n%s\n", focus))
	}

	sb.WriteString(`
## 三问决策（请按顺序回答，以 JSON 格式输出全部三个答案）

Q1: 宪法区是否需要追加？
分析：当前任务是否产生了新的操作禁止、约束或提示词更新？
行动：若有，列出追加条目；若无，返回空数组。

Q2: 头部简述是否需要转移？
分析：当前任务焦点相比现有头部简述是否已发生显著转移？
行动：若有变化，输出新头部简述；若无变化，输出空字符串。

Q3: 每个 step 的保留/压缩等级？
对每个 step，根据其与当前焦点的相关性决策目标等级：
  L0=保留原文, L1=去噪, L2=事实提取, L3=掩码, -1=丢弃

输出格式（严格 JSON）：
{
  "q1_constitutional_append": ["条目1", "条目2"],
  "q2_new_head_summary": "新头部简述（无变化则为空字符串）",
  "q3_step_decisions": [
    {"step_id": 3, "target_level": 2, "reason": "已完成且不再相关"},
    {"step_id": 7, "target_level": 0, "reason": "当前焦点依赖此步骤结果"}
  ]
}

约束：
- q1_constitutional_append 必须是字符串数组，每个条目用中文简洁描述（≤80字）
- q2_new_head_summary 不得超过 200 字
- q3_step_decisions 必须覆盖所有活跃 step_id，不得遗漏
- 仅输出 JSON，不要包裹在 markdown 代码块中`)

	return sb.String()
}

// statusString returns a human-readable summary of all active steps.
func (h *HybridManager) statusString() string {
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return "(无法获取 step 列表)\n"
	}
	var sb strings.Builder
	for _, id := range ids {
		ref, err := h.store.GetRef(id)
		if err != nil {
			continue
		}
		step, _ := h.store.GetStep(id)
		typ := "reasoning"
		tokens := 0
		if step != nil {
			typ = step.Type
			tokens = step.TokenCount
		}
		sb.WriteString(fmt.Sprintf("  step_%d | type=%s | level=L%d | tokens=%d\n", id, typ, ref.Level, tokens))
	}
	return sb.String()
}

// parseMergedResponse parses the merged three-question JSON response.
func parseMergedResponse(resp string) (*ReorganizeDecision, error) {
	// Strip markdown code fences if present
	cleaned := cleanJSONBlock(resp)

	var raw struct {
		Q1Append    []string `json:"q1_constitutional_append"`
		Q2Summary   string   `json:"q2_new_head_summary"`
		Q3Decisions []struct {
			StepID      int    `json:"step_id"`
			TargetLevel int    `json:"target_level"`
			Reason      string `json:"reason"`
		} `json:"q3_step_decisions"`
	}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	decision := &ReorganizeDecision{}

	// Q1 validation: each entry ≤80 chars
	for i, entry := range raw.Q1Append {
		if len([]rune(entry)) > 80 {
			return nil, fmt.Errorf("q1 entry %d too long: %d chars (max 80)", i, len([]rune(entry)))
		}
	}
	decision.ConstitutionalAppend = raw.Q1Append

	// Q2 validation: ≤200 chars
	if len([]rune(raw.Q2Summary)) > 200 {
		return nil, fmt.Errorf("q2 summary too long: %d chars (max 200)", len([]rune(raw.Q2Summary)))
	}
	decision.NewHeadSummary = raw.Q2Summary

	// Q3 validation
	for _, d := range raw.Q3Decisions {
		if d.TargetLevel < -1 || d.TargetLevel > 4 {
			return nil, fmt.Errorf("step %d: invalid target_level %d (range: -1..4)", d.StepID, d.TargetLevel)
		}
		if len([]rune(d.Reason)) > 60 {
			return nil, fmt.Errorf("step %d: reason too long: %d chars (max 60)", d.StepID, len([]rune(d.Reason)))
		}
		decision.StepDecisions = append(decision.StepDecisions, StepLevelDecision{
			StepID:      d.StepID,
			TargetLevel: d.TargetLevel,
			Reason:      d.Reason,
		})
	}
	return decision, nil
}

// cleanJSONBlock strips markdown code fences from a JSON response.
func cleanJSONBlock(s string) string {
	s = strings.TrimSpace(s)
	// Remove ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			// Remove first and last lines (fences)
			start := 1
			end := len(lines)
			if strings.HasPrefix(lines[end-1], "```") {
				end--
			}
			s = strings.Join(lines[start:end], "\n")
		}
	}
	return strings.TrimSpace(s)
}
