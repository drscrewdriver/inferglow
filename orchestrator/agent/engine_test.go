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

	decision, err := engine.executeLoop(context.Background(), "Hi", 3)
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

	decision, err := engine.executeLoop(context.Background(), "What is 21*2?", 5)
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

	decision, err := engine.executeLoop(context.Background(), "test", 2)
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
