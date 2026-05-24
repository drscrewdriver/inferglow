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

package agenthook

import (
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/security/pii"
)

// Compile-time guard: *PIIMasker must satisfy agent.PIIMasker so it can be
// passed to agent.WithPIIMasker.
var _ agent.PIIMasker = (*PIIMasker)(nil)

// PIIMasker adapts a *pii.Masker to the agent.PIIMasker interface. It
// delegates MaskInput and MaskOutput to the underlying masker, which
// honors its own ApplyOn configuration to decide whether each side is
// actually masked.
//
// Use NewPIIMasker to construct one; the returned value can be passed
// directly to agent.WithPIIMasker or agent.WithAgentPIIMasker.
type PIIMasker struct {
	m *pii.Masker
}

// NewPIIMasker wraps a *pii.Masker as an agent.PIIMasker. Pass the result
// to agent.WithPIIMasker (or agent.WithAgentPIIMasker on ChatModelAgent)
// to wire PII masking into the agent without the orchestrator needing a
// direct dependency on security/pii.
func NewPIIMasker(m *pii.Masker) *PIIMasker {
	return &PIIMasker{m: m}
}

// MaskInput delegates to the underlying *pii.Masker.MaskInput, which
// redacts text for the input side of the conversation (a no-op when the
// masker's ApplyOn does not include MaskOnInput).
func (p *PIIMasker) MaskInput(text string) string {
	if p == nil || p.m == nil {
		return text
	}
	return p.m.MaskInput(text)
}

// MaskOutput delegates to the underlying *pii.Masker.MaskOutput, which
// redacts text for the output side of the conversation (a no-op when the
// masker's ApplyOn does not include MaskOnOutput).
func (p *PIIMasker) MaskOutput(text string) string {
	if p == nil || p.m == nil {
		return text
	}
	return p.m.MaskOutput(text)
}
