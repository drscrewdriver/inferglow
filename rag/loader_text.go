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

// TextLoader loads plain text files, splitting by paragraphs.
type TextLoader struct {
	// ParagraphSep is the separator used to split text into paragraphs.
	// Default is "\n\n" (double newline).
	ParagraphSep string
}

// NewTextLoader creates a TextLoader with default settings.
func NewTextLoader() *TextLoader {
	return &TextLoader{ParagraphSep: "\n\n"}
}

// Load reads text from the reader and splits it into documents by paragraph.
func (l *TextLoader) Load(ctx context.Context, r io.Reader) ([]Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	text := string(data)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	sep := l.ParagraphSep
	if sep == "" {
		sep = "\n\n"
	}

	paragraphs := strings.Split(text, sep)
	var docs []Document
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		docs = append(docs, Document{
			Content:  p,
			Metadata: map[string]any{"type": "paragraph"},
		})
	}

	// If no paragraphs found, return entire text as one document
	if len(docs) == 0 {
		docs = append(docs, Document{
			Content:  text,
			Metadata: map[string]any{"type": "text"},
		})
	}

	return docs, nil
}

// LineLoader loads text files, one document per line.
type LineLoader struct{}

// NewLineLoader creates a LineLoader.
func NewLineLoader() *LineLoader {
	return &LineLoader{}
}

// Load reads text line by line, creating one document per non-empty line.
func (l *LineLoader) Load(ctx context.Context, r io.Reader) ([]Document, error) {
	scanner := bufio.NewScanner(r)
	var docs []Document
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if line == "" {
			continue
		}
		docs = append(docs, Document{
			Content: line,
			Metadata: map[string]any{
				"type":     "line",
				"line_num": lineNum,
			},
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return docs, nil
}
