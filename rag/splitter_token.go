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

// TokenSplitter splits text by estimated token count.
// It uses a simple heuristic: 1 token ≈ 4 characters (English approximation).
type TokenSplitter struct {
	// MaxTokens is the maximum number of tokens per chunk.
	MaxTokens int

	// TokenOverlap is the number of tokens of overlap between chunks.
	TokenOverlap int
}

// NewTokenSplitter creates a TokenSplitter with the given token limits.
func NewTokenSplitter(maxTokens, tokenOverlap int) *TokenSplitter {
	return &TokenSplitter{
		MaxTokens:    maxTokens,
		TokenOverlap: tokenOverlap,
	}
}

// estimateTokens provides a rough token count from text length.
func estimateTokens(text string) int {
	return len([]rune(text)) / 4
}

// Split divides text into chunks based on estimated token count.
func (s *TokenSplitter) Split(text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	maxTokens := s.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 250
	}
	overlap := s.TokenOverlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxTokens {
		overlap = maxTokens / 4
	}

	// Convert token limits to character limits
	charLimit := maxTokens * 4
	overlapChars := overlap * 4

	words := strings.Fields(text)
	var chunks []string
	var current []string
	currentLen := 0

	for _, word := range words {
		wordLen := len([]rune(word)) + 1 // +1 for space
		if currentLen+wordLen > charLimit && len(current) > 0 {
			chunk := strings.Join(current, " ")
			chunks = append(chunks, chunk)

			// Calculate overlap
			if overlapChars > 0 {
				var overlapWords []string
				overlapLen := 0
				for i := len(current) - 1; i >= 0; i-- {
					wl := len([]rune(current[i])) + 1
					if overlapLen+wl > overlapChars {
						break
					}
					overlapWords = append([]string{current[i]}, overlapWords...)
					overlapLen += wl
				}
				current = overlapWords
				currentLen = overlapLen
			} else {
				current = nil
				currentLen = 0
			}
		}
		current = append(current, word)
		currentLen += wordLen
	}

	// Flush remaining
	if len(current) > 0 {
		chunk := strings.Join(current, " ")
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
