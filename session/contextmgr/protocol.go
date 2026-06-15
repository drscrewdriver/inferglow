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

import "fmt"

// ContextProtocolTemplate is the CONTEXT_PROTOCOL constitutional sub-zone (§4B.2).
// It is injected into head_buffer once at session init and never changes (prefix cache stable).
const ContextProtocolTemplate = `[CONTEXT_PROTOCOL]
本对话采用分级上下文管理。窗口大小: %d tokens。
你看到的历史内容已经过压缩处理。

分节标记：
- 每个历史片段以 ⟨§N·type·level⟩ 开头，N 为 step 编号
- L0=原文, L1=精简, L2=事实, L3=掩码
- L3 掩码仅描述"做了什么"，具体内容不可见

引用规则：
- 当你需要引用历史 step 的内容时，使用 §N 格式（如 "根据 §3 的结果"）
- 引用会自动触发该 step 的活跃度追踪，影响后续压缩策略
- 不要编造不存在的 §N 编号

可用工具：
- context_search(query): 检索历史 step（当 L3 掩码不够用时）
- context_expand(step_id): 展开某个 step 的原文（从 L0 恢复）
- context_surround(step_id): 查看某 step 前后的上下文
- memory_search(query): 检索跨 session 长期记忆（配置值/决策/约束等持久知识）

压缩感知：
- 当你看到 [hint] 中的上下文压力超过 80%%，应主动精简输出
- 不要尝试在输出中复制分节标记格式（⟨§...⟩）
- 若发现信息不足（掩码太略），先调用 context_expand 再回答
[/CONTEXT_PROTOCOL]`

// BuildContextProtocol generates the CONTEXT_PROTOCOL block with the given window size.
func BuildContextProtocol(windowTokens int) string {
	return fmt.Sprintf(ContextProtocolTemplate, windowTokens)
}

// HintBlockTemplate is the dynamic hint injected as zone 5 each step (§4B.3).
const HintBlockTemplate = `[hint] pressure:%.0f%% | task_group:#%d | active_facts:%d | tail:%dsteps`

// BuildHintBlock generates the dynamic hint block content.
func BuildHintBlock(pressure float64, taskGroupID, activeFacts, tailSteps int) string {
	return fmt.Sprintf(HintBlockTemplate, pressure*100, taskGroupID, activeFacts, tailSteps)
}
