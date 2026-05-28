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

	"github.com/inferglow/model"
)

func TestWithOutputSchema(t *testing.T) {
	// Apply WithOutputSchema and assert the schema is propagated to runConfig.
	c := &runConfig{}
	WithOutputSchema(&model.OutputSchema{Type: "object"})(c)
	if c.outputSchema == nil {
		t.Fatalf("expected outputSchema to be non-nil after WithOutputSchema")
	}
	if c.outputSchema.Type != "object" {
		t.Fatalf("expected outputSchema.Type to be \"object\", got %q", c.outputSchema.Type)
	}

	// Default runConfig with no options applied should have a nil outputSchema.
	c2 := &runConfig{}
	if c2.outputSchema != nil {
		t.Fatalf("expected outputSchema to be nil by default, got %v", c2.outputSchema)
	}
}
