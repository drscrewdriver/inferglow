package agent

import (
	"context"
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

func TestAgentCreation(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			return nil, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	if agent == nil {
		t.Fatal("Agent should not be nil")
	}
}

func TestAgentRunResponse(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"Hello from agent!"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	result, err := agent.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result != "Hello from agent!" {
		t.Errorf("Result: got %q, want %q", result, "Hello from agent!")
	}
}

func TestAgentRunExecuteNoResponseReturnsError(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"execute","action_calls":[{"name":"test","params":{}}]}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	_, err := agent.Run(context.Background(), "test")
	if err != ErrNoFinalResponse {
		t.Errorf("Expected ErrNoFinalResponse, got %v", err)
	}
}

func TestAgentRunWithSystemPrompt(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var capturedSystem string
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			capturedSystem = req.System
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"OK"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq, WithSystemPrompt("You are a test assistant"))
	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if capturedSystem != "You are a test assistant" {
		t.Errorf("System prompt not passed: got %q", capturedSystem)
	}
}
