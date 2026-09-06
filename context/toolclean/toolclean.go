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

// Package toolclean provides deterministic, mechanical denoising of tool
// output before it is stored as the context pipeline's L0 record. It is
// purely rule-based (no LLM, no network) and orthogonal to the context-mode
// system: the single ingest funnel cleans the content before any
// ContextManager sees it, so every mode benefits from the same gate.
//
// The cleaner is byte-oriented on purpose: tool output may carry ANSI
// escapes, \r redraw frames and invalid UTF-8, all of which must survive
// the scan without allocation on the noise-free path.
package toolclean

import "strings"

// Report is the verifiable-shrinkage accounting of one Clean run. All
// fields are deterministic (no timestamps, no randomness), so two runs
// over the same input produce identical reports.
type Report struct {
	// Changed reports whether any rule modified the content.
	Changed     bool
	InputBytes  int
	OutputBytes int
	// ANSIRemoved counts ANSI escape sequences eliminated (CSI, OSC,
	// intermediate and two-byte escapes; an unterminated sequence consumes
	// the rest of its line and counts as one).
	ANSIRemoved int
	// CRFolded counts carriage returns eliminated: redraw frames folded
	// to their final frame plus CRLF line endings normalized to LF.
	CRFolded int
	// DupLinesRemoved counts adjacent duplicate non-blank lines dropped
	// (first occurrence kept, mirroring compress.MechanicalL1).
	DupLinesRemoved int
	// ErrorLinesKept counts redraw frames preserved as their own line
	// because they carry an error keyword (they would otherwise be folded
	// away with the rest of the progress-bar frames).
	ErrorLinesKept int
}

// errorKeywords is the conservative reserved-word list for error-line
// preservation. Intentionally small: a false "keep" only costs a few
// bytes, a false "drop" loses an error signal.
var errorKeywords = [...]string{"error", "fatal", "panic", "failed"}

// Clean mechanically denoises tool output. It is a pure function: no I/O,
// no shared state (safe under the concurrent action dispatcher),
// deterministic and idempotent (Clean(Clean(x)) == Clean(x)).
//
// Rules (v1):
//   - strip ANSI escape sequences (colors, cursor moves, OSC titles);
//   - fold \r redraw frames (progress bars) to the final visible frame,
//     keeping frames that carry an error keyword as their own line;
//   - collapse adjacent duplicate non-blank lines;
//   - normalize CRLF line endings to LF.
//
// The cleaner never amplifies and never fails: if the result would be
// longer than the input, or the rules panic, the original content is
// returned unchanged (fail-open). The "[status] ..." prefix added by the
// CLI ingest formatter is plain text and passes through untouched. Noise
// detection and the duplicate-line scan are allocation-free; noise-free
// input is returned as the original string (zero-copy).
func Clean(content string) (cleaned string, rep Report) {
	rep.InputBytes = len(content)
	defer func() {
		// Fail-open guard: a panic or an accidental amplification must
		// never take the tool output down with it.
		if p := recover(); p != nil || len(cleaned) > rep.InputBytes {
			cleaned = content
			rep = Report{InputBytes: len(content), OutputBytes: len(content)}
		}
	}()
	if content == "" {
		return content, rep
	}
	out := content
	rewrote := false
	if strings.IndexByte(content, 0x1b) >= 0 || strings.IndexByte(content, '\r') >= 0 {
		out = stripANSIAndFoldCR(content, &rep)
		rewrote = true
	}
	out = foldAdjacentDups(out, &rep)
	if !rewrote && rep.DupLinesRemoved == 0 {
		rep.OutputBytes = rep.InputBytes
		return content, rep
	}
	rep.Changed = true
	rep.OutputBytes = len(out)
	return out, rep
}

