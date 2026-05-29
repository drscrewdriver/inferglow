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
	"testing"

	"github.com/inferglow/orchestrator/taskdag"
)

func TestDirectStrategyName(t *testing.T) {
	s := DirectStrategy{}
	if s.Name() != "direct" {
		t.Errorf("expected 'direct', got %s", s.Name())
	}
}

func TestTaskDAGStrategyName(t *testing.T) {
	s := TaskDAGStrategy{Resolver: taskdag.NewStaticResolver()}
	if s.Name() != "task_dag" {
		t.Errorf("expected 'task_dag', got %s", s.Name())
	}
}

func TestDefaultExtensions(t *testing.T) {
	ext := DefaultExtensions()
	if ext.Strategy != nil {
		t.Error("expected nil strategy by default")
	}
}

func TestExecutionStrategyInterface(t *testing.T) {
	// Compile-time check.
	var _ ExecutionStrategy = DirectStrategy{}
	var _ ExecutionStrategy = TaskDAGStrategy{}
}
