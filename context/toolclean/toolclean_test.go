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

package toolclean

import (
	"strconv"
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		changed bool
		ansi    int
		cr      int
		dup     int
		errKept int
	}{
		{
			name: "noise-free passthrough",
			in:   "[ok] file list\none\ntwo\n",
			want: "[ok] file list\none\ntwo\n",
		},
		{
			name: "plain CJK passthrough",
			in:   "中文输出正常\n第二行\n",
			want: "中文输出正常\n第二行\n",
		},
		{
			name:    "ansi color stripped",
			in:      "\x1b[31merror text\x1b[0m\n",
			want:    "error text\n",
			changed: true,
			ansi:    2,
		},
		{
			name:    "csi cursor control stripped before redraw",
			in:      "a\x1b[2K\rb\n",
			want:    "b\n",
			changed: true,
			ansi:    1,
			cr:      1,
		},
		{
			name:    "osc title stripped",
			in:      "\x1b]0;my title\x07prompt$\n",
			want:    "prompt$\n",
			changed: true,
			ansi:    1,
		},
		{
			name:    "two-byte escape dropped",
			in:      "a\x1bb\nc\n",
			want:    "a\nc\n",
			changed: true,
			ansi:    1,
		},
		{
			name:    "lone trailing esc dropped",
			in:      "ok\x1b",
			want:    "ok",
			changed: true,
			ansi:    1,
		},
		{
			name:    "crlf normalized",
			in:      "a\r\nb\r\n",
			want:    "a\nb\n",
			changed: true,
			cr:      2,
		},
		{
			name:    "progress bar folded to final frame",
			in:      "Downloading 1%\rDownloading 50%\rDownloading 100%\n",
			want:    "Downloading 100%\n",
			changed: true,
			cr:      2,
		},
		{
			name:    "error frame rescued",
			in:      "checking\rERROR: disk fail\rOK\n",
			want:    "ERROR: disk fail\nOK\n",
			changed: true,
			cr:      2,
			errKept: 1,
		},
		{
			name:    "failed keyword frame rescued",
			in:      "compiling\rbuild FAILED\rdone\n",
			want:    "build FAILED\ndone\n",
			changed: true,
			cr:      2,
			errKept: 1,
		},
		{
			name:    "trailing cr keeps last frame",
			in:      "abc\r",
			want:    "abc",
			changed: true,
			cr:      1,
		},
		{
			name:    "pure redraw noise vanishes",
			in:      "\r\r\r",
			want:    "",
			changed: true,
			cr:      3,
		},
		{
			name:    "adjacent duplicate folded",
			in:      "line1\nline1\nline2\n",
			want:    "line1\nline2\n",
			changed: true,
			dup:     1,
		},
		{
			name:    "duplicate run folded",
			in:      "same\nsame\nsame\nnext\n",
			want:    "same\nnext\n",
			changed: true,
			dup:     2,
		},
		{
			name: "blank lines never folded",
			in:   "\n\n\n",
			want: "\n\n\n",
		},
		{
			name: "duplicate separated by blank kept",
			in:   "a\n\na\n",
			want: "a\n\na\n",
		},
		{
			name: "status prefix preserved",
			in:   "[ok] build passed\n",
			want: "[ok] build passed\n",
		},
		{
			name:    "no trailing newline preserved",
			in:      "a\rbc",
			want:    "bc",
			changed: true,
			cr:      1,
		},
		{
			name:    "real-world mixed log",
			in:      "\x1b[1m\x1b[32m✓ build\x1b[0m\r\x1b[1m\x1b[31m✗ failed\x1b[0m\r\x1b[1m\x1b[32m✓ ok\x1b[0m\n",
			want:    "✗ failed\n✓ ok\n",
			changed: true,
			ansi:    9,
			cr:      2,
			errKept: 1,
		},
		{
			name: "invalid utf-8 passes through",
			in:   "\xff\xfe bad bytes\n",
			want: "\xff\xfe bad bytes\n",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rep := Clean(tt.in)
			if got != tt.want {
				t.Errorf("Clean() = %q, want %q", got, tt.want)
			}
			if rep.Changed != tt.changed {
				t.Errorf("Changed = %v, want %v", rep.Changed, tt.changed)
			}
			if rep.ANSIRemoved != tt.ansi {
				t.Errorf("ANSIRemoved = %d, want %d", rep.ANSIRemoved, tt.ansi)
			}
			if rep.CRFolded != tt.cr {
				t.Errorf("CRFolded = %d, want %d", rep.CRFolded, tt.cr)
			}
			if rep.DupLinesRemoved != tt.dup {
				t.Errorf("DupLinesRemoved = %d, want %d", rep.DupLinesRemoved, tt.dup)
			}
			if rep.ErrorLinesKept != tt.errKept {
				t.Errorf("ErrorLinesKept = %d, want %d", rep.ErrorLinesKept, tt.errKept)
			}
			if rep.InputBytes != len(tt.in) {
				t.Errorf("InputBytes = %d, want %d", rep.InputBytes, len(tt.in))
			}
			if rep.OutputBytes != len(got) {
				t.Errorf("OutputBytes = %d, want %d", rep.OutputBytes, len(got))
			}
			if rep.OutputBytes > rep.InputBytes {
				t.Errorf("amplified: %d -> %d", rep.InputBytes, rep.OutputBytes)
			}
			// Idempotence: cleaning already-cleaned output is a no-op.
			got2, rep2 := Clean(got)
			if got2 != got {
				t.Errorf("not idempotent: %q -> %q", got, got2)
			}
			if rep2.Changed {
				t.Errorf("second pass reported changes: %+v", rep2)
			}
		})
	}
}

func TestCleanDeterministic(t *testing.T) {
	in := "\x1b[31merr\x1b[0m\nx\r\r100%\nx\nx\n"
	out1, rep1 := Clean(in)
	out2, rep2 := Clean(in)
	if out1 != out2 || rep1 != rep2 {
		t.Errorf("non-deterministic: %q/%+v vs %q/%+v", out1, rep1, out2, rep2)
	}
}

// noiseFreeSample builds n distinct lines (adjacent-line folding must not
// trigger on it) for the zero-alloc contract tests.
func noiseFreeSample(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("[ok] plain line of output with some text #")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TestCleanZeroAllocNoiseFree pins the zero-copy contract: noise-free
// input must come back as the original string without a single allocation.
func TestCleanZeroAllocNoiseFree(t *testing.T) {
	sample := noiseFreeSample(200) // ~8.6KB
	allocs := testing.AllocsPerRun(200, func() {
		Clean(sample)
	})
	if allocs != 0 {
		t.Errorf("noise-free Clean allocated %v times per run, want 0", allocs)
	}
}

func BenchmarkClean(b *testing.B) {
	samples := map[string]string{
		"noise-free-8KB": noiseFreeSample(200),
		"noise-free-1MB": noiseFreeSample(1 << 20 / 46),
		"ansi-heavy":     strings.Repeat("\x1b[32mcontent line with color\x1b[0m more text follows\n", 20000),
		"cr-progress":    strings.Repeat("frame 10%\rframe 20%\rframe 30%\rframe done\n", 20000),
		"dup-lines":      strings.Repeat("same line repeated\n", 100000),
		"mixed-real-log": strings.Repeat("\x1b[1mcompiling\x1b[0m\rERROR: retry\rstep ok\nsame\nsame\n", 10000),
	}
	for name, s := range samples {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(s)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				Clean(s)
			}
		})
	}
}
