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

package policies

import (
	"github.com/inferglow/action"
	"github.com/inferglow/builtins/actions"
)

// RestrictivePolicy returns an ActionRegistry populated only with Actions
// whose SideEffectLevel is "none" or "read". Write and exec Actions are
// excluded entirely so the registry is safe to expose in untrusted
// contexts.
func RestrictivePolicy() *action.ActionRegistry {
	r := action.NewRegistry()
	acts := []*action.Action{
		actions.NewCalculatorAction(),
		actions.NewWebSearchAction(nil),
		actions.NewURLFetchAction(actions.URLFetchConfig{}),
		actions.NewFileReadAction(actions.FileReadConfig{}),
		actions.NewJSONProcessorAction(),
	}
	for _, a := range acts {
		if err := r.Register(a); err != nil {
			panic(err)
		}
	}
	return r
}
