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

package compress

import "fmt"

// BuildPrompt constructs the compression prompt for a given level (§4.1-4.3).
func BuildPrompt(level int, stepID int, toolName, keyParams, content string) string {
	switch level {
	case 1:
		return buildL1Prompt(content)
	case 2:
		return buildL2Prompt(content)
	case 3:
		return buildL3Prompt(stepID, toolName, keyParams, content)
	default:
		return content
	}
}

// buildL1Prompt builds the L1 simple compression prompt (§4.1).
func buildL1Prompt(content string) string {
	return `[System]
你是一个文本压缩器。对输入内容执行"去噪瘦身"：
- 删除重复的日志行（保留首次出现）
- 删除纯空白行、分隔线、装饰性格式
- 删除过渡性废话（"让我来看看"、"好的，现在"、"以上就是"等）
- 保留所有代码块、命令输出、路径、配置值的原始格式
- 不改变任何事实性内容的措辞

输出压缩后的文本，不要添加任何解释。

[User]
` + content
}

// buildL2Prompt builds the L2 fact extraction prompt (§4.2).
func buildL2Prompt(content string) string {
	return `[System]
你是一个事实提取器。从输入内容中提取关键事实：
- 保留：文件路径、配置键值对、错误消息原文、命令及其关键输出、决策结论
- 丢弃：推理过程、尝试性探索、中间计算、已被后续结论覆盖的假设
- 格式：每行一个事实，前缀 "[事实] "
- 若内容为工具调用结果，保留 [call] 摘要（工具名+关键参数）+ [result] 关键行

输出提取的事实列表，不要添加解释。

[User]
` + content
}

// buildL3Prompt builds the L3 mask generation prompt (§4.3).
func buildL3Prompt(stepID int, toolName, keyParams, content string) string {
	tokenCount := len(content) / 4 // rough estimate
	return fmt.Sprintf(`[System]
你是一个掩码生成器。为以下操作记录生成一行掩码总结：
- 格式：[掩码 step_%d|原%dt|%s|%s] {一句话意图}
- 意图总结不超过 20 字
- key_params 提取最关键的 1-2 个参数（如 pattern、path、command）

只输出一行掩码，不要添加解释。

[User]
step_id: %d
token_count: %d
tool_name: %s
content:
%s`, stepID, tokenCount, toolName, keyParams, stepID, tokenCount, toolName, content)
}
