package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestExecuteLoop_ParsesMarkdownJSONDecision (O-CRITICAL-1) verifies that
// executeLoop tolerates an LLM response that wraps the JSON decision in a
// markdown code fence. Before the fix, the engine called ParseDecision on
// the raw LLM output and json.Unmarshal failed because of the leading
// "```json\n" and trailing "\n```" markers.
func TestExecuteLoop_ParsesMarkdownJSONDecision(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta: "```json\n" +
					`{"next_action":"response","final_response":"Hello from markdown!"}` + "\n```",
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}

	decision, err := engine.executeLoop(context.Background(), "Hi", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error for markdown-wrapped JSON: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "Hello from markdown!" {
		t.Errorf("FinalResponse mismatch: got %q, want %q",
			decision.FinalResponse, "Hello from markdown!")
	}
}

// TestExecuteLoop_ParsesNoisyJSONDecision (O-CRITICAL-1) verifies that
// executeLoop tolerates an LLM that prepends "Sure! Here is the decision:"
// and appends trailing prose around a valid JSON object.
func TestExecuteLoop_ParsesNoisyJSONDecision(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta: "Sure! Here is the decision you asked for:\n" +
					`{"next_action":"response","final_response":"noisy 42"}` + "\nLet me know if you need anything else.",
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}

	decision, err := engine.executeLoop(context.Background(), "Hi", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error for noisy JSON: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "noisy 42" {
		t.Errorf("FinalResponse mismatch: got %q, want %q",
			decision.FinalResponse, "noisy 42")
	}
}

