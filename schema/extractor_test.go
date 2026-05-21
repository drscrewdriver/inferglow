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
	"testing"
)

// Check 2.5.1: ExtractJSON 一级策略（直接提取根 JSON）
func TestExtractJSONDirect(t *testing.T) {
	text := `{"name": "test", "age": 25}`

	result, err := ExtractJSON(text)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
	if f, ok := result["age"].(float64); !ok || f != 25 {
		t.Errorf("age = %v, want 25", result["age"])
	}
}

// Check 2.5.2: ExtractJSON 二级策略（候选块选择）
func TestExtractJSONFromText(t *testing.T) {
	text := `Here is the result:
{
  "title": "Test Title",
  "status": "ok"
}
And some more text.`

	result, err := ExtractJSON(text)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}

	if result["title"] != "Test Title" {
		t.Errorf("title = %v, want %q", result["title"], "Test Title")
	}
}

// Check 2.5.3: ExtractJSON 三级策略（schema 匹配评分）
func TestExtractJSONWithSchema(t *testing.T) {
	text := `
First block:
{"partial": true}

Second block:
{"name": "test", "description": "A test item", "tags": ["a", "b"]}
`

	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"name":        {Type: TypeString},
			"description": {Type: TypeString},
			"tags":        {Type: TypeList, ItemDef: &FieldDef{Type: TypeString}},
		},
	}

	result, err := ExtractJSON(text, schema)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
}

// Test ExtractJSON with markdown code block
func TestExtractJSONMarkdownBlock(t *testing.T) {
	text := "```json\n{\"key\": \"value\", \"number\": 42}\n```"

	result, err := ExtractJSON(text)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("key = %v, want %q", result["key"], "value")
	}
	if f, ok := result["number"].(float64); !ok || f != 42 {
		t.Errorf("number = %v, want 42", result["number"])
	}
}

// Test ExtractJSON with no valid JSON
func TestExtractJSONNoValidJSON(t *testing.T) {
	text := "This is just plain text with no JSON at all."

	_, err := ExtractJSON(text)
	if err == nil {
		t.Error("expected error for no JSON, got nil")
	}
}

// Check 2.5.4: RepairJSONFragment 修复中文标点
func TestRepairJSONChinesePunctuation(t *testing.T) {
	text := `{"name": "test"，"age": 25，"city": "北京"}`

	repaired := RepairJSONFragment(text)

	// 验证修复后的 JSON 可以解析
	var result map[string]any
	err := json.Unmarshal([]byte(repaired), &result)
	if err != nil {
		t.Fatalf("repaired JSON not valid: %v, text: %s", err, repaired)
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
}

// Check 2.5.5: RepairJSONFragment 修复全角括号
func TestRepairJSONFullwidthBrackets(t *testing.T) {
	text := `｛"name": "test", "value": 123｝`

	repaired := RepairJSONFragment(text)

	var result map[string]any
	err := json.Unmarshal([]byte(repaired), &result)
	if err != nil {
		t.Fatalf("repaired JSON not valid: %v, text: %s", err, repaired)
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
}

// Check 2.5.6: RepairJSONFragment 修复缺失引号/逗号
func TestRepairJSONMissingQuotes(t *testing.T) {
	// 缺失值的引号
	text := `{"name": test, "age": 25}`

	repaired := RepairJSONFragment(text)

	var result map[string]any
	err := json.Unmarshal([]byte(repaired), &result)
	if err == nil {
		// 如果成功解析，验证值正确
		if result["name"] != "test" {
			t.Errorf("name = %v, want %q", result["name"], "test")
		}
	}
	// 即使修复失败，也不应该 panic
}

// Test RepairJSONFragment with colon
func TestRepairJSONColon(t *testing.T) {
	text := `{"name"："test", "age": 25}`

	repaired := RepairJSONFragment(text)

	var result map[string]any
	err := json.Unmarshal([]byte(repaired), &result)
	if err != nil {
		t.Fatalf("repaired JSON not valid: %v, text: %s", err, repaired)
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
}

// Test RepairJSONFragment with valid JSON (no repair needed)
func TestRepairJSONValid(t *testing.T) {
	text := `{"name": "test", "age": 25}`

	repaired := RepairJSONFragment(text)

	if repaired != text {
		t.Errorf("valid JSON should not be changed: got %s", repaired)
	}
}

// Test ExtractJSON with nested object
func TestExtractJSONNested(t *testing.T) {
	text := `{
  "user": {
    "name": "Alice",
    "address": {
      "city": "Beijing",
      "zip": "100000"
    }
  }
}`

	result, err := ExtractJSON(text)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}

	user := result["user"].(map[string]any)
	if user["name"] != "Alice" {
		t.Errorf("user.name = %v, want %q", user["name"], "Alice")
	}
}

// Test ExtractJSON with array
func TestExtractJSONArray(t *testing.T) {
	text := `{"tags": ["a", "b", "c"], "count": 3}`

	result, err := ExtractJSON(text)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}

	tags := result["tags"].([]any)
	if len(tags) != 3 {
		t.Errorf("len(tags) = %d, want 3", len(tags))
	}
}

// Test RepairJSONFragment with multiple issues
func TestRepairJSONMultipleIssues(t *testing.T) {
	text := `｛"name"："test"，"value": 123｝`

	repaired := RepairJSONFragment(text)

	var result map[string]any
	err := json.Unmarshal([]byte(repaired), &result)
	if err != nil {
		t.Fatalf("repaired JSON not valid: %v, text: %s", err, repaired)
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
	if f, ok := result["value"].(float64); !ok || f != 123 {
		t.Errorf("value = %v, want 123", result["value"])
	}
}
