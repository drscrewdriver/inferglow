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

	return nil, fmt.Errorf("no valid JSON found in text")
}

// extractDirect 尝试直接解析整段文本为 JSON
func extractDirect(text string) (map[string]any, error) {
	trimmed := strings.TrimSpace(text)
	// 去除可能的 markdown 代码块
	trimmed = stripCodeBlock(trimmed)

	if len(trimmed) > 0 && trimmed[0] == '{' {
		return parseJSON(trimmed)
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
