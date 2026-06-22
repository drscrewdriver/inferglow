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

// SystemPromptHint returns the <context-management> template that should
// be injected into the system prompt when sweet-spot or compression is active.
//
// This template informs the LLM about the compression scheme so it can
// correctly interpret compressed history markers and use context tools.
func SystemPromptHint() string {
	return `<context-management>
当前上下文采用分级压缩管理。历史消息可能包含以下标记：
- [step_N|role|Lx] 表示第 N 步、角色 role、压缩级别 Lx（L0=原文, L1=去噪, L2=事实, L3=行为掩码）
- <compaction-summary> 包裹早期对话的单级摘要（甜点区内模式）
使用 context_search 工具在压缩历史中搜索关键词。
使用 context_expand 工具展开某个被压缩的 step 回原文。
压缩感知：
- 当上下文压力较高时，应主动精简输出
- 不要尝试在输出中复制分节标记格式
- 若发现信息不足（掩码太略），先调用 context_expand 再回答
</context-management>`
}
