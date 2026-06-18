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
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// CSVLoader loads CSV files, creating one document per row.
// Column headers are stored in metadata.
type CSVLoader struct {
	// ContentColumns specifies which columns to use as document content.
	// If empty, all columns are concatenated.
	ContentColumns []string

	// HasHeader indicates whether the first row is a header row.
	// Default is true.
	HasHeader bool

	// Comma is the field delimiter. Default is ','.
	Comma rune
}

// NewCSVLoader creates a CSVLoader with default settings.
func NewCSVLoader() *CSVLoader {
	return &CSVLoader{
		HasHeader: true,
		Comma:     ',',
	}
}

// Load reads CSV content and converts each row to a document.
func (l *CSVLoader) Load(ctx context.Context, r io.Reader) ([]Document, error) {
	reader := csv.NewReader(r)
	if l.Comma != 0 {
		reader.Comma = l.Comma
	}
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv read: %w", err)
	}

	if len(records) == 0 {
		return nil, nil
	}

	var headers []string
	startRow := 0

	if l.HasHeader && len(records) > 0 {
		headers = records[0]
		startRow = 1
	}

	docs := make([]Document, 0, len(records)-startRow)
	for i := startRow; i < len(records); i++ {
		row := records[i]
		doc := l.rowToDocument(row, headers, i-startRow+1)
		docs = append(docs, doc)
	}

	return docs, nil
}

// rowToDocument converts a CSV row to a Document.
func (l *CSVLoader) rowToDocument(row, headers []string, rowNum int) Document {
	var contentParts []string
	meta := make(map[string]any)
	meta["type"] = "csv_row"
	meta["row_num"] = rowNum

	contentCols := l.ContentColumns
	if len(contentCols) == 0 {
		// Use all columns
		for i, val := range row {
			val = strings.TrimSpace(val)
			if i < len(headers) && headers[i] != "" {
				meta[headers[i]] = val
				contentParts = append(contentParts, val)
			} else {
				colName := fmt.Sprintf("col%d", i)
				meta[colName] = val
				contentParts = append(contentParts, val)
			}
		}
	} else {
		// Use specified columns for content
		colMap := make(map[string]int)
		for i, h := range headers {
			colMap[h] = i
		}

		for _, col := range contentCols {
			if idx, ok := colMap[col]; ok && idx < len(row) {
				contentParts = append(contentParts, strings.TrimSpace(row[idx]))
			}
		}

		// Add all other columns to metadata
		for i, val := range row {
			if i < len(headers) {
				isContentCol := false
				for _, cc := range contentCols {
					if cc == headers[i] {
						isContentCol = true
						break
					}
				}
				if !isContentCol {
					meta[headers[i]] = strings.TrimSpace(val)
				}
			}
		}
	}

	content := strings.Join(contentParts, " ")

	return Document{
		Content:  content,
		Metadata: meta,
	}
}
