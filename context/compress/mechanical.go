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

import (
	"fmt"
	"regexp"
	"strings"
)

// MechanicalCompress performs compression without LLM (§3.1 mechanical fallback).
func MechanicalCompress(level int, content string) (string, error) {
	switch level {
	case 1:
		return MechanicalL1(content), nil
	case 2:
		return MechanicalL2(content), nil
	case 3:
		return MechanicalL3(0, "", "", content), nil
	default:
		return content, nil
	}
}

// MechanicalL1 performs regex-based denoising (§4.4).
func MechanicalL1(content string) string {
	// Remove consecutive blank lines → single blank line
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")

	// Remove filler phrases at line start
	fillers := []string{
		`^让我来.*\n`,
		`^好的，现在.*\n`,
		`^以上就是.*\n`,
		`^我来帮你.*\n`,
	}
	for _, f := range fillers {
		re := regexp.MustCompile(f)
		content = re.ReplaceAllString(content, "")
	}

	// Remove duplicate adjacent lines
	lines := strings.Split(content, "\n")
	var result []string
	prev := ""
	for _, line := range lines {
		if line != prev || strings.TrimSpace(line) == "" {
			result = append(result, line)
		}
		prev = line
	}

	// Truncate lines longer than 500 chars
	for i, line := range result {
		if len(line) > 500 {
			result[i] = line[:500] + "...[截断]"
		}
	}

	return strings.Join(result, "\n")
}

// MechanicalL2 extracts key patterns (§4.4).
func MechanicalL2(content string) string {
	var facts []string

	// Extract file:line patterns
	pathRe := regexp.MustCompile(`[\w/]+\.\w+:\d+`)
	for _, match := range pathRe.FindAllString(content, 10) {
		facts = append(facts, "[事实] "+match)
	}

	// Extract KEY=VALUE patterns
	kvRe := regexp.MustCompile(`[A-Z_]+=[\w./\-]+`)
	for _, match := range kvRe.FindAllString(content, 10) {
		facts = append(facts, "[事实] "+match)
	}

	// Extract error lines
	errRe := regexp.MustCompile(`(?m)^(error|Error|ERROR).*`)
	for _, match := range errRe.FindAllString(content, 5) {
		facts = append(facts, "[事实] "+match)
	}

	if len(facts) == 0 {
		// Fallback: first 200 chars
		if len(content) > 200 {
			return "[事实] " + content[:200] + "..."
		}
		return "[事实] " + content
	}

	return strings.Join(facts, "\n")
}

// MechanicalL3 generates a structural mask without LLM (§4.4).
// Format: [掩码 step_N|原X t|tool|params] {工具名称用法} 总token量
func MechanicalL3(stepID int, toolName, keyParams, content string) string {
	tokenCount := len(content) / 4
	return fmt.Sprintf("[掩码 step_%d|原%dt|%s|%s] 工具:%s 总%dt",
		stepID, tokenCount, toolName, keyParams, toolName, tokenCount)
}