// stripANSIAndFoldCR rewrites one buffer: lines are re-emitted with ANSI
// escapes stripped and \r redraw frames folded. Only called when the
// content is known to contain ESC or CR, so the output always differs.
func stripANSIAndFoldCR(s string, rep *Report) string {
	var b strings.Builder
	b.Grow(len(s)) // output is never longer than the input
	ls := 0
	for ls < len(s) {
		nl := strings.IndexByte(s[ls:], '\n')
		var line string
		if nl < 0 {
			line = s[ls:]
			ls = len(s)
		} else {
			line = s[ls : ls+nl]
			ls += nl + 1
		}
		// CRLF line ending: the '\r' is eliminated (normalized to LF).
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
			rep.CRFolded++
		}
		emitLine(line, &b, rep)
		if nl >= 0 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// emitLine appends the visible content of one logical line (without its
// terminating '\n') to b.
func emitLine(line string, b *strings.Builder, rep *Report) {
	if strings.IndexByte(line, 0x1b) >= 0 {
		line = stripANSI(line, rep)
	}
	if strings.IndexByte(line, '\r') < 0 {
		b.WriteString(line)
		return
	}
	// Redraw fold: every '\r' supersedes the frame before it. Frames that
	// carry an error keyword are rescued as their own line; the rest are
	// dropped and the final frame is emitted. A trailing '\r' leaves an
	// empty final frame — the last non-empty frame is what the terminal
	// still shows, so it is emitted unless it was already rescued.
	segStart := 0
	lastNonEmpty := ""
	lastRescued := false
	for {
		j := strings.IndexByte(line[segStart:], '\r')
		if j < 0 {
			break
		}
		seg := line[segStart : segStart+j]
		segStart += j + 1
		rep.CRFolded++
		if seg != "" {
			lastNonEmpty = seg
			lastRescued = false
		}
		if containsErrorKeyword(seg) {
			b.WriteString(seg)
			b.WriteByte('\n')
			rep.ErrorLinesKept++
			lastRescued = true
		}
	}
	if final := line[segStart:]; final != "" {
		b.WriteString(final)
		return
	}
	if lastNonEmpty != "" && !lastRescued {
		b.WriteString(lastNonEmpty)
	}
}

// stripANSI removes ANSI escape sequences in one chunked scan.
func stripANSI(s string, rep *Report) string {
	var b strings.Builder
	b.Grow(len(s))
	rest := s
	for {
		j := strings.IndexByte(rest, 0x1b)
		if j < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:j])
		n := ansiSeqLen(rest[j:])
		rest = rest[j+n:]
		rep.ANSIRemoved++
	}
}

// ansiSeqLen returns the byte length of the escape sequence at the start
// of s (which must begin with ESC); it always returns >= 1. An
// unterminated sequence consumes the rest of the line.
func ansiSeqLen(s string) int {
	if len(s) < 2 {
		return len(s) // lone ESC at end of line
	}
	switch s[1] {
	case '[': // CSI: parameter bytes 0x30-0x3F, intermediates 0x20-0x2F, final 0x40-0x7E
		for i := 2; i < len(s); i++ {
			c := s[i]
			if c >= 0x40 && c <= 0x7e {
				return i + 1
			}
			if c < 0x20 || c > 0x3f {
				return i // malformed: drop what was consumed so far
			}
		}
		return len(s)
	case ']': // OSC: terminated by BEL or ST (ESC \)
		for i := 2; i < len(s); i++ {
			switch s[i] {
			case 0x07:
				return i + 1
			case 0x1b:
				if i+1 < len(s) && s[i+1] == '\\' {
					return i + 2
				}
				return i // ST not completed: end before the stray ESC
			}
		}
		return len(s)
	default:
		if s[1] >= 0x20 && s[1] <= 0x2f { // intermediates (ESC ( B, ESC # 8, ...)
			i := 1
			for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
				i++
			}
			if i < len(s) {
				return i + 1
			}
			return len(s)
		}
		return 2 // two-byte escape (ESC M, ESC 7, ESC >, ...)
	}
}

// foldAdjacentDups collapses adjacent duplicate non-blank lines (first
// occurrence kept), mirroring the semantics of compress.MechanicalL1 so
// that ingest-time denoise and compression-time mechanical folding stay
// idempotent together. Blank (whitespace-only) lines are never folded.
// When nothing folds, the input string is returned unchanged (zero-copy).
func foldAdjacentDups(s string, rep *Report) string {
	var b strings.Builder
	writing := false
	flushed := 0 // bytes of s already copied into b
	prev := ""
	prevSet := false
	pos := 0
	for pos < len(s) {
		end := len(s)
		hasNL := false
		if nl := strings.IndexByte(s[pos:], '\n'); nl >= 0 {
			end = pos + nl
			hasNL = true
		}
		line := s[pos:end]
		dup := prevSet && line == prev && strings.TrimSpace(line) != ""
		prev = line
		prevSet = true
		if !dup {
			pos = end
			if hasNL {
				pos++
			}
			continue
		}
		if !writing {
			b.Grow(len(s))
			writing = true
		}
		b.WriteString(s[flushed:pos])
		rep.DupLinesRemoved++
		pos = end
		if hasNL {
			pos++
		}
		flushed = pos
	}
	if !writing {
		return s
	}
	b.WriteString(s[flushed:])
	return b.String()
}

// containsErrorKeyword reports whether s carries one of the reserved
// error keywords (case-insensitive ASCII match).
func containsErrorKeyword(s string) bool {
	for _, kw := range errorKeywords {
		if containsFoldASCII(s, kw) {
			return true
		}
	}
	return false
}

// containsFoldASCII is an allocation-free case-insensitive substring
// search (ASCII letters only).
func containsFoldASCII(s, sub string) bool {
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		match := true
		for j := 0; j < n; j++ {
			a := s[i+j]
			b := sub[j]
			if 'A' <= a && a <= 'Z' {
				a += 'a' - 'A'
			}
			if 'A' <= b && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
