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

func TestMechanicalCompress_Level1(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, output string)
	}{
		{
			name:  "removes consecutive blank lines",
			input: "line1\n\n\n\nline2\n\n\nline3",
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "\n\n\n") {
					t.Errorf("expected no triple newlines, got: %q", out)
				}
			},
		},
		{
			name:  "removes filler phrases",
			input: "让我来看看这个结果。\n好的，现在开始处理。\n以上就是全部内容。\n我来帮你解决。\nkeep this",
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "让我来看看") {
					t.Errorf("expected filler '让我来看看' removed, got: %q", out)
				}
				if strings.Contains(out, "好的，现在") {
					t.Errorf("expected filler '好的，现在' removed, got: %q", out)
				}
				if strings.Contains(out, "以上就是") {
					t.Errorf("expected filler '以上就是' removed, got: %q", out)
				}
				if strings.Contains(out, "我来帮你") {
					t.Errorf("expected filler '我来帮你' removed, got: %q", out)
				}
				if !strings.Contains(out, "keep this") {
					t.Errorf("expected content 'keep this' preserved, got: %q", out)
				}
			},
		},
		{
			name:  "preserves code blocks",
			input: "some text\n```\nfunc main() {\n\treturn\n}\n```\nmore text",
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "```") {
					t.Errorf("expected code block preserved, got: %q", out)
				}
				if !strings.Contains(out, "func main()") {
					t.Errorf("expected code content preserved, got: %q", out)
				}
			},
		},
		{
			name:  "removes duplicate adjacent lines",
			input: "line1\nline1\nline2\nline2\nline2\nline3",
			check: func(t *testing.T, out string) {
				lines := strings.Split(out, "\n")
				// "line1" appears twice adjacently → should be collapsed to one
				// "line2" appears three times adjacently → should be collapsed to one
				count1 := 0
				count2 := 0
				for _, line := range lines {
					if line == "line1" {
						count1++
					}
					if line == "line2" {
						count2++
					}
				}
				if count1 > 1 {
					t.Errorf("expected line1 deduplicated, got %d occurrences", count1)
				}
				if count2 > 1 {
					t.Errorf("expected line2 deduplicated, got %d occurrences", count2)
				}
				if !strings.Contains(out, "line3") {
					t.Errorf("expected line3 preserved, got: %q", out)
				}
			},
		},
		{
			name:  "truncates long lines",
			input: "short\n" + strings.Repeat("a", 600) + "\nshort2",
			check: func(t *testing.T, out string) {
				lines := strings.Split(out, "\n")
				// Truncated line = 500 chars + "...[截断]" (11 bytes) = 511 bytes
				for _, line := range lines {
					if len(line) > 511 && line != "short" && line != "short2" {
						t.Errorf("long line should be truncated, len=%d > 511", len(line))
					}
				}
				if !strings.Contains(out, "[截断]") {
					t.Errorf("expected truncation marker, got: %q", out)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MechanicalL1(tt.input)
			tt.check(t, result)
		})
	}
}

func TestMechanicalCompress_Level2(t *testing.T) {
	input := `some processing happening
/var/log/app.go:42 error occurred
some intermediate reasoning
CONFIG_PATH=/etc/app/config.yaml
ERROR: connection refused
more exploration
`

	result := MechanicalL2(input)
	t.Logf("L2 result:\n%s", result)

	if !strings.Contains(result, "[事实]") {
		t.Errorf("expected facts prefixed with [事实], got: %q", result)
	}
	if !strings.Contains(result, "/var/log/app.go:42") {
		t.Errorf("expected file:line pattern extracted, got: %q", result)
	}
	if !strings.Contains(result, "CONFIG_PATH=/etc/app/config.yaml") {
		t.Errorf("expected KEY=VALUE pattern extracted, got: %q", result)
	}
	if !strings.Contains(result, "ERROR: connection refused") {
		t.Errorf("expected error line extracted, got: %q", result)
	}
}

func TestMechanicalCompress_Level2_Fallback(t *testing.T) {
	input := "plain text without any facts or patterns to extract"
	result := MechanicalL2(input)
	if !strings.Contains(result, "[事实]") {
		t.Errorf("expected fallback with [事实] prefix, got: %q", result)
	}
}

func TestMechanicalCompress_Level3(t *testing.T) {
	result := MechanicalL3(5, "read_file", "path=main.go", "some content here")
	expectedPrefix := "[掩码 step_5|"
	if !strings.HasPrefix(result, expectedPrefix) {
		t.Errorf("expected mask starting with %q, got: %q", expectedPrefix, result)
	}
	if !strings.Contains(result, "read_file") {
		t.Errorf("expected tool name in mask, got: %q", result)
	}
	if !strings.Contains(result, "path=main.go") {
		t.Errorf("expected key params in mask, got: %q", result)
	}
}

func TestMechanicalCompress_InvalidLevel(t *testing.T) {
	tests := []struct {
		level int
		name  string
	}{
		{0, "level 0 returns unchanged"},
		{4, "level 4 returns unchanged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "some content to test"
			result, err := MechanicalCompress(tt.level, input)
			if err != nil {
				t.Fatalf("MechanicalCompress(%d) returned error: %v", tt.level, err)
			}
			if result != input {
				t.Errorf("expected content unchanged for level %d, got %q", tt.level, result)
			}
		})
	}
}
