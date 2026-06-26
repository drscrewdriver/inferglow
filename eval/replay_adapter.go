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

package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/inferglow/session"
)

// GoldenSession represents a loaded golden session file for replay-based
// evaluation.
type GoldenSession struct {
	// Path is the file path to the golden session JSON.
	Path string

	// data is the parsed session data.
	data session.SessionData
}

// LoadGoldenSession reads and parses a golden session JSON file.
func LoadGoldenSession(path string) (*GoldenSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval: read golden session: %w", err)
	}
	var sd session.SessionData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, fmt.Errorf("eval: parse golden session JSON: %w", err)
	}
	return &GoldenSession{Path: path, data: sd}, nil
}

// ToCase converts a golden session into an evaluation Case. It extracts
// user messages as input and assistant messages as expected Contains
// assertions.
func (g *GoldenSession) ToCase(name string) Case {
	c := Case{Name: name}

	for _, msg := range g.data.FullContext {
		switch msg.Role {
		case "user":
			text := extractGoldenText(msg.Content)
			if text != "" {
				if c.Input != "" {
					c.Input += "\n"
				}
				c.Input += text
			}
		case "assistant":
			text := extractGoldenText(msg.Content)
			if text != "" {
				c.Expect.Contains = append(c.Expect.Contains, text)
			}
		}
	}

	return c
}

// extractGoldenText converts a session ChatMessage Content to a plain string.
// Handles both plain string and []ContentBlock formats.
func extractGoldenText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return fmt.Sprintf("%v", content)
	}
}
