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
	"strings"
)

// MarkdownSplitter splits text by Markdown headings (# / ## / ###).
type MarkdownSplitter struct {
	// MaxChunkSize is the maximum size of each chunk in characters.
	// If a section exceeds this size, it is further split by paragraphs.
	MaxChunkSize int
}

// NewMarkdownSplitter creates a MarkdownSplitter with the given max chunk size.
func NewMarkdownSplitter(maxChunkSize int) *MarkdownSplitter {
	if maxChunkSize <= 0 {
		maxChunkSize = 1000
	}
	return &MarkdownSplitter{MaxChunkSize: maxChunkSize}
}

// Split divides Markdown text into chunks by heading boundaries.
func (s *MarkdownSplitter) Split(text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")
	var sections []string
	var current strings.Builder

	for _, line := range lines {
		if isMarkdownHeading(line) {
			if current.Len() > 0 {
				sections = append(sections, strings.TrimSpace(current.String()))
				current.Reset()
			}
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}

	// Flush last section
	if current.Len() > 0 {
		sections = append(sections, strings.TrimSpace(current.String()))
	}

	// Further split oversized sections
	var chunks []string
	for _, section := range sections {
		if len(section) > s.MaxChunkSize {
			subChunks := splitByParagraph(section, s.MaxChunkSize)
			chunks = append(chunks, subChunks...)
		} else if section != "" {
			chunks = append(chunks, section)
		}
	}

	return chunks, nil
}

// isMarkdownHeading reports whether the line is a Markdown heading.
func isMarkdownHeading(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] != '#' {
		return false
	}
	// Count # characters
	level := 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	// Must be 1-6 # followed by a space or end of line
	return level >= 1 && level <= 6 && (len(trimmed) == level || trimmed[level] == ' ')
}

// splitByParagraph splits text by double-newline boundaries, respecting max size.
func splitByParagraph(text string, maxSize int) []string {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	var current string

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		candidate := current
		if candidate != "" {
			candidate += "\n\n" + p
		} else {
			candidate = p
		}

		if len(candidate) > maxSize && current != "" {
			chunks = append(chunks, strings.TrimSpace(current))
			current = p
		} else {
			current = candidate
		}
	}

	if current != "" {
		trimmed := strings.TrimSpace(current)
		if trimmed != "" {
			chunks = append(chunks, trimmed)
		}
	}

	return chunks
}
