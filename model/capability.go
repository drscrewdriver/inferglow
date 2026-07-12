// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and licensed documentation files (the "Software"), to deal
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

package model

// ModelCapability declares the capabilities supported by a specific model.
// Used for runtime capability queries (e.g. "can this model process images?")
// and routing decisions (e.g. "only route to vision-capable models").
type ModelCapability struct {
	// Vision indicates the model can process image inputs.
	Vision bool
	// Audio indicates the model can process audio inputs.
	Audio bool
	// Video indicates the model can process video inputs.
	Video bool
	// Thinking indicates the model produces reasoning/thinking content
	// (e.g. DeepSeek R1, Claude 3.5 with extended thinking, o1/o3).
	Thinking bool
	// ToolCalling indicates the model supports function/tool calling.
	ToolCalling bool
	// JSONMode indicates the model supports structured JSON output
	// (response_format=json_object or json_schema).
	JSONMode bool
	// Streaming indicates the model supports SSE streaming.
	Streaming bool
	// MaxContext is the maximum context window size in tokens.
	// 0 means unknown or unlimited.
	MaxContext int
}

// ModelCapabilityRegistry maps model names/patterns to their capabilities.
// Keys are exact model names (e.g. "gpt-4o") or prefix patterns (e.g. "gpt-4").
// Prefix patterns end with "*" to match any model starting with that prefix.
var ModelCapabilityRegistry = map[string]ModelCapability{
	// OpenAI GPT-4 family
	"gpt-4o":                 {Vision: true, Audio: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},
	"gpt-4o-mini":            {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},
	"gpt-4-turbo":            {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},
	"gpt-4":                  {ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 8192},
	"o1":                     {Vision: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},
	"o1-mini":                {Thinking: true, ToolCalling: true, Streaming: true, MaxContext: 128000},
	"o3":                     {Vision: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},
	"o3-mini":                {Thinking: true, ToolCalling: true, Streaming: true, MaxContext: 200000},
	"o4-mini":                {Vision: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},

	// Anthropic Claude family
	"claude-opus-4-20250514":  {Vision: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},
	"claude-sonnet-4-20250514": {Vision: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},
	"claude-3-7-sonnet-20250219": {Vision: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},
	"claude-3-5-sonnet":        {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},
	"claude-3-5-haiku":         {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},
	"claude-3-opus":            {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 200000},

	// Google Gemini family
	"gemini-2.5-pro":   {Vision: true, Audio: true, Video: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 1000000},
	"gemini-2.5-flash": {Vision: true, Audio: true, Video: true, Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 1000000},
	"gemini-2.0-flash": {Vision: true, Audio: true, Video: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 1048576},

	// DeepSeek family
	"deepseek-chat":    {Vision: false, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 64000},
	"deepseek-reasoner": {Thinking: true, ToolCalling: false, JSONMode: true, Streaming: true, MaxContext: 64000},
	"deepseek-vl2":     {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 4096},

	// Qwen family
	"qwen-vl-max":    {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 32000},
	"qwen-vl-plus":   {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 32000},
	"qwen-max":       {ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 32000},
	"qwen-plus":      {ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 131072},
	"qwen-turbo":     {ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 131072},
	"qwen3-235b-a22b": {Thinking: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 131072},

	// GLM family (Zhipu AI)
	"glm-4v":    {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},
	"glm-4":     {ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},

	// Kimi/Moonshot family
	"kimi-latest": {Vision: true, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},
	"moonshot-v1-128k": {ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},

	// Cohere family
	"command-r-plus": {Vision: false, ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},
	"command-r":      {ToolCalling: true, JSONMode: true, Streaming: true, MaxContext: 128000},
}

// LookupModelCapability returns the capability for the given model name.
// It first tries exact match, then prefix match (longest prefix wins).
// Returns the capability and a boolean indicating whether it was found.
func LookupModelCapability(modelName string) (ModelCapability, bool) {
	// Exact match first
	if cap, ok := ModelCapabilityRegistry[modelName]; ok {
		return cap, true
	}

	// Prefix match: find the longest matching prefix
	bestLen := 0
	var bestCap ModelCapability
	found := false

	for pattern, cap := range ModelCapabilityRegistry {
		// Skip exact matches (already handled above)
		if len(pattern) <= bestLen {
			continue
		}
		// Check if modelName starts with pattern
		if len(modelName) >= len(pattern) && modelName[:len(pattern)] == pattern {
			// Ensure the match is at a word boundary (dash, dot, or end)
			if len(modelName) == len(pattern) ||
				modelName[len(pattern)] == '-' ||
				modelName[len(pattern)] == '.' {
				bestLen = len(pattern)
				bestCap = cap
				found = true
			}
		}
	}

	return bestCap, found
}

// SupportsVision reports whether the model supports image input.
// Returns false for unknown models (conservative default).
func SupportsVision(modelName string) bool {
	cap, _ := LookupModelCapability(modelName)
	return cap.Vision
}

// SupportsThinking reports whether the model produces reasoning content.
func SupportsThinking(modelName string) bool {
	cap, _ := LookupModelCapability(modelName)
	return cap.Thinking
}

// SupportsToolCalling reports whether the model supports function calling.
func SupportsToolCalling(modelName string) bool {
	cap, _ := LookupModelCapability(modelName)
	return cap.ToolCalling
}
