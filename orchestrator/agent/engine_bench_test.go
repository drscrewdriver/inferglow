package agent

import (
	"context"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// G1-07.5: Engine executeLoop Benchmark
// 覆盖 executeLoop 单轮迭代（直接 response）和多轮迭代（execute→response）。

// newBenchEngine 构造一个带 mock ModelRequester 的 Engine，responseFn 决定每轮返回内容。
func newBenchEngine(responseFn func(callIdx int) (string, bool)) *Engine {
	sess := NewSessionExtension(session.NewSession("bench", 1<<20))
	actExt := NewActionExtension()
	callIdx := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			callIdx++
			delta, isDone := responseFn(callIdx)
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: delta, IsDone: isDone}
			close(ch)
			return ch, nil
		},
	}
	return &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
	}
}

// BenchmarkExecuteLoopDirectResponse 测试单轮迭代：LLM 直接返回 response。
func BenchmarkExecuteLoopDirectResponse(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		engine := newBenchEngine(func(callIdx int) (string, bool) {
			return `{"next_action":"response","final_response":"Hello!"}`, true
		})
		b.StartTimer()
		_, _ = engine.executeLoop(ctx, "Hi", 3, "You are helpful.")
	}
}

// BenchmarkExecuteLoopExecuteThenResponse 测试两轮迭代：先 execute action 再 response。
func BenchmarkExecuteLoopExecuteThenResponse(b *testing.B) {
	ctx := context.Background()

	calcAction, _ := action.New("calc", "calculator",
		func(ctx context.Context, input map[string]any) (any, error) { return 42, nil })
	actExt := NewActionExtension()
	if err := actExt.Register(calcAction); err != nil {
		b.Fatalf("Register calc: %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sess := NewSessionExtension(session.NewSession("bench", 1<<20))
		callIdx := 0
		mockReq := &mockModelRequester{
			responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
				callIdx++
				ch := make(chan *model.StreamChunk, 1)
				if callIdx == 1 {
					ch <- &model.StreamChunk{
						Delta:  `{"next_action":"execute","action_calls":[{"name":"calc","params":{}}]}`,
						IsDone: true,
					}
				} else {
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
		b.StartTimer()
		_, _ = engine.executeLoop(ctx, "What is 21*2?", 5, "You are helpful.")
	}
}

// BenchmarkBuildToolDefinitions 测试 buildToolDefinitions 的开销（含排序 + hash 计算）。
func BenchmarkBuildToolDefinitions(b *testing.B) {
	sess := NewSessionExtension(session.NewSession("bench", 1<<20))
	actExt := NewActionExtension()
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		a, _ := action.New(name, name+" action",
			func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
		a.Schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
		}
		if err := actExt.Register(a); err != nil {
			b.Fatalf("Register %s: %v", name, err)
		}
	}
	engine := &Engine{session: sess, actionExt: actExt}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.buildToolDefinitions()
	}
}