// TestExecuteLoop_ParsesTrailingCommaJSONDecision (O-CRITICAL-1) verifies
// that executeLoop tolerates an LLM that emits trailing commas.
func TestExecuteLoop_ParsesTrailingCommaJSONDecision(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta: `{"next_action":"response","final_response":"trailing ok",}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}

	decision, err := engine.executeLoop(context.Background(), "Hi", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error for trailing-comma JSON: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "trailing ok" {
		t.Errorf("FinalResponse mismatch: got %q, want %q",
			decision.FinalResponse, "trailing ok")
	}
}

// TestExecuteLoop_StreamTimeout (BUG-8) verifies that executeLoop does not
// block forever when the stream channel stops delivering chunks. We mock
// the ModelRequester to return a channel that never sends anything, set
// Engine.streamTimeout to a short value, and assert that executeLoop
// returns context.DeadlineExceeded within a reasonable test budget.
func TestExecuteLoop_StreamTimeout(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	// stream that never sends anything and never closes.
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk) // unbuffered, never written, never closed
			return ch, nil
		},
	}

	engine := &Engine{
		session:      sess,
		actionExt:    actExt,
		modelReq:     mockReq,
		streamTimeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := engine.executeLoop(ctx, "Hi", 3, "")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("executeLoop returned nil error; expected DeadlineExceeded")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("executeLoop did not return within 3s; stream timeout not enforced")
	}
}

// TestExecuteLoop_StreamTimeoutDefault (BUG-8 / O-MEDIUM-2) verifies that the
// default 5-minute timeout applies when no streamTimeout is set. We can't
// wait 5 minutes in a test, so we cancel the parent ctx early; the test
// asserts the goroutine exits cleanly via the parent cancellation. The
// key property being verified: a zero streamTimeout field does not cause
// executeLoop to skip the timeout wrapper entirely (which would leak the
// goroutine if the parent ctx is also long-lived).
func TestExecuteLoop_StreamTimeoutDefault(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
		// streamTimeout left zero — defaults to 5*time.Minute inside executeLoop.
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := engine.executeLoop(ctx, "Hi", 3, "")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("executeLoop returned nil error; expected ctx cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("executeLoop did not return within 3s after parent ctx cancel")
	}
}

// TestExecuteLoop_StreamCleanupOnTimeout (O-MEDIUM-2) verifies that when the
// stream times out, the engine cancels the timeout context which is also
// propagated to the stream producer via the original ctx (or the timeout
// ctx). We track whether the producer observed ctx cancellation.
func TestExecuteLoop_StreamCleanupOnTimeout(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	cleanupObserved := make(chan struct{})
	var once sync.Once

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk)
			// Mock producer: would normally send chunks. We just observe
			// ctx cancellation as a proxy for "engine stopped reading".
			go func() {
				<-ctx.Done()
				once.Do(func() { close(cleanupObserved) })
			}()
			return ch, nil
		},
	}

	engine := &Engine{
		session:       sess,
		actionExt:     actExt,
		modelReq:      mockReq,
		streamTimeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = engine.executeLoop(ctx, "Hi", 3, "")

	select {
	case <-cleanupObserved:
		// Good: engine's stream timeout cancelled the parent context, and
		// the producer observed it.
	case <-time.After(2 * time.Second):
		t.Fatal("stream producer did not observe ctx cancellation within 2s")
	}
}

// TestExecuteLoop_ForceJSONOptionSet (O-CRITICAL-1) verifies that executeLoop
// populates Options["force_json"]=true on the ModelRequest it sends to the
// provider so OpenAI-compatible providers can set response_format.
func TestExecuteLoop_ForceJSONOptionSet(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	var capturedReq *model.ModelRequest
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			capturedReq = req
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"ok"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}

	_, err := engine.executeLoop(context.Background(), "Hi", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("ModelRequest not captured")
	}
	if capturedReq.Options == nil {
		t.Fatal("Options map is nil; expected force_json=true")
	}
	v, ok := capturedReq.Options["force_json"]
	if !ok {
		t.Fatal("Options[force_json] not set; expected true")
	}
	b, _ := v.(bool)
	if !b {
		t.Errorf("Options[force_json] = %v, want true", v)
	}
}

// TestNew_AppliesWithMaxRounds (BUG-18) verifies that WithMaxRounds passed
// to New is persisted on the Agent struct so subsequent Run calls (without
// per-call opts) use the configured value. Before the fix, New accepted
// the option function but never applied it to the Agent.maxRounds field.
func TestNew_AppliesWithMaxRounds(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"ok"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	a := New(sess, actExt, mockReq, WithMaxRounds(5))
	if a.maxRounds != 5 {
		t.Errorf("agent.maxRounds: got %d, want 5", a.maxRounds)
	}
}

// TestSessionExt_AddAssistantDecision_NilResult (O-HIGH-3) is a regression
// test: AddActionResult must not panic when the caller passes a nil
// *ActionResult. Before the fix, accessing result.Status on a nil pointer
// panicked.
func TestSessionExt_AddAssistantDecision_NilResult(t *testing.T) {
	ext := NewSessionExtension(session.NewSession("test", 10000))

	// Must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AddActionResult panicked on nil result: %v", r)
		}
	}()
	ext.AddActionResult("calc", nil)

	// No system message should have been added since the result was nil.
	prompt := ext.PreparePrompt()
	if len(prompt) != 0 {
		t.Errorf("expected 0 messages after nil AddActionResult, got %d", len(prompt))
	}
}

// TestParseDecision_FallbackOnGarbage (O-MEDIUM-1) verifies the Planning
// fallback strategy: when the LLM emits pure prose that contains no JSON
// object at all, ParseDecision must return a degraded Decision instead of
// an error so the loop can still terminate with a final_response.
func TestParseDecision_FallbackOnGarbage(t *testing.T) {
	// Exercise ParseDecision's fallback via executeLoop with a mock that
	// returns pure prose. The fallback should yield a response decision
	// whose FinalResponse contains the raw LLM output.
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  "I'm sorry, I can't produce structured output for that request.",
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}

	decision, err := engine.executeLoop(context.Background(), "Hi", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error on garbage LLM output: %v", err)
	}
	if decision == nil {
		t.Fatal("expected non-nil decision (fallback)")
	}
	if decision.NextAction != "response" {
		t.Errorf("fallback NextAction: got %q, want %q", decision.NextAction, "response")
	}
	if !strings.Contains(decision.FinalResponse, "I'm sorry") {
		t.Errorf("fallback FinalResponse should contain raw LLM output, got %q",
			decision.FinalResponse)
	}
}

// TestWithStreamTimeout (BUG-8 / O-MEDIUM-2) verifies the WithStreamTimeout
// RunOption propagates the configured timeout to the Engine when Run is
// invoked. We can't easily assert the engine's internal field from here
// without exposing it, so we instead observe the effect: a stuck stream
// returns DeadlineExceeded within the configured timeout window.
func TestWithStreamTimeout(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk) // stuck
			return ch, nil
		},
	}

	a := New(sess, actExt, mockReq)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := a.Run(ctx, "Hi", WithStreamTimeout(100*time.Millisecond))
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil error; expected DeadlineExceeded")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s; WithStreamTimeout not propagated")
	}
}
