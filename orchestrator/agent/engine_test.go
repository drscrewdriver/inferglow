package agent

import (
	"context"
	"testing"

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

func TestEngine_DirectResponse(t *testing.T) {
	// Test when LLM directly returns a response without executing actions
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:    `{"next_action":"response","final_response":"Hello!"}`,
				IsDone:   true,
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
					Delta: `{"next_action":"execute","action_calls":[{"name":"calc","params":{}}]}`,
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

func TestEngine_MaxRounds(t *testing.T) {
	// Test that loop terminates when maxRounds is reached
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"execute","action_calls":[{"name":"noop","params":{}}]}`,
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

	decision, err := engine.executeLoop(context.Background(), "test", 2, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "execute" {
		t.Error("Expected execute (loop was forced to stop)")
	}
	// maxRounds=2 意味着最多执行 2 轮 after first call, so 3 total
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
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
