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

package attempt

import (
	"testing"
	"time"
)

// stubChunk is a minimal Chunk implementation for tests that exercise
// behavior independent of chunk contents (e.g. backoff calculation).
type stubChunk struct{}

func (stubChunk) ChunkDelta() string        { return "" }
func (stubChunk) ChunkReasoning() string    { return "" }
func (stubChunk) ChunkToolCount() int       { return 0 }
func (stubChunk) ChunkMeta() map[string]any { return nil }

// Check 1.4.2: 指数退避策略生效（不精确测试时间，只验证逻辑）
//
// This test was moved from model/provider_test.go because it calls the
// unexported calculateBackoff method, which is only accessible from within
// the attempt package now that AttemptRunner lives here.
func TestAttemptRunnerBackoffLogic(t *testing.T) {
	runner := NewAttemptRunner[stubChunk]()
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
