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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package retrieval

import (
	"strings"
	"unicode"
)

// Tokenize splits text into search tokens with CJK bigram support.
//
// - Latin/Cyrillic text: split on whitespace, lowercase, strip punctuation.
// - CJK text (Han/Hiragana/Katakana/Hangul): character bigrams.
//   Example: "机器学习" → ["机器", "器学", "学习"]
// - Mixed text: each script region handled independently.
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder
	var prevCJK bool
	var pendingCJK rune

	for _, r := range text {
		if isCJK(r) {
			// Flush any accumulated Latin word.
			if current.Len() > 0 {
				flushWord(&current, &tokens)
			}
			// Produce bigram with previous CJK character.
			if prevCJK {
				tokens = append(tokens, string([]rune{pendingCJK, r}))
			}
			pendingCJK = r
			prevCJK = true
		} else if unicode.IsSpace(r) || unicode.IsPunct(r) {
			// Flush Latin word.
			if current.Len() > 0 {
				flushWord(&current, &tokens)
			}
			// Reset CJK state — punctuation/space breaks the bigram chain.
			prevCJK = false
			pendingCJK = 0
		} else {
			// Regular Latin/alphanumeric character.
			if prevCJK {
				// Transition from CJK to Latin — reset CJK chain.
				prevCJK = false
				pendingCJK = 0
			}
			current.WriteRune(r)
		}
	}
	// Flush remaining.
	if current.Len() > 0 {
		flushWord(&current, &tokens)
	}
	return tokens
}

// isCJK reports whether r belongs to a CJK script family.
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// flushWord trims punctuation from the builder, appends the result to tokens
// if long enough, and resets the builder.
func flushWord(b *strings.Builder, tokens *[]string) {
	w := strings.Trim(b.String(), ".,;:!?()[]{}\"'`")
	b.Reset()
	if len(w) > 1 {
		*tokens = append(*tokens, w)
	}
}
