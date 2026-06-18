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

package rag

import (
	"bufio"
	"context"
	"io"
	"strings"
)

// MarkdownLoader loads Markdown files, splitting by heading hierarchy.
type MarkdownLoader struct{}

// NewMarkdownLoader creates a MarkdownLoader.
func NewMarkdownLoader() *MarkdownLoader {
	return &MarkdownLoader{}
}

// Load reads Markdown content and splits it into documents by headings.
// Each heading starts a new document section, with the heading text stored in metadata.
func (l *MarkdownLoader) Load(ctx context.Context, r io.Reader) ([]Document, error) {
	scanner := bufio.NewScanner(r)
	var docs []Document
	var currentContent strings.Builder
	var currentHeading string
	var currentLevel int

	flushSection := func() {
		content := strings.TrimSpace(currentContent.String())
		if content == "" {
			return
		}
		meta := map[string]any{"type": "markdown_section"}
		if currentHeading != "" {
			meta["heading"] = currentHeading
			meta["heading_level"] = currentLevel
		}
		docs = append(docs, Document{
			Content:  content,
			Metadata: meta,
		})
		currentContent.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check if this is a heading
		if level, heading := parseHeading(trimmed); level > 0 {
			// Flush previous section
			flushSection()
			currentHeading = heading
			currentLevel = level
			continue
		}

		// Accumulate content
		if currentContent.Len() > 0 {
			currentContent.WriteString("\n")
		}
		currentContent.WriteString(line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Flush final section
	flushSection()

	// If no sections found, return entire content as one document
	if len(docs) == 0 {
		return nil, nil
	}

	return docs, nil
}

// parseHeading checks if a line is a Markdown heading and returns the level and text.
// Returns (0, "") if not a heading.
func parseHeading(line string) (int, string) {
	if !strings.HasPrefix(line, "#") {
		return 0, ""
	}

	level := 0
	for _, ch := range line {
		if ch == '#' {
			level++
		} else {
			break
		}
	}

	if level > 6 || level == 0 {
		return 0, ""
	}

	// Must have a space after #
	if len(line) <= level || line[level] != ' ' {
		return 0, ""
	}

	return level, strings.TrimSpace(line[level:])
}
