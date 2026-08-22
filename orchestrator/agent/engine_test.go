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

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// mockModelRequester is a test double for model.ModelRequester
type mockModelRequester struct {
	responseFn func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error)
	nameFn     func() string
	requestFn  func(ctx context.Context, req *model.ModelRequest)
}

func (m *mockModelRequester) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return "mock"
}

func (m *mockModelRequester) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	if m.requestFn != nil {
		m.requestFn(ctx, req)
	}
	return &model.RequestData{
		Model: req.Model,
	}, nil
}

func (m *mockModelRequester) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	if m.responseFn != nil {
		return m.responseFn(ctx, data)
	}
	return nil, nil
}

func (m *mockModelRequester) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

func makeStreamChunk(content string, isDone bool) *model.StreamChunk {
	return &model.StreamChunk{Delta: content, IsDone: isDone}
}

func TestEngine_PreemptReturnsErrTurnInterrupted(t *testing.T) {
	// A preempted turn must surface ErrTurnInterrupted (via errors.Is) so
	// callers can distinguish steering from real failures.
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- makeStreamChunk(`{"next_action":"response","final_response":"ok"}`, true)
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}
	cm := NewCancelManager(nil) // no turn loop: CancelImmediate still fires at Point 1
	engine.cancelManager = cm

	var runErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, runErr = engine.executeLoop(context.Background(), "hi", 3, "")
	}()
	// Issue an immediate cancel (steer now) right after the run starts.
	cm.Cancel(CancelImmediate, WithReason("test steer now"))
	<-done

	if !errors.Is(runErr, ErrTurnInterrupted) {
		t.Fatalf("executeLoop error = %v, want ErrTurnInterrupted", runErr)
	}
}

