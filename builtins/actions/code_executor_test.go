package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/inferglow/action"
)

// fakeCodeExecutor is a test double for CodeExecutor.
type fakeCodeExecutor struct {
	result *CodeExecutionResult
	err    error
	gotReq CodeExecutionRequest
}

func (f *fakeCodeExecutor) Execute(ctx context.Context, req CodeExecutionRequest) (*CodeExecutionResult, error) {
	f.gotReq = req
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return &CodeExecutionResult{Language: req.Language, ExitCode: 0, Stdout: "ok"}, nil
	}
	return f.result, nil
}

func TestCodeExecutorSpec(t *testing.T) {
	if CodeExecutorSpec.SideEffectLevel != action.SideEffectExec {
		t.Errorf("SideEffectLevel = %q, want %q", CodeExecutorSpec.SideEffectLevel, action.SideEffectExec)
	}
	if !CodeExecutorSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = false, want true")
	}
	if !CodeExecutorSpec.SandboxRequired {
		t.Errorf("SandboxRequired = false, want true")
	}
}

func TestCodeExecutorSuccess(t *testing.T) {
	fake := &fakeCodeExecutor{
		result: &CodeExecutionResult{Language: "python", ExitCode: 0, Stdout: "42\n"},
	}
	a := NewCodeExecutorAction(fake)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"language": "python",
		"source":   "print(6*7)",
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
	out, ok := res.Result.(*CodeExecutionResult)
	if !ok {
		t.Fatalf("Result not *CodeExecutionResult: %T", res.Result)
	}
	if out.Stdout != "42\n" {
		t.Errorf("Stdout = %q, want %q", out.Stdout, "42\n")
	}
	if fake.gotReq.Language != "python" {
		t.Errorf("got language = %q", fake.gotReq.Language)
	}
	if fake.gotReq.Source != "print(6*7)" {
		t.Errorf("got source = %q", fake.gotReq.Source)
	}
}

func TestCodeExecutorNonZeroExit(t *testing.T) {
	fake := &fakeCodeExecutor{
		result: &CodeExecutionResult{ExitCode: 1, Stderr: "boom"},
	}
	a := NewCodeExecutorAction(fake)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"language": "go",
		"source":   "panic(\"x\")",
	})
	if res.OK {
		t.Errorf("expected OK=false for non-zero exit")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
}

func TestCodeExecutorMissingLanguage(t *testing.T) {
	a := NewCodeExecutorAction(&fakeCodeExecutor{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"source": "x",
	})
	if res.OK {
		t.Errorf("expected OK=false for missing language")
	}
	if res.Error != "code_executor: language is required" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestCodeExecutorMissingSource(t *testing.T) {
	a := NewCodeExecutorAction(&fakeCodeExecutor{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"language": "python",
	})
	if res.OK {
		t.Errorf("expected OK=false for missing source")
	}
}

func TestCodeExecutorNilExecutor(t *testing.T) {
	a := NewCodeExecutorAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"language": "python",
		"source":   "print(1)",
	})
	if res.OK {
		t.Errorf("expected OK=false for nil executor")
	}
	if res.Error != "code_executor: no executor injected" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestCodeExecutorProviderError(t *testing.T) {
	a := NewCodeExecutorAction(&fakeCodeExecutor{err: errors.New("sandbox unavailable")})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"language": "python",
		"source":   "print(1)",
	})
	if res.OK {
		t.Errorf("expected OK=false when executor errors")
	}
	if res.Error != "code_executor: sandbox unavailable" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestCodeExecutorBadInputType(t *testing.T) {
	a := NewCodeExecutorAction(&fakeCodeExecutor{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"language": 123,
		"source":   "x",
	})
	if res.OK {
		t.Errorf("expected OK=false for non-string language")
	}
}

func TestCodeExecutorActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewCodeExecutorAction(&fakeCodeExecutor{})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(CodeExecutorActionID) {
		t.Errorf("registry missing %q", CodeExecutorActionID)
	}
}
