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

package cli

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	inputHistoryFile = "input_history.json"
	inputHistoryMax  = 200
)

// loadInputHistory reads the persisted input history (newest LAST, matching
// the in-memory submittedInputs order). Corrupt lines are skipped; missing or
// unreadable files yield an empty list (non-fatal).
func loadInputHistory() []string {
	return loadInputHistoryFrom(filepath.Join(prefsDir(), inputHistoryFile))
}

func loadInputHistoryFrom(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var s string
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			continue // skip corrupt line
		}
		out = append(out, s)
	}
	// Cap: keep the most recent inputHistoryMax entries.
	if len(out) > inputHistoryMax {
		out = out[len(out)-inputHistoryMax:]
	}
	return out
}

// appendInputHistory appends one input line to the persisted history
// (NDJSON). Adjacent duplicates are dropped; the file is capped at
// inputHistoryMax entries. All failures are silent (best-effort).
func appendInputHistory(line string) {
	appendInputHistoryTo(filepath.Join(prefsDir(), inputHistoryFile), line)
}

func appendInputHistoryTo(path, line string) {
	if line == "" {
		return
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	cur := loadInputHistoryFrom(path)
	if len(cur) > 0 && cur[len(cur)-1] == line {
		return // adjacent duplicate
	}
	cur = append(cur, line)
	if len(cur) > inputHistoryMax {
		cur = cur[len(cur)-inputHistoryMax:]
	}
	// Rewrite the whole file (simplest correct approach for cap + dedupe).
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, s := range cur {
		b, err := json.Marshal(s)
		if err != nil {
			continue
		}
		w.Write(b)
		w.WriteByte('\n')
	}
	_ = w.Flush()
}
