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

package skill

// SkillBinding declares which skills are bound to an agent.
type SkillBinding struct {
	// Skills lists the skill references.
	Skills []SkillRef `json:"skills"`
	// Mode controls selection strategy.
	Mode SkillMode `json:"mode"`
}

// NewSkillBinding creates a binding with the given skills and mode.
func NewSkillBinding(mode SkillMode, refs ...SkillRef) *SkillBinding {
	return &SkillBinding{
		Skills: refs,
		Mode:   mode,
	}
}

// HasSkill returns true if the binding includes the given source.
func (b *SkillBinding) HasSkill(source string) bool {
	for _, s := range b.Skills {
		if s.Source == source {
			return true
		}
	}
	return false
}