func TestEngine_PreemptDrainContinuesNextInput(t *testing.T) {
	// When a preempt fires with a queued input, the engine must drain the
	// next highest-priority request, continue with it, and deliver the
	// response on its ResponseCh (no leak, no stall).
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()
	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			callCount++
			ch := make(chan *model.StreamChunk, 1)
			ch <- makeStreamChunk(`{"next_action":"response","final_response":"ok"}`, true)
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}
	cm := NewCancelManager(nil)
	engine.cancelManager = cm
	q := NewInputQueue(4)
	engine.inputQueue = q

	// Pre-queue a force input; its cancel preempts the initial turn at
	// Point 1, and preemptDrainNext must continue with it.
	respCh := make(chan InputResponse, 1)
	if err := q.Enqueue(InputRequest{
		Message:    "steered",
		Mode:       PreemptForce,
		Ctx:        context.Background(),
		ResponseCh: respCh,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	cm.Cancel(CancelImmediate, WithReason("steer now"))

	// RunLoop (not executeLoop) is required: the pendingInterleave response
	// is delivered by RunLoop after executeLoop returns.
	finalResponse, err := engine.RunLoop(context.Background(), "initial", 3, "")
	if err != nil {
		t.Fatalf("RunLoop error: %v (drain should continue with the queued input)", err)
	}
	if callCount != 1 {
		t.Errorf("LLM calls = %d, want 1 (initial turn preempted, drained steer turn only)", callCount)
	}
	if finalResponse != "ok" {
		t.Errorf("FinalResponse = %q, want ok", finalResponse)
	}

	// The drained request must have received its response.
	select {
	case resp := <-respCh:
		if resp.Error != nil {
			t.Errorf("drained response error: %v", resp.Error)
		}
		if resp.Response != "ok" {
			t.Errorf("drained response = %q, want ok", resp.Response)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("drained request got no response (ResponseCh leak)")
	}
}

// TestEngine_DirectResponse tests the basic response path.
func TestEngine_DirectResponse(t *testing.T) {
	// Test when LLM directly returns a response without executing actions
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"Hello!"}`,
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
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "Hello!" {
		t.Errorf("FinalResponse mismatch: got %q", decision.FinalResponse)
	}
}

func TestEngine_ExecuteThenResponse(t *testing.T) {
	// Test: LLM executes an action, then responds
	sess := NewSessionExtension(session.NewSession("test", 10000))

	actionInst, _ := action.New("calc", "calc",
		func(ctx context.Context, input map[string]any) (any, error) {
			return 42, nil
		})
	actExt := NewActionExtension()
	actExt.Register(actionInst)

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			if callCount == 1 {
				// First call: LLM decides to execute calc
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"execute","action_calls":[{"name":"calc","params":{}}]}`,
					IsDone: true,
				}
			} else {
				// Second call: LLM responds
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"response","final_response":"Result is 42"}`,
					IsDone: true,
				}
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

	decision, err := engine.executeLoop(context.Background(), "What is 21*2?", 5, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "Result is 42" {
		t.Errorf("FinalResponse: got %q", decision.FinalResponse)
	}
}

func TestEngine_ToolCallCap(t *testing.T) {
	// Test that loop terminates when tool-call cap is reached.
	// Use a small custom cap to keep the test fast.
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			// Vary the params each round so the stale-dedup detector never
			// fires; this isolates the tool-call cap (maxToolCallRounds=5)
			// path from the stale→synthesis path.
			delta := `{"next_action":"execute","action_calls":[{"name":"noop","params":{"round":` +
				strconv.Itoa(callCount) + `}}]}`
			ch <- &model.StreamChunk{Delta: delta, IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:           sess,
		actionExt:         actExt,
		modelReq:          mockReq,
		maxToolCallRounds: 5, // small cap for fast test
	}

	decision, err := engine.executeLoop(context.Background(), "test", 0, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "execute" {
		t.Error("Expected execute (loop was forced to stop by tool cap)")
	}
	// maxToolCallRounds=5 means the loop runs 5 tool rounds
	if callCount != 5 {
		t.Errorf("Expected 5 calls (tool cap), got %d", callCount)
	}
}

// TestEngine_BuildToolDefinitionsReturnsValidTools is a regression test for
// O-HIGH-1: buildToolDefinitions previously used direct type assertions
// (a["name"].(string)) which would panic if the map lacked a key or held
// the wrong type. The comma-ok fix should produce well-formed ToolDefinition
// values for normal Action registrations (including actions with empty
// descriptions and nil schemas).
func TestEngine_BuildToolDefinitionsReturnsValidTools(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	// Action with both description and schema populated.
	withSchema, _ := action.New("with_schema", "described action",
		func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	withSchema.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
	}
	// Action with empty description and nil schema — must not panic on
	// either type assertion under the new comma-ok code path.
	emptyDesc, _ := action.New("empty_desc", "",
		func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })

	if err := actExt.Register(withSchema); err != nil {
		t.Fatalf("Register with_schema: %v", err)
	}
	if err := actExt.Register(emptyDesc); err != nil {
		t.Fatalf("Register empty_desc: %v", err)
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  nil, // buildToolDefinitions does not touch the model
	}

	tools := engine.buildToolDefinitions()
	if len(tools) != 2 {
		t.Fatalf("Expected 2 tools, got %d", len(tools))
	}

	byName := map[string]model.ToolDefinition{}
	for _, td := range tools {
		byName[td.Name] = td
	}
	if td, ok := byName["with_schema"]; !ok {
		t.Errorf("Expected tool 'with_schema' in result, got %v", tools)
	} else {
		if td.Description != "described action" {
			t.Errorf("with_schema.Description = %q, want %q", td.Description, "described action")
		}
		if td.Parameters == nil {
			t.Error("with_schema.Parameters should not be nil")
		}
	}
	if td, ok := byName["empty_desc"]; !ok {
		t.Errorf("Expected tool 'empty_desc' in result, got %v", tools)
	} else {
		if td.Description != "" {
			t.Errorf("empty_desc.Description = %q, want empty", td.Description)
		}
		// nil schema coerces to nil map under comma-ok; the model layer
		// tolerates a nil Parameters field.
	}
}

// TestEngine_BuildToolDefinitionsSortedByName verifies the returned slice is
// sorted by Name (a hard requirement for byte-stable serialization / prefix
// cache hits).
func TestEngine_BuildToolDefinitionsSortedByName(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	// Register actions in non-alphabetical order.
	zAction, _ := action.New("zeta", "z", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	aAction, _ := action.New("alpha", "a", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	mAction, _ := action.New("mid", "m", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })

	if err := actExt.Register(zAction); err != nil {
		t.Fatalf("Register zeta: %v", err)
	}
	if err := actExt.Register(aAction); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}
	if err := actExt.Register(mAction); err != nil {
		t.Fatalf("Register mid: %v", err)
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
	}
	tools := engine.buildToolDefinitions()
	if len(tools) != 3 {
		t.Fatalf("Expected 3 tools, got %d", len(tools))
	}
	wantOrder := []string{"alpha", "mid", "zeta"}
	for i, w := range wantOrder {
		if tools[i].Name != w {
			t.Errorf("tools[%d].Name = %q, want %q", i, tools[i].Name, w)
		}
	}
}

// TestEngine_ToolDefsHashStable verifies that calling buildToolDefinitions
// multiple times produces the same toolDefsHash (since the tools haven't
// changed). This is the core prefix-cache stability guarantee.
func TestEngine_ToolDefsHashStable(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	aAction, _ := action.New("alpha", "a", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	aAction.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
			"y": map[string]any{"type": "string"},
		},
	}
	bAction, _ := action.New("beta", "b", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	bAction.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"q": map[string]any{"type": "boolean"},
		},
	}

	if err := actExt.Register(aAction); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}
	if err := actExt.Register(bAction); err != nil {
		t.Fatalf("Register beta: %v", err)
	}

	engine := &Engine{session: sess, actionExt: actExt}

	// First call
	engine.buildToolDefinitions()
	firstHash := engine.ToolDefsHash()
	if firstHash == "" {
		t.Fatal("ToolDefsHash should not be empty after buildToolDefinitions")
	}

	// Subsequent calls should produce the same hash
	for i := 0; i < 20; i++ {
		engine.buildToolDefinitions()
		h := engine.ToolDefsHash()
		if h != firstHash {
			t.Fatalf("iter %d: ToolDefsHash changed\nfirst: %s\ncurr:  %s", i, firstHash, h)
		}
	}
}

// TestEngine_ToolDefsHashIdenticalAcrossEngines verifies that two engines
// with the same registered actions produce the same toolDefsHash. This is
// the property that lets two sessions share a Zone 1 prefix cache.
func TestEngine_ToolDefsHashIdenticalAcrossEngines(t *testing.T) {
	mkEngine := func() *Engine {
		sess := NewSessionExtension(session.NewSession("test", 10000))
		actExt := NewActionExtension()
		a, _ := action.New("calc", "calculator", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
		a.Schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expr": map[string]any{"type": "string"},
			},
		}
		b, _ := action.New("search", "web search", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
		b.Schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		}
		// Register in different orders to verify registration order doesn't
		// affect the hash.
		_ = actExt.Register(b)
		_ = actExt.Register(a)
		return &Engine{session: sess, actionExt: actExt}
	}

	e1 := mkEngine()
	e2 := mkEngine()
	// Register in opposite order in e2 to verify order-independence.
	e2.actionExt = NewActionExtension()
	a, _ := action.New("calc", "calculator", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	a.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expr": map[string]any{"type": "string"},
		},
	}
	b, _ := action.New("search", "web search", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	b.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
	_ = e2.actionExt.Register(a)
	_ = e2.actionExt.Register(b)

	e1.buildToolDefinitions()
	e2.buildToolDefinitions()
	if e1.ToolDefsHash() != e2.ToolDefsHash() {
		t.Errorf("engines with same tools should have identical hash\ne1: %s\ne2: %s",
			e1.ToolDefsHash(), e2.ToolDefsHash())
	}
}

// TestEngine_ToolDefsHashChangesWhenToolsChange verifies the hash changes
// when the tool set changes (so callers can detect cache invalidation).
func TestEngine_ToolDefsHashChangesWhenToolsChange(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	a, _ := action.New("alpha", "a", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	if err := actExt.Register(a); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}

	engine := &Engine{session: sess, actionExt: actExt}
	engine.buildToolDefinitions()
	hash1 := engine.ToolDefsHash()

	// Register a second tool — registry mutation requires a new action name.
	b, _ := action.New("beta", "b", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	// actExt's registry was already constructed; use SetRegistry to swap in a
	// fresh registry that contains both actions.
	newReg := actExt.GetRegistry() // same registry — actions ARE appended on Register
	_ = newReg
	if err := actExt.Register(b); err != nil {
		t.Fatalf("Register beta: %v", err)
	}

	engine.buildToolDefinitions()
	hash2 := engine.ToolDefsHash()
	if hash1 == hash2 {
		t.Errorf("hash should change when tool set changes, both = %s", hash1)
	}
}

// ---------------------------------------------------------------------------
// formatToolResult tests
// ---------------------------------------------------------------------------

// fileReadResult mirrors builtins/actions.FileReadResult so we can test
// formatToolResult without importing the builtins package (which would
// create a circular dependency risk).
type fileReadResult struct {
	Path      string `json:"path"`
	BytesRead int64  `json:"bytes_read"`
	Content   string `json:"content"`
}

type fileWriteResult struct {
	Path         string `json:"path"`
	BytesWritten int64  `json:"bytes_written"`
}

func TestFormatToolResult_FileWrite(t *testing.T) {
	ar := &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: fileWriteResult{Path: "/tmp/hello.go", BytesWritten: 42},
	}
	got := formatToolResult(ar)

	var parsed fileWriteResult
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("formatToolResult output is not valid JSON: %v\noutput: %s", err, got)
	}
	if parsed.Path != "/tmp/hello.go" {
		t.Errorf("Path = %q, want /tmp/hello.go", parsed.Path)
	}
	if parsed.BytesWritten != 42 {
		t.Errorf("BytesWritten = %d, want 42", parsed.BytesWritten)
	}
}

func TestFormatToolResult_FileRead(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	ar := &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: fileReadResult{Path: "/tmp/hello.go", BytesRead: int64(len(content)), Content: content},
	}
	got := formatToolResult(ar)

	var parsed fileReadResult
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("formatToolResult output is not valid JSON: %v\noutput: %s", err, got)
	}
	if parsed.Path != "/tmp/hello.go" {
		t.Errorf("Path = %q, want /tmp/hello.go", parsed.Path)
	}
	if parsed.Content != content {
		t.Errorf("Content mismatch: got %q, want %q", parsed.Content, content)
	}
	if parsed.BytesRead != int64(len(content)) {
		t.Errorf("BytesRead = %d, want %d", parsed.BytesRead, len(content))
	}
}

func TestFormatToolResult_Error(t *testing.T) {
	ar := &action.ActionResult{OK: false, Status: "error", Error: "file not found"}
	got := formatToolResult(ar)
	if got != "error: file not found" {
		t.Errorf("got %q, want %q", got, "error: file not found")
	}
}

func TestFormatToolResult_Nil(t *testing.T) {
	got := formatToolResult(nil)
	if got != "null" {
		t.Errorf("got %q, want %q", got, "null")
	}
}

// ---------------------------------------------------------------------------
// E2E: file_write → tool result → session → file_read → tool result → response
// ---------------------------------------------------------------------------

func TestEngine_FileWriteThenRead_E2E(t *testing.T) {
	// Create a temp directory for file operations.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.txt")
	expectedContent := "hello from agent test"

	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	// Register real file_write and file_read actions.
	writeAction, err := action.New("file_write", "Write content to a file.", func(ctx context.Context, input map[string]any) (any, error) {
		path, _ := input["path"].(string)
		content, _ := input["content"].(string)
		if e := os.WriteFile(path, []byte(content), 0o644); e != nil {
			return nil, e
		}
		return fileWriteResult{Path: path, BytesWritten: int64(len(content))}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	writeAction.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
		},
		"required": []string{"path", "content"},
	}

	readAction, err := action.New("file_read", "Read a file.", func(ctx context.Context, input map[string]any) (any, error) {
		path, _ := input["path"].(string)
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		return fileReadResult{Path: path, BytesRead: int64(len(data)), Content: string(data)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	readAction.Schema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []string{"path"},
	}

	if err := actExt.Register(writeAction); err != nil {
		t.Fatal(err)
	}
	if err := actExt.Register(readAction); err != nil {
		t.Fatal(err)
	}

	// Mock model: 3 phases
	//   Call 1: file_write tool call
	//   Call 2: file_read tool call
	//   Call 3: final response with the content
	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			switch callCount {
			case 1:
				// file_write
				ch <- &model.StreamChunk{
					Delta: "",
					Tools: []model.ToolCall{
						{
							ID:        "call_write_1",
							Name:      "file_write",
							Arguments: map[string]any{"path": testFile, "content": expectedContent},
						},
					},
					IsDone: true,
				}
			case 2:
				// file_read
				ch <- &model.StreamChunk{
					Delta: "",
					Tools: []model.ToolCall{
						{
							ID:        "call_read_1",
							Name:      "file_read",
							Arguments: map[string]any{"path": testFile},
						},
					},
					IsDone: true,
				}
			default:
				// final response
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"response","final_response":"done"}`,
					IsDone: true,
				}
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

	decision, err := engine.executeLoop(context.Background(), "write then read", 10, "")
	if err != nil {
		t.Fatalf("executeLoop error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Fatalf("expected response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "done" {
		t.Errorf("FinalResponse = %q, want %q", decision.FinalResponse, "done")
	}

	// Verify the file was actually written.
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != expectedContent {
		t.Errorf("file content = %q, want %q", string(data), expectedContent)
	}

	// Verify session history contains proper tool result messages.
	msgs := sess.PreparePrompt()
	// Expected messages: system(none) + user + assistant(tool_calls:write) + tool(write_result) + assistant(tool_calls:read) + tool(read_result) + assistant(response)
	var toolMsgs []model.ChatMessage
	var assistantMsgs []model.ChatMessage
	for _, m := range msgs {
		if m.Role == "tool" {
			toolMsgs = append(toolMsgs, m)
		}
		if m.Role == "assistant" {
			assistantMsgs = append(assistantMsgs, m)
		}
	}

	if len(toolMsgs) != 2 {
		t.Fatalf("expected 2 tool result messages, got %d", len(toolMsgs))
	}
	// First tool result: file_write
	if toolMsgs[0].ToolCallID != "call_write_1" {
		t.Errorf("tool[0].ToolCallID = %q, want call_write_1", toolMsgs[0].ToolCallID)
	}
	var writeRes fileWriteResult
	if err := json.Unmarshal([]byte(toolMsgs[0].Content), &writeRes); err != nil {
		t.Fatalf("tool[0].Content not valid JSON: %v (content=%q)", err, toolMsgs[0].Content)
	}
	if writeRes.BytesWritten != int64(len(expectedContent)) {
		t.Errorf("write result BytesWritten = %d, want %d", writeRes.BytesWritten, len(expectedContent))
	}

	// Second tool result: file_read
	if toolMsgs[1].ToolCallID != "call_read_1" {
		t.Errorf("tool[1].ToolCallID = %q, want call_read_1", toolMsgs[1].ToolCallID)
	}
	var readRes fileReadResult
	if err := json.Unmarshal([]byte(toolMsgs[1].Content), &readRes); err != nil {
		t.Fatalf("tool[1].Content not valid JSON: %v (content=%q)", err, toolMsgs[1].Content)
	}
	if readRes.Content != expectedContent {
		t.Errorf("read result Content = %q, want %q", readRes.Content, expectedContent)
	}

	// Verify assistant messages carry tool_calls.
	if len(assistantMsgs) < 2 {
		t.Fatalf("expected >=2 assistant messages, got %d", len(assistantMsgs))
	}
	if len(assistantMsgs[0].ToolCalls) != 1 || assistantMsgs[0].ToolCalls[0].Name != "file_write" {
		t.Errorf("assistant[0] should have file_write tool call, got %+v", assistantMsgs[0].ToolCalls)
	}
	if len(assistantMsgs[1].ToolCalls) != 1 || assistantMsgs[1].ToolCalls[0].Name != "file_read" {
		t.Errorf("assistant[1] should have file_read tool call, got %+v", assistantMsgs[1].ToolCalls)
	}
}

// TestTruncateToolResult verifies that large tool results are truncated
// while small ones pass through unchanged.
func TestTruncateToolResult(t *testing.T) {
	// Small result: should pass through unchanged.
	small := `{"status":"ok"}`
	if got := truncateToolResult(small, 4096); got != small {
		t.Errorf("small result should pass through unchanged, got %q", got)
	}

	// Large result: should be truncated.
	large := strings.Repeat("x", 10000)
	got := truncateToolResult(large, 4096)
	if len(got) >= len(large) {
		t.Errorf("large result should be truncated: got %d bytes, original %d bytes", len(got), len(large))
	}
	if !strings.Contains(got, "truncated") {
		t.Error("truncated result should contain 'truncated' marker")
	}
	if !strings.Contains(got, "10000 bytes total") {
		t.Error("truncated result should contain original size")
	}
}

// TestFormatToolResult_Truncation verifies that formatToolResult applies
// truncation to large action results.
func TestFormatToolResult_Truncation(t *testing.T) {
	// Large result that should be truncated.
	bigContent := strings.Repeat("A", 8000)
	result := &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: bigContent,
	}
	got := formatToolResult(result)
	if len(got) > defaultToolResultMaxBytes+200 { // some margin for the marker
		t.Errorf("formatToolResult should truncate: got %d bytes, limit %d", len(got), defaultToolResultMaxBytes)
	}

	// Nil result should return "null".
	if formatToolResult(nil) != "null" {
		t.Error("nil result should return 'null'")
	}

	// Error result.
	errResult := &action.ActionResult{OK: false, Error: "something failed"}
	got = formatToolResult(errResult)
	if got != "error: something failed" {
		t.Errorf("error result: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// P1-21: Engine depends only on model.StreamRequester (not BroadcastResponse)
// ---------------------------------------------------------------------------

// streamOnlyRequester implements model.StreamRequester and intentionally
// omits BroadcastResponse. It proves the Engine's production path does not
// depend on the wider ModelRequester interface.
type streamOnlyRequester struct {
	response string
	calls    int
}

func (s *streamOnlyRequester) Name() string { return "stream-only" }

func (s *streamOnlyRequester) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return &model.RequestData{Model: req.Model}, nil
}

func (s *streamOnlyRequester) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	ch := make(chan *model.StreamChunk, 1)
	ch <- &model.StreamChunk{Delta: s.response, IsDone: true}
	close(ch)
	s.calls++
	return ch, nil
}

// Compile-time guarantee that streamOnlyRequester satisfies the narrower
// StreamRequester interface used by the Engine.
var _ model.StreamRequester = (*streamOnlyRequester)(nil)

// NOTE: streamOnlyRequester deliberately does NOT implement model.ModelRequester
// (it lacks BroadcastResponse). Uncommenting the line below would fail to
// compile, which is exactly the point of the P1-21 interface split:
//
// var _ model.ModelRequester = (*streamOnlyRequester)(nil)

// TestEngine_AcceptsStreamOnlyRequester verifies that the Engine can run a
// full PLAN→EXECUTE turn with a requester that implements only
// model.StreamRequester (no BroadcastResponse). This is the P1-21 guarantee:
// the agent production path depends solely on StreamRequester.
func TestEngine_AcceptsStreamOnlyRequester(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	req := &streamOnlyRequester{
		response: `{"next_action":"response","final_response":"stream-only ok"}`,
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  req,
	}

	decision, err := engine.executeLoop(context.Background(), "hi", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected NextAction %q, got %q", "response", decision.NextAction)
	}
	if decision.FinalResponse != "stream-only ok" {
		t.Errorf("FinalResponse = %q, want %q", decision.FinalResponse, "stream-only ok")
	}
	if req.calls != 1 {
		t.Errorf("expected exactly 1 RequestModel call, got %d", req.calls)
	}
}

func TestEngine_CacheBudgetHook(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	var hookCalls []int
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			// First chunk: content with UsageInfo containing cached_tokens.
			ch <- &model.StreamChunk{
				Delta: `{"next_action":"response","final_response":"cached!"}`,
				Usage: &model.UsageInfo{
					PromptTokens:    1000,
					CompletionTokens: 10,
					TotalTokens:     1010,
					PromptTokensDetails: map[string]int{
						"cached_tokens": 800,
					},
				},
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
		cacheBudgetHook: func(cachedTokens int) {
			hookCalls = append(hookCalls, cachedTokens)
		},
	}

	decision, err := engine.executeLoop(context.Background(), "test", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, "response")
	}

	if len(hookCalls) == 0 {
		t.Fatal("cacheBudgetHook was never called")
	}
	if hookCalls[0] != 800 {
		t.Errorf("cacheBudgetHook called with %d, want 800", hookCalls[0])
	}
}

func TestEngine_CacheBudgetHook_NilSafe(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
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

	// cacheBudgetHook is nil — should not panic.
	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}

	_, err := engine.executeLoop(context.Background(), "test", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
}
