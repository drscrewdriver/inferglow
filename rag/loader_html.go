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
	"context"
	"html"
	"io"
	"strings"
)

// HTMLLoader loads HTML files, extracting text content.
type HTMLLoader struct {
	// KeepStructure preserves heading structure in metadata when true.
	KeepStructure bool
}

// NewHTMLLoader creates an HTMLLoader with default settings.
func NewHTMLLoader() *HTMLLoader {
	return &HTMLLoader{KeepStructure: true}
}

// Load reads HTML content and extracts text, splitting by block elements.
func (l *HTMLLoader) Load(ctx context.Context, r io.Reader) ([]Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// Extract body content if present
	bodyStart := strings.Index(strings.ToLower(content), "<body")
	bodyEnd := strings.Index(strings.ToLower(content), "</body>")
	if bodyStart >= 0 && bodyEnd > bodyStart {
		// Find the end of the <body> tag
		tagEnd := strings.Index(content[bodyStart:], ">")
		if tagEnd >= 0 {
			content = content[bodyStart+tagEnd+1 : bodyEnd]
		}
	}

	// Remove script and style tags
	content = removeTags(content, "script")
	content = removeTags(content, "style")

	// Split by block-level elements
	sections := splitByBlocks(content)

	var docs []Document
	for _, section := range sections {
		text := extractText(section)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		meta := map[string]any{"type": "html_section"}

		// Try to extract heading if present
		if l.KeepStructure {
			if heading := extractFirstHeading(section); heading != "" {
				meta["heading"] = heading
			}
		}

		docs = append(docs, Document{
			Content:  text,
			Metadata: meta,
		})
	}

	// If no sections found, return entire text as one document
	if len(docs) == 0 {
		text := extractText(content)
		text = strings.TrimSpace(text)
		if text != "" {
			docs = append(docs, Document{
				Content:  text,
				Metadata: map[string]any{"type": "html"},
			})
		}
	}

	return docs, nil
}

// removeTags removes all occurrences of a specific tag and its content.
func removeTags(content, tag string) string {
	for {
		start := strings.Index(strings.ToLower(content), "<"+tag)
		if start < 0 {
			break
		}
		end := strings.Index(strings.ToLower(content), "</"+tag+">")
		if end < 0 {
			break
		}
		end += len("</" + tag + ">")
		content = content[:start] + " " + content[end:]
	}
	return content
}

// splitByBlocks splits HTML content by block-level elements.
func splitByBlocks(content string) []string {
	// Split by common block elements
	blockTags := []string{"<p>", "<div>", "<section>", "<article>", "<header>", "<footer>", "<li>", "<blockquote>"}
	result := []string{content}

	for _, tag := range blockTags {
		var newResult []string
		for _, section := range result {
			parts := strings.Split(section, tag)
			newResult = append(newResult, parts...)
		}
		result = newResult
	}

	// Also split by closing tags
	closeTags := []string{"</p>", "</div>", "</section>", "</article>"}
	for _, tag := range closeTags {
		var newResult []string
		for _, section := range result {
			parts := strings.Split(section, tag)
			newResult = append(newResult, parts...)
		}
		result = newResult
	}

	return result
}

// extractText extracts plain text from HTML content.
func extractText(content string) string {
	// Decode HTML entities
	text := html.UnescapeString(content)

	// Remove all HTML tags
	var result strings.Builder
	inTag := false
	for _, ch := range text {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			result.WriteRune(' ') // Replace tag with space
			continue
		}
		if !inTag {
			result.WriteRune(ch)
		}
	}

	// Normalize whitespace
	text = result.String()
	// Replace multiple spaces/newlines with single space
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	for strings.Contains(text, "\n\n") {
		text = strings.ReplaceAll(text, "\n\n", "\n")
	}

	return strings.TrimSpace(text)
}

// extractFirstHeading extracts the first heading (h1-h6) from HTML content.
func extractFirstHeading(content string) string {
	lower := strings.ToLower(content)
	for i := 1; i <= 6; i++ {
		tag := "<h" + string(rune('0'+i))
		endTag := "</h" + string(rune('0'+i)) + ">"

		start := strings.Index(lower, tag)
		if start < 0 {
			continue
		}

		// Find end of opening tag
		tagEnd := strings.Index(content[start:], ">")
		if tagEnd < 0 {
			continue
		}
		contentStart := start + tagEnd + 1

		// Find closing tag
		end := strings.Index(strings.ToLower(content[contentStart:]), endTag)
		if end < 0 {
			continue
		}

		heading := content[contentStart : contentStart+end]
		return extractText(heading)
	}
	return ""
}
