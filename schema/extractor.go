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

package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ExtractJSON 从文本中提取 JSON，支持三级策略
func ExtractJSON(text string, schemas ...*OutputSchema) (map[string]any, error) {
	// 策略1：直接提取根 JSON
	result, err := extractDirect(text)
	if err == nil {
		return result, nil
	}

	// 策略2：提取所有候选 JSON 块
	candidates := extractCandidates(text)
	if len(candidates) > 0 {
		if len(schemas) > 0 {
			// 策略3：schema 匹配评分
			return scoreAndSelect(candidates, schemas[0])
		}
		// 返回第一个有效的候选
		for _, c := range candidates {
			result, err := parseJSON(c)
			if err == nil {
				return result, nil
			}
		}
	}

	// 策略4：尝试修复截断 JSON（适用于 LLM 输出被截断的场景）
	if repaired, ok := tryRepairTruncatedFromText(text); ok {
		if result, err := parseJSON(repaired); err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("no valid JSON found in text")
}

// tryRepairTruncatedFromText 从文本中找到首个 JSON 起始字符（{ 或 [），
// 从该位置开始用 RepairTruncatedJSON 修复，并验证修复结果可被解析。
func tryRepairTruncatedFromText(text string) (string, bool) {
	startIdx := -1
	for i, c := range text {
		if c == '{' || c == '[' {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return "", false
	}
	repaired := RepairTruncatedJSON(text[startIdx:])
	// 验证修复结果确实是合法 JSON
	var tmp any
	if err := json.Unmarshal([]byte(repaired), &tmp); err != nil {
		return "", false
	}
	return repaired, true
}

// extractDirect 尝试直接解析整段文本为 JSON
func extractDirect(text string) (map[string]any, error) {
	trimmed := strings.TrimSpace(text)
	// 去除可能的 markdown 代码块
	trimmed = stripCodeBlock(trimmed)

	if len(trimmed) > 0 && trimmed[0] == '{' {
		result, err := parseJSON(trimmed)
		if err == nil {
			return result, nil
		}
		// 尝试修复截断 JSON 后再解析
		repaired := RepairTruncatedJSON(trimmed)
		if repaired != trimmed {
			if result, err := parseJSON(repaired); err == nil {
				return result, nil
			}
		}
		return nil, fmt.Errorf("text does not start with JSON")
	}
	return nil, fmt.Errorf("text does not start with JSON")
}

// stripCodeBlock 去除 markdown 代码块标记
func stripCodeBlock(text string) string {
	prefix := "\x60\x60\x60(?:json)?\\s*\\n"
	re := regexp.MustCompile(prefix)
	result := re.ReplaceAllString(text, "")
	suffix := "\\n\x60\x60\x60"
	result = strings.TrimRight(result, suffix)
	return strings.TrimSpace(result)
}

// extractCandidates 提取所有候选 JSON 块
func extractCandidates(text string) []string {
	// 查找所有可能的 JSON 对象
	re := regexp.MustCompile(`\{[^{}]*\}`)
	return re.FindAllString(text, -1)
}

// scoreAndSelect 使用 schema 评分选择最佳候选
func scoreAndSelect(candidates []string, schema *OutputSchema) (map[string]any, error) {
	var bestResult map[string]any
	var bestScore int

	fieldNames := make([]string, 0, len(schema.Fields))
	for name := range schema.Fields {
		fieldNames = append(fieldNames, name)
	}

	for _, candidate := range candidates {
		result, err := parseJSON(candidate)
		if err != nil {
			continue
		}

		score := scoreResult(result, fieldNames)
		if score > bestScore {
			bestScore = score
			bestResult = result
		}
	}

	if bestResult == nil {
		return nil, fmt.Errorf("no candidate matched schema")
	}

	return bestResult, nil
}

// scoreResult 计算结果与 schema 的匹配分数
func scoreResult(result map[string]any, fieldNames []string) int {
	score := 0
	for _, name := range fieldNames {
		if _, ok := result[name]; ok {
			score++
		}
	}
	return score
}

// parseJSON 解析 JSON 字符串
func parseJSON(text string) (map[string]any, error) {
	var result map[string]any
	err := json.Unmarshal([]byte(text), &result)
	return result, err
}

// RepairJSONFragment 尝试修复常见的 JSON 错误
func RepairJSONFragment(text string) string {
	// 修复全角括号
	text = strings.ReplaceAll(text, "\uff5b", "{")
	text = strings.ReplaceAll(text, "\uff5d", "}")
	text = strings.ReplaceAll(text, "\uff3b", "[")
	text = strings.ReplaceAll(text, "\uff3d", "]")

	// 修复中文标点
	text = strings.ReplaceAll(text, "\uff0c", ",")
	text = strings.ReplaceAll(text, "\uff1a", ":")

	// 修复缺失的引号（简单情况：属性名缺少引号）
	re := regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)
	text = re.ReplaceAllString(text, `$1"$2":`)

	// 修复缺失的尾部逗号（简单情况）：在值后面缺少逗号
	// 只匹配值后的空白 + 另一个值的开头，但不匹配已经有关联的逗号
	re = regexp.MustCompile(`(["'}\]])(\s+)(["{[\{])`)
	text = re.ReplaceAllString(text, `$1,$2$3`)

	return text
}

// RepairTruncatedJSON 修复因截断而不完整的 JSON 文本。
// 处理以下场景：
//   - 未闭合的字符串（添加结尾 "）
//   - 未闭合的数组/对象（按嵌套顺序添加 ] 或 }）
//   - 末尾多余逗号（在闭合括号前删除）
//
// 输入若已是合法 JSON 则原样返回。
func RepairTruncatedJSON(text string) string {
	if text == "" {
		return text
	}

	// 先用 json.Unmarshal 探测是否已合法
	var tmp any
	if err := json.Unmarshal([]byte(text), &tmp); err == nil {
		return text
	}

	// 找到第一个 JSON 起始位置（{ 或 [）
	startIdx := -1
	for i, c := range text {
		if c == '{' || c == '[' {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return text
	}

	// 用栈跟踪未闭合的容器；同时跟踪是否在字符串内
	type frame struct {
		open  byte // '{' or '['
		index int  // position in text where the open char appears
	}
	var stack []frame
	inString := false
	escaped := false

	for i := startIdx; i < len(text); i++ {
		c := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, frame{open: c, index: i})
		case '}', ']':
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				if (c == '}' && top.open == '{') || (c == ']' && top.open == '[') {
					stack = stack[:len(stack)-1]
				}
			}
		}
	}

	// 截取从第一个 JSON 字符到末尾
	endIdx := len(text)
	suffix := strings.Builder{}

	// 若字符串未闭合，先闭合字符串
	if inString {
		suffix.WriteByte('"')
	}

	// 移除末尾多余的逗号（仅看 text 末尾，因为 suffix 目前只是 "）
	trimmed := strings.TrimRight(text[startIdx:endIdx], " \t\r\n")
	// 移除末尾的逗号
	for len(trimmed) > 0 && (trimmed[len(trimmed)-1] == ',' || trimmed[len(trimmed)-1] == ' ' || trimmed[len(trimmed)-1] == '\t' || trimmed[len(trimmed)-1] == '\r' || trimmed[len(trimmed)-1] == '\n') {
		if trimmed[len(trimmed)-1] == ',' {
			trimmed = trimmed[:len(trimmed)-1]
			break
		}
		trimmed = trimmed[:len(trimmed)-1]
	}

	// 倒序闭合栈中的容器
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i].open {
		case '{':
			suffix.WriteByte('}')
		case '[':
			suffix.WriteByte(']')
		}
	}

	// 拼接：前缀 + 修剪后的主体 + 后缀
	prefix := text[:startIdx]
	return prefix + trimmed + suffix.String()
}
