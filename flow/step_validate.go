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

package flow

import "github.com/inferglow/schema"

// validateStepOutput validates step output against the bound OutputSchema.
// Uses schema.ContractEngine with EnsureKeys built from Required fields.
func validateStepOutput(output any, s *schema.OutputSchema) error {
	if s == nil {
		return nil
	}
	engine := &schema.ContractEngine{
		EnsureKeys: buildEnsureKeysFromSchema(s),
		EnsureAll:  true,
	}
	return engine.ValidateResult(output)
}

// buildEnsureKeysFromSchema builds EnsureKeys map from OutputSchema.Fields.
// Only fields with Required=true are included, with EnsurePresence policy.
// The field name is the map key (FieldDef has no Name field).
func buildEnsureKeysFromSchema(s *schema.OutputSchema) map[string]schema.EnsurePolicy {
	keys := make(map[string]schema.EnsurePolicy)
	if s == nil || s.Fields == nil {
		return keys
	}
	for name, field := range s.Fields {
		if field != nil && field.Required {
			keys[name] = schema.EnsurePresence
		}
	}
	return keys
}
