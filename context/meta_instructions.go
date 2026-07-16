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

// MetaInstructionEntries returns Zone 0.5 constitutional entries that guide
// the LLM on how to use context/memory tools, when to suggest background
// updates, and how to interpret compression markers.
//
// The returned slice is compatible with HybridManager.AppendConstitutional.
func MetaInstructionEntries(cfg Config) []string {
	var entries []string
	entries = append(entries, toolUsageGuide()...)
	entries = append(entries, backgroundSelfDrive()...)
	entries = append(entries, compressionAwareness()...)
	return entries
}

// toolUsageGuide instructs the LLM on available context/memory tools.
func toolUsageGuide() []string {
	return []string{
		"[tool_guide] 使用 context_search 工具在压缩历史中按关键词搜索早期对话内容。",
		"[tool_guide] 使用 context_expand 工具将某个被压缩的 step 展开回原文（当掩码信息不足时优先调用）。",
		"[tool_guide] 使用 memory_store 工具将重要事实/决策持久化到长期记忆，以便跨会话复用。",
		"[tool_guide] 使用 context_surround 工具查看某个 step 的前后上下文。",
	}
}

// backgroundSelfDrive tells the LLM when to suggest /rebackground.
func backgroundSelfDrive() []string {
	return []string{
		"[background_drive] 若发现当前任务方向与 <constitutional> 或 head buffer 中的项目背景明显偏离，主动建议用户执行 /rebackground 更新背景。",
		"[background_drive] 当会话目标发生重大转变（如从功能开发切换到 bug 修复），提醒用户背景可能需要刷新。",
	}
}

// compressionAwareness provides compression marker interpretation rules,
// derived from SystemPromptHint() core content.
func compressionAwareness() []string {
	return []string{
		"[compress_aware] 历史消息中 [step_N|role|Lx] 标记表示第 N 步、角色 role、压缩级别 Lx（L0=原文, L1=去噪, L2=事实, L3=行为掩码）。",
		"[compress_aware] <compaction-summary> 包裹早期对话的单级摘要（甜点区内模式）。",
		"[compress_aware] 当上下文压力较高时，应主动精简输出；不要尝试复制分节标记格式。",
		"[compress_aware] 若发现信息不足（掩码太略），先调用 context_expand 再回答。",
	}
}
