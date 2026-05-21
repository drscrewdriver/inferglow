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

package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/inferglow/action"
)

// fakeBashExecutor is a test double for BashExecutor.
type fakeBashExecutor struct {
	result *BashExecutionResult
	err    error
	gotReq BashExecutionRequest
}

func (f *fakeBashExecutor) Execute(ctx context.Context, req BashExecutionRequest) (*BashExecutionResult, error) {
	f.gotReq = req
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return &BashExecutionResult{ExitCode: 0, Stdout: "ok"}, nil
	}
	return f.result, nil
}

func TestBashExecutorSpec(t *testing.T) {
	if BashExecutorSpec.SideEffectLevel != action.SideEffectExec {
		t.Errorf("SideEffectLevel = %q, want %q", BashExecutorSpec.SideEffectLevel, action.SideEffectExec)
	}
	if !BashExecutorSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = false, want true")
	}
	if !BashExecutorSpec.SandboxRequired {
		t.Errorf("SandboxRequired = false, want true")
	}
}

func TestBashExecutorSuccess(t *testing.T) {
	fake := &fakeBashExecutor{
		result: &BashExecutionResult{ExitCode: 0, Stdout: "hello\n"},
	}
	a := NewBashExecutorAction(fake)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want success", res.Status)
	}
	out, ok := res.Result.(*BashExecutionResult)
	if !ok {
		t.Fatalf("Result not *BashExecutionResult: %T", res.Result)
	}
	if out.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", out.Stdout, "hello\n")
	}
	if fake.gotReq.Command != "echo hello" {
		t.Errorf("got command = %q", fake.gotReq.Command)
	}
}

func TestBashExecutorNonZeroExit(t *testing.T) {
	fake := &fakeBashExecutor{
		result: &BashExecutionResult{ExitCode: 127, Stderr: "command not found"},
	}
	a := NewBashExecutorAction(fake)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"command": "nope",
	})
	if res.OK {
		t.Errorf("expected OK=false for non-zero exit")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
}

func TestBashExecutorMissingCommand(t *testing.T) {
	a := NewBashExecutorAction(&fakeBashExecutor{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing command")
	}
	if res.Error != "bash_executor: command is required" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestBashExecutorNilExecutor(t *testing.T) {
	a := NewBashExecutorAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"command": "ls",
	})
	if res.OK {
		t.Errorf("expected OK=false for nil executor")
	}
	if res.Error != "bash_executor: no executor injected" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestBashExecutorProviderError(t *testing.T) {
	a := NewBashExecutorAction(&fakeBashExecutor{err: errors.New("sandbox down")})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"command": "ls",
	})
	if res.OK {
		t.Errorf("expected OK=false when executor errors")
	}
	if res.Error != "bash_executor: sandbox down" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestBashExecutorBadInputType(t *testing.T) {
	a := NewBashExecutorAction(&fakeBashExecutor{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"command": 123,
	})
	if res.OK {
		t.Errorf("expected OK=false for non-string command")
	}
}

func TestBashExecutorEnvPassthrough(t *testing.T) {
	fake := &fakeBashExecutor{}
	a := NewBashExecutorAction(fake)
	_, _ = a.Executor.Execute(context.Background(), map[string]any{
		"command": "env",
		"env": map[string]any{
			"FOO": "bar",
			"BAZ": "qux",
		},
		"workdir": "/tmp",
		"stdin":   "input",
		"timeout": "5s",
	})
	if fake.gotReq.Workdir != "/tmp" {
		t.Errorf("Workdir = %q, want /tmp", fake.gotReq.Workdir)
	}
	if fake.gotReq.Stdin != "input" {
		t.Errorf("Stdin = %q, want input", fake.gotReq.Stdin)
	}
	if fake.gotReq.Timeout != "5s" {
		t.Errorf("Timeout = %q, want 5s", fake.gotReq.Timeout)
	}
	if fake.gotReq.Env["FOO"] != "bar" || fake.gotReq.Env["BAZ"] != "qux" {
		t.Errorf("Env = %+v, want FOO=bar BAZ=qux", fake.gotReq.Env)
	}
}

func TestBashExecutorActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewBashExecutorAction(&fakeBashExecutor{})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(BashExecutorActionID) {
		t.Errorf("registry missing %q", BashExecutorActionID)
	}
}
