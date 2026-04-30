package model

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Check 1.2.1: Register() 方法正确注册 Provider
func TestRegistryRegister(t *testing.T) {
	reg := NewModelRequesterRegistry()

	provider := &OpenAICompatibleProvider{
		BaseURL: "https://api.openai.com",
		APIKey:  "test-key",
		Model:   "gpt-4",
	}

	if err := reg.Register(provider); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	list := reg.List()
	if len(list) != 1 || list[0] != "openai-compatible" {
		t.Errorf("unexpected list: %v", list)
	}
}

// Check 1.2.2: Get() 方法按名称查找 Provider
func TestRegistryGet(t *testing.T) {
	reg := NewModelRequesterRegistry()

	provider := &OpenAICompatibleProvider{
		BaseURL: "https://api.openai.com",
		APIKey:  "test-key",
		Model:   "gpt-4",
	}

	if err := reg.Register(provider); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	found, err := reg.Get("openai-compatible")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if found.Name() != "openai-compatible" {
		t.Errorf("wrong provider: got %q", found.Name())
	}
}

// Check 1.2.3: 不存在名称返回 error
func TestRegistryGetNotFound(t *testing.T) {
	reg := NewModelRequesterRegistry()

	_, err := reg.Get("non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent provider")
	}

	expected := `provider "non-existent" not found`
	if err.Error() != expected {
		t.Errorf("wrong error: got %q, want %q", err.Error(), expected)
	}
}

// Check 1.2.4: 并发安全（sync.RWMutex）
func TestRegistryConcurrent(t *testing.T) {
	reg := NewModelRequesterRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 注册一个 provider
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	if err := reg.Register(provider); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 并发读写
	done := make(chan bool, 100)
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- true }()
			_, _ = reg.Get("openai-compatible")
		}()
		go func() {
			defer func() { done <- true }()
			_ = reg.List()
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 100; i++ {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("concurrent test timed out")
		}
	}
}

// Check: 重复注册返回 error
func TestRegistryDuplicateRegister(t *testing.T) {
	reg := NewModelRequesterRegistry()

	provider1 := &OpenAICompatibleProvider{Model: "gpt-4"}
	provider2 := &OpenAICompatibleProvider{Model: "gpt-3.5"}

	if err := reg.Register(provider1); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	err := reg.Register(provider2)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}

	expected := `provider "openai-compatible" already registered`
	if err.Error() != expected {
		t.Errorf("wrong error: got %q, want %q", err.Error(), expected)
	}
}

// Check: nil provider 返回 error
func TestRegistryNilProvider(t *testing.T) {
	reg := NewModelRequesterRegistry()
	err := reg.Register(nil)
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

// Check 1.4.1: 重试逻辑正确（失败 → 重试 → 成功）
func TestAttemptRunnerRetrySuccess(t *testing.T) {
	runner := NewAttemptRunner()
	runner.BackoffBase = 1 * time.Millisecond
	runner.BackoffMax = 10 * time.Millisecond
	runner.MaxAttempts = 5

	callCount := 0
	stream, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("temporary error")
		}
		ch := make(chan *StreamChunk, 1)
		ch <- &StreamChunk{IsDone: true}
		close(ch)
		return ch, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}

	chunk := <-stream
	if !chunk.IsDone {
		t.Error("expected IsDone=true")
	}
}

// Check 1.4.3: 达到最大重试次数后返回 error
func TestAttemptRunnerMaxRetries(t *testing.T) {
	runner := NewAttemptRunner()
	runner.BackoffBase = 1 * time.Millisecond
	runner.BackoffMax = 10 * time.Millisecond
	runner.MaxAttempts = 3

	callCount := 0
	_, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		callCount++
		return nil, errors.New("persistent error")
	})

	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// Check 1.4.4: output_started=true 后不重试
func TestAttemptRunnerOutputStarted(t *testing.T) {
	runner := NewAttemptRunner()
	runner.BackoffBase = 1 * time.Millisecond
	runner.BackoffMax = 10 * time.Millisecond
	runner.OutputStarted = true

	callCount := 0
	_, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		callCount++
		return nil, errors.New("error after output")
	})

	if err == nil {
		t.Fatal("expected error when output started")
	}
	// 应该只调用1次，因为 output_started=true 后不重试
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

// Check 1.4.2: 指数退避策略生效（不精确测试时间，只验证逻辑）
func TestAttemptRunnerBackoffLogic(t *testing.T) {
	runner := NewAttemptRunner()
	runner.BackoffBase = 1 * time.Second
	runner.BackoffMax = 30 * time.Second

	// 验证退避时间随着重试次数增加而增长
	firstBackoff := runner.calculateBackoff()
	runner.AttemptIndex = 4 // 第4次重试
	secondBackoff := runner.calculateBackoff()

	// 第4次重试的退避应该大于等于第一次（由于抖动可能相等，但不能小于）
	if secondBackoff < firstBackoff {
		t.Errorf("backoff should increase with attempts: first=%v, fourth=%v", firstBackoff, secondBackoff)
	}

	// 限制最大值检查
	if secondBackoff > runner.BackoffMax {
		t.Errorf("backoff exceeds max: %v > %v", secondBackoff, runner.BackoffMax)
	}
}
