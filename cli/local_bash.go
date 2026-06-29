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
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/inferglow/builtins/actions"
)

// localBashRunner implements actions.BashExecutor for local shell execution.
type localBashRunner struct {
	workdir string
	unsafe  bool
	timeout time.Duration
}

// Execute runs the command in a local shell with timeout and workdir restrictions.
func (r *localBashRunner) Execute(ctx context.Context, req actions.BashExecutionRequest) (*actions.BashExecutionResult, error) {
	if req.Command == "" {
		return nil, fmt.Errorf("bash_executor: command is required")
	}

	timeout := r.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workdir := r.workdir
	if req.Workdir != "" {
		workdir = req.Workdir
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", req.Command)
	cmd.Dir = workdir

	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	// Set environment.
	if len(req.Env) > 0 {
		cmd.Env = make([]string, 0, len(req.Env))
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &actions.BashExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result, nil
}

// localGrepRunner implements actions.GrepRunner using the system grep command.
type localGrepRunner struct {
	workdir string
}

// Run executes a grep search using the system's grep command.
func (r *localGrepRunner) Run(ctx context.Context, req actions.GrepRequest) ([]actions.GrepMatch, error) {
	if req.Pattern == "" {
		return nil, fmt.Errorf("grep: pattern is required")
	}
	if req.Path == "" {
		return nil, fmt.Errorf("grep: path is required")
	}

	args := []string{"-n", "--no-messages"}
	if req.Recursive {
		args = append(args, "-r")
	}
	args = append(args, req.Pattern, req.Path)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "grep", args...)
	cmd.Dir = r.workdir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// grep returns exit code 1 when no matches found — not an error.
	_ = cmd.Run()

	var matches []actions.GrepMatch
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Parse grep -n output: file:line:content
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		lineNum := 0
		fmt.Sscanf(parts[1], "%d", &lineNum)
		matches = append(matches, actions.GrepMatch{
			File:    parts[0],
			Line:    lineNum,
			Content: parts[2],
		})
		if req.MaxResult > 0 && len(matches) >= req.MaxResult {
			break
		}
	}

	return matches, nil
}
