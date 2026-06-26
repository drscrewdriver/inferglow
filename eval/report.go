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

package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// FormatText writes a human-readable text table of the report to w.
func (r *Report) FormatText(w io.Writer) {
	fmt.Fprintf(w, "=== Eval Report: %s ===\n", r.Suite)
	fmt.Fprintf(w, "Total: %d  Passed: %d  Failed: %d\n", r.Total, r.Passed, r.Failed)
	fmt.Fprintf(w, "P50 Latency: %s  P95 Latency: %s\n", r.P50Latency, r.P95Latency)
	fmt.Fprintln(w, strings.Repeat("-", 72))

	for _, res := range r.Results {
		status := "PASS"
		if !res.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(w, "[%s] %s  (%s)\n", status, res.CaseName, res.Latency)
		if res.Response != "" {
			preview := res.Response
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			}
			fmt.Fprintf(w, "  Response: %s\n", preview)
		}
		if len(res.ToolCalls) > 0 {
			fmt.Fprintf(w, "  Tools: %s\n", strings.Join(res.ToolCalls, " → "))
		}
		for _, e := range res.Errors {
			fmt.Fprintf(w, "  ERROR: %s\n", e)
		}
	}
	fmt.Fprintln(w)
}

// FormatJSON writes the report as indented JSON to w.
func (r *Report) FormatJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ExitCode returns 0 if all cases passed, 1 otherwise.
func (r *Report) ExitCode() int {
	if r.Failed > 0 {
		return 1
	}
	return 0
}
