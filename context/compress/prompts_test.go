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
	"strings"
	"testing"
)

func TestBuildPrompt_L1(t *testing.T) {
	content := "some log content\nmore logs"
	result := BuildPrompt(1, 0, "", "", content)

	if !strings.Contains(result, "你是一个文本压缩器") {
		t.Errorf("L1 prompt should contain compression instruction, got: %q", result)
	}
	if !strings.Contains(result, content) {
		t.Errorf("L1 prompt should contain original content, got: %q", result)
	}
	if !strings.Contains(result, "[System]") {
		t.Errorf("L1 prompt should contain [System] section, got: %q", result)
	}
	if !strings.Contains(result, "[User]") {
		t.Errorf("L1 prompt should contain [User] section, got: %q", result)
	}
}

func TestBuildPrompt_L2(t *testing.T) {
	content := "some tool output with file.go:123 error"
	result := BuildPrompt(2, 42, "read_file", "path=main.go", content)

	if !strings.Contains(result, "你是一个事实提取器") {
		t.Errorf("L2 prompt should contain fact extraction instruction, got: %q", result)
	}
	if !strings.Contains(result, "step_id: 42") {
		t.Errorf("L2 prompt should contain step_id, got: %q", result)
	}
	if !strings.Contains(result, "read_file") {
		t.Errorf("L2 prompt should contain tool_name, got: %q", result)
	}
	if !strings.Contains(result, "path=main.go") {
		t.Errorf("L2 prompt should contain key_params, got: %q", result)
	}
	if !strings.Contains(result, "[掩码") {
		t.Errorf("L2 prompt should mention mask header format, got: %q", result)
	}
}

func TestBuildPrompt_L3(t *testing.T) {
	content := "some tool output data"
	result := BuildPrompt(3, 7, "write_file", "path=test.txt", content)

	if !strings.Contains(result, "你是一个掩码生成器") {
		t.Errorf("L3 prompt should contain mask generation instruction, got: %q", result)
	}
	if !strings.Contains(result, "[掩码 step_7") {
		t.Errorf("L3 prompt should contain mask format with step_7, got: %q", result)
	}
	if !strings.Contains(result, "write_file") {
		t.Errorf("L3 prompt should contain tool name, got: %q", result)
	}
	if !strings.Contains(result, "path=test.txt") {
		t.Errorf("L3 prompt should contain key params, got: %q", result)
	}
}

func TestBuildPrompt_Default(t *testing.T) {
	content := "original content"
	// Level 0 should return content unchanged
	result := BuildPrompt(0, 0, "", "", content)
	if result != content {
		t.Errorf("expected original content for level 0, got: %q", result)
	}

	// Level 4 (unsupported) should also return content unchanged
	result = BuildPrompt(4, 0, "", "", content)
	if result != content {
		t.Errorf("expected original content for level 4, got: %q", result)
	}
}
