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

package actionruntime

import "testing"

func TestShouldContinue_Execute(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: []ActionCall{{Name: "test"}}}
	if !ShouldContinue(decision, 0, 5) {
		t.Error("Expected shouldContinue=true for execute decision")
	}
}

func TestShouldContinue_Response(t *testing.T) {
	decision := Decision{NextAction: "response", FinalResponse: "done"}
	if ShouldContinue(decision, 0, 5) {
		t.Error("Expected shouldContinue=false for response decision")
	}
}

func TestShouldContinue_MaxRounds(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: []ActionCall{{Name: "test"}}}
	if ShouldContinue(decision, 5, 5) {
		t.Error("Expected shouldContinue=false at max rounds")
	}
}

func TestShouldContinue_MaxRoundsExceeded(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: []ActionCall{{Name: "test"}}}
	if ShouldContinue(decision, 6, 5) {
		t.Error("Expected shouldContinue=false when exceeding max rounds")
	}
}

func TestShouldContinue_EmptyActionCalls(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: nil}
	if ShouldContinue(decision, 0, 5) {
		t.Error("Expected shouldContinue=false when ActionCalls is empty")
	}
}
