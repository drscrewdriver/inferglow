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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package model

// Effort wire-format translation (port of pi-ai's thinkingFormat dispatch).
//
// pi-ai (the DeepSeek Harness LLM layer) normalizes every provider's reasoning
// control behind one 7-level semantic scale
// (off → minimal → low → medium → high → xhigh → max) and a per-model
// thinkingLevelMap (level → wire value; null = not offered). The *wire shape*
// of the reasoning parameter is chosen by the provider's thinkingFormat:
//
//	openai      → reasoning_effort: "<level>"
//	deepseek    → thinking: {type: enabled|disabled} + reasoning_effort
//	openrouter  → reasoning: {effort: "<level>"}   (off → effort:"none")
//	together    → reasoning: {enabled: bool} + reasoning_effort
//	zai         → thinking: {type: enabled|disabled} + reasoning_effort
//	qwen        → enable_thinking: <bool>
//	string      → thinking: "<level>"              (off → "none")
//	ant-ling    → reasoning: {effort: "<level>"}
//	google      → thinkingConfig: {thinkingLevel: LOW|HIGH, thinkingBudget}
//	anthropic   → thinking: {type: enabled|disabled} + effort/budget_tokens
//	mistral     → reasoningEffort: "<level>"
//	bedrock     → output_config: {effort: "<level>"}
//
// The CLI produces only the semantic level; providers translate it here using
// the format they declared, falling back to no injection when the level is not
// offered (mirroring pi-ai's "a level absent from the dict is not offered").

// EffortWireFormat selects how a provider maps a semantic effort level to wire
// parameters.
type EffortWireFormat string

const (
	EffortOpenAI     EffortWireFormat = "openai"     // reasoning_effort (default, OpenAI-compatible)
	EffortDeepSeek   EffortWireFormat = "deepseek"   // thinking:{type} + reasoning_effort
	EffortOpenRouter EffortWireFormat = "openrouter" // reasoning:{effort}
	EffortTogether   EffortWireFormat = "together"   // reasoning:{enabled} + reasoning_effort
	EffortZAI        EffortWireFormat = "zai"        // thinking:{type} + reasoning_effort
	EffortQwen       EffortWireFormat = "qwen"       // enable_thinking: bool
	EffortString     EffortWireFormat = "string"     // thinking: "<level>"
	EffortAntLing    EffortWireFormat = "ant-ling"   // reasoning:{effort}
	EffortGoogle     EffortWireFormat = "google"     // thinkingConfig:{thinkingLevel, thinkingBudget}
	EffortAnthropic  EffortWireFormat = "anthropic"  // thinking:{type} + effort/budget_tokens
	EffortMistral    EffortWireFormat = "mistral"    // reasoningEffort: "<level>"
	EffortBedrock    EffortWireFormat = "bedrock"    // output_config:{effort}
)

// canonicalEffortLevels is the 7-level semantic scale in escalation order
// (pi-ai THINKING_LEVELS).
var canonicalEffortLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// EffortLevelMap maps a semantic level to its wire value for one model.
// A nil entry means the level is known but not offered (send nothing);
// an absent level means the model does not provide it. The map is nil-safe:
// a nil map offers nothing.
type EffortLevelMap map[string]any

// TranslateEffort converts a semantic effort level into the wire parameters
// for the given format and level map. The returned map contains only keys the
// provider should merge into its request body.
//
// Semantics (mirroring pi-ai):
//   - level "" or "auto" → nil (provider default, no injection).
//   - level not in the map:
//     - if the format has an explicit off-wire (deepseek/openrouter/string),
//     the level is still injected as a passthrough value; otherwise nil.
//   - level in the map with a nil value → the level is explicitly not offered
//     → nil (no injection).
//   - level in the map with a string/number value → injected per format.
func TranslateEffort(format EffortWireFormat, level string, lm EffortLevelMap) map[string]any {
	if level == "" || level == "auto" {
		return nil
	}
	wire, offered := lm[level]
	if !offered {
		// Not declared by the model: pi-ai passes the raw level through when
		// the format supports it, else nothing.
		if formatAllowsPassthrough(format) {
			return formatPassthrough(format, level)
		}
		return nil
	}
	if wire == nil {
		return nil // explicitly not offered
	}
	return formatParams(format, level, wire)
}

