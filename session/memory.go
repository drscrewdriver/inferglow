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

package session

// Memory is the interface for conversation memory management.
// Implementations control how chat history is stored, retrieved, and
// trimmed. Load returns the current memory state as a list of messages.
// Save persists the given messages. Clear resets the memory.
//
// Memory integrates with the Session's resize mechanism: implementations
// can be registered as resize handlers via RegisterResizeHandler to
// automatically manage context window size.
type Memory interface {
	// Load returns the current conversation memory as ChatMessages.
	Load() []ChatMessage
	// Save replaces the memory contents with the given messages.
	Save(messages []ChatMessage)
	// Clear resets all memory state.
	Clear()
}

// Summarizer generates a summary of the given text. Used by SummaryMemory
// to compress old conversation history. The interface is defined in the
// session package to avoid a direct dependency on the model package.
type Summarizer interface {
	// Summarize produces a concise summary of the input text.
	Summarize(input string) (string, error)
}
