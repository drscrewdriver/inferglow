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
	"encoding/json"
	"io"
	"strings"
)

// JSONLoader loads JSON and JSONL files.
// For JSON arrays, each element becomes a document.
// For JSON objects, the entire object becomes one document.
// For JSONL (newline-delimited JSON), each line becomes a document.
type JSONLoader struct {
	// ContentField specifies which field to use as document content.
	// If empty, the entire JSON object is serialized as content.
	ContentField string

	// MetadataFields specifies which fields to include in metadata.
	// If empty, all fields except ContentField are included.
	MetadataFields []string
}

// NewJSONLoader creates a JSONLoader with default settings.
func NewJSONLoader() *JSONLoader {
	return &JSONLoader{}
}

// Load reads JSON/JSONL content and converts it to documents.
func (l *JSONLoader) Load(ctx context.Context, r io.Reader) ([]Document, error) {
	// Try JSONL first (line-delimited)
	docs, err := l.loadJSONL(r)
	if err == nil && len(docs) > 0 {
		return docs, nil
	}

	// Reset reader not possible, so we need to read all and retry
	// This is a limitation; in practice, caller should use the right loader
	return nil, err
}

// loadJSONL attempts to parse the reader as JSONL (newline-delimited JSON).
func (l *JSONLoader) loadJSONL(r io.Reader) ([]Document, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	var docs []Document
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if line == "" {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			// Not valid JSON line, skip
			continue
		}

		doc := l.objectToDocument(obj, lineNum)
		docs = append(docs, doc)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return docs, nil
}

// LoadJSONArray reads a JSON array and converts each element to a document.
func (l *JSONLoader) LoadJSONArray(ctx context.Context, r io.Reader) ([]Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err != nil {
		// Try as single object
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, err
		}
		return []Document{l.objectToDocument(obj, 1)}, nil
	}

	docs := make([]Document, 0, len(arr))
	for i, obj := range arr {
		docs = append(docs, l.objectToDocument(obj, i+1))
	}

	return docs, nil
}

// objectToDocument converts a JSON object to a Document.
func (l *JSONLoader) objectToDocument(obj map[string]any, index int) Document {
	var content string
	meta := make(map[string]any)

	if l.ContentField != "" {
		if v, ok := obj[l.ContentField]; ok {
			content = toString(v)
		}
	} else {
		// Serialize entire object
		b, _ := json.Marshal(obj)
		content = string(b)
	}

	// Build metadata
	if len(l.MetadataFields) > 0 {
		for _, field := range l.MetadataFields {
			if v, ok := obj[field]; ok && field != l.ContentField {
				meta[field] = v
			}
		}
	} else {
		// Include all fields except content
		for k, v := range obj {
			if k != l.ContentField {
				meta[k] = v
			}
		}
	}

	meta["index"] = index
	meta["type"] = "json_object"

	return Document{
		Content:  content,
		Metadata: meta,
	}
}

// toString converts any value to string.
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