// formatAllowsPassthrough reports whether the format injects an undeclared
// level verbatim (OpenAI-style reasoning_effort accepts any string; formats
// with structured envelopes only accept declared values).
func formatAllowsPassthrough(format EffortWireFormat) bool {
	switch format {
	case EffortOpenAI, EffortOpenRouter, EffortString, EffortMistral, EffortBedrock:
		return true
	}
	return false
}

// formatPassthrough injects an undeclared level verbatim for formats that
// accept any string wire value.
func formatPassthrough(format EffortWireFormat, level string) map[string]any {
	switch format {
	case EffortOpenAI:
		return map[string]any{"reasoning_effort": level}
	case EffortOpenRouter:
		return map[string]any{"reasoning": map[string]any{"effort": level}}
	case EffortString:
		return map[string]any{"thinking": level}
	case EffortMistral:
		return map[string]any{"reasoningEffort": level}
	case EffortBedrock:
		return map[string]any{"output_config": map[string]any{"effort": level}}
	}
	return nil
}

// formatParams builds the wire parameters for a declared level.
func formatParams(format EffortWireFormat, level string, wire any) map[string]any {
	switch format {
	case EffortOpenAI:
		return map[string]any{"reasoning_effort": wire}

	case EffortDeepSeek:
		// thinking enabled for any non-off level; reasoning_effort carries
		// the wire value when it is a string.
		p := map[string]any{"thinking": map[string]any{"type": "enabled"}}
		if s, ok := wire.(string); ok {
			p["reasoning_effort"] = s
		}
		return p

	case EffortOpenRouter:
		return map[string]any{"reasoning": map[string]any{"effort": wire}}

	case EffortTogether:
		p := map[string]any{"reasoning": map[string]any{"enabled": true}}
		if s, ok := wire.(string); ok {
			p["reasoning_effort"] = s
		}
		return p

	case EffortZAI:
		p := map[string]any{"thinking": map[string]any{"type": "enabled"}}
		if s, ok := wire.(string); ok {
			p["reasoning_effort"] = s
		}
		return p

	case EffortQwen:
		return map[string]any{"enable_thinking": true}

	case EffortString:
		return map[string]any{"thinking": wire}

	case EffortAntLing:
		return map[string]any{"reasoning": map[string]any{"effort": wire}}

	case EffortGoogle:
		return map[string]any{"thinkingConfig": map[string]any{"thinkingLevel": wire}}

	case EffortAnthropic:
		p := map[string]any{"thinking": map[string]any{"type": "enabled"}}
		if s, ok := wire.(string); ok {
			p["effort"] = s
		}
		return p

	case EffortMistral:
		return map[string]any{"reasoningEffort": wire}

	case EffortBedrock:
		return map[string]any{"output_config": map[string]any{"effort": wire}}
	}
	return nil
}

// EffortOffWire returns the wire parameters that disable reasoning for the
// format, or nil when the format has no explicit off representation (absence
// of the reasoning parameter is the off state — matching pi-ai's `off` with a
// null value).
func EffortOffWire(format EffortWireFormat) map[string]any {
	switch format {
	case EffortDeepSeek, EffortZAI:
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	case EffortQwen:
		return map[string]any{"enable_thinking": false}
	case EffortOpenRouter:
		return map[string]any{"reasoning": map[string]any{"effort": "none"}}
	case EffortString:
		return map[string]any{"thinking": "none"}
	case EffortTogether:
		return map[string]any{"reasoning": map[string]any{"enabled": false}}
	}
	return nil
}
