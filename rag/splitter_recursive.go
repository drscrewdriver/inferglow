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

import "strings"

// RecursiveCharacterTextSplitter splits text using a hierarchy of separators,
// trying each separator in order until chunks are small enough.
type RecursiveCharacterTextSplitter struct {
	// ChunkSize is the maximum size of each chunk (in characters).
	ChunkSize int

	// ChunkOverlap is the number of characters of overlap between chunks.
	ChunkOverlap int

	// Separators is the ordered list of separators to try.
	// Default: ["\n\n", "\n", " ", ""]
	Separators []string
}

// NewRecursiveCharacterTextSplitter creates a splitter with default settings.
func NewRecursiveCharacterTextSplitter(chunkSize, chunkOverlap int) *RecursiveCharacterTextSplitter {
	return &RecursiveCharacterTextSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
		Separators:   []string{"\n\n", "\n", " ", ""},
	}
}

// Split divides text into chunks using recursive character splitting.
func (s *RecursiveCharacterTextSplitter) Split(text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	chunkSize := s.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	overlap := s.ChunkOverlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	seps := s.Separators
	if len(seps) == 0 {
		seps = []string{"\n\n", "\n", " ", ""}
	}

	return s.splitText(text, seps, chunkSize, overlap), nil
}

func (s *RecursiveCharacterTextSplitter) splitText(text string, seps []string, chunkSize, overlap int) []string {
	if len(seps) == 0 {
		// Last resort: hard split by character count
		return hardSplit(text, chunkSize, overlap)
	}

	sep := seps[0]
	remaining := seps[1:]

	var parts []string
	if sep == "" {
		// Empty separator: split by character
		for _, ch := range text {
			parts = append(parts, string(ch))
		}
	} else {
		parts = strings.Split(text, sep)
	}

	var chunks []string
	var current string

	for _, part := range parts {
		candidate := current
		if candidate != "" {
			candidate += sep + part
		} else {
			candidate = part
		}

		if len(candidate) > chunkSize {
			// Flush current buffer
			if current != "" {
				trimmed := strings.TrimSpace(current)
				if trimmed != "" {
					chunks = append(chunks, trimmed)
				}
			}

			if len(part) > chunkSize {
				// Recurse with remaining separators
				subChunks := s.splitText(part, remaining, chunkSize, overlap)
				chunks = append(chunks, subChunks...)
				// Set overlap from last sub-chunk
				if len(subChunks) > 0 {
					last := subChunks[len(subChunks)-1]
					if overlap > 0 && len(last) > overlap {
						current = last[len(last)-overlap:]
					} else {
						current = ""
					}
				}
			} else {
				// Start new chunk with overlap
				if overlap > 0 && len(current) > overlap {
					current = current[len(current)-overlap:] + sep + part
				} else {
					current = part
				}
			}
		} else {
			current = candidate
		}
	}

	// Flush remaining
	if current != "" {
		trimmed := strings.TrimSpace(current)
		if trimmed != "" {
			chunks = append(chunks, trimmed)
		}
	}

	return chunks
}

// hardSplit splits text into chunks of at most chunkSize characters with overlap.
func hardSplit(text string, chunkSize, overlap int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	var chunks []string
	start := 0
	for start < len(runes) {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		start = end - overlap
		if start >= len(runes) {
			break
		}
		// Prevent infinite loop when overlap is 0
		if overlap == 0 {
			start = end
		}
	}
	return chunks
}
