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

package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// ParseEventType 流式 JSON 解析事件类型
type ParseEventType string

const (
	// ParseObjectStart is emitted when a JSON object opening brace is encountered.
	ParseObjectStart ParseEventType = "object_start"
	// ParseObjectEnd is emitted when a JSON object closing brace is encountered.
	ParseObjectEnd ParseEventType = "object_end"
	// ParseArrayStart is emitted when a JSON array opening bracket is encountered.
	ParseArrayStart ParseEventType = "array_start"
	// ParseArrayEnd is emitted when a JSON array closing bracket is encountered.
	ParseArrayEnd ParseEventType = "array_end"
	// ParseKey is emitted when an object key is parsed.
	ParseKey ParseEventType = "key"
	// ParseValue is emitted when a value is parsed.
	ParseValue ParseEventType = "value"
	// ParseDone is emitted when the JSON document has been fully parsed.
	ParseDone ParseEventType = "done"
	// ParseError is emitted when a parse error occurs.
	ParseError ParseEventType = "error"
)

// JSONParseEvent 流式解析事件
type JSONParseEvent struct {
	Type  ParseEventType
	Key   string // 当 Type=ParseKey 时为键名；Type=ParseValue 时为对象上下文中的键名
	Value any    // 当 Type=ParseValue 时为值；ParseDone 时为完整结果；ParseError 时为错误消息字符串
	Path  string // 当前 JSON 路径，如 "user.addresses[0].city"
}

// pathFrame 路径栈帧
type pathFrame struct {
	isArr bool   // 是否数组上下文
	key   string // 对象上下文：等待值的键名
	index int    // 数组上下文：下一个值的索引
	path  string // 此容器自身的路径
}

// StreamingJSONParser 流式 JSON 解析器。
// 接收 SSE chunk 的增量文本，逐 token 解析 JSON 并产生增量事件。
// 支持不完整 JSON：在 token 边界暂停，等待更多数据。
//
// 用法：
//
//	parser := NewStreamingJSONParser()
//	go func() { /* 消费 parser.Events() */ }()
//	parser.Feed(`{"a":`)
//	parser.Feed(`1}`)
//	parser.Close()
type StreamingJSONParser struct {
	mu sync.Mutex

	buffer      []byte
	lastEmitted int // buffer 中已 emit 过的字节偏移量

	events chan JSONParseEvent

	closed bool // 是否已调用 Close（输入结束）
	done   bool // 是否已产生 Done/Error 事件

	result any
	err    error
}

// NewStreamingJSONParser 创建流式 JSON 解析器
func NewStreamingJSONParser() *StreamingJSONParser {
	return &StreamingJSONParser{
		events: make(chan JSONParseEvent, 256),
	}
}

// Feed 喂入增量文本。解析 token 边界并产生事件到 Events() channel。
// 不完整的 token 会等待下一次 Feed。
func (p *StreamingJSONParser) Feed(chunk string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.done {
		return fmt.Errorf("parser already done")
	}
	if p.err != nil {
		return p.err
	}
	if chunk == "" {
		return nil
	}
	p.buffer = append(p.buffer, chunk...)
	return p.drainTokens()
}

// Close 标记输入结束。如果 JSON 不完整，产生 ParseError 事件。
func (p *StreamingJSONParser) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.done {
		return nil
	}
	p.closed = true
	return p.drainTokens()
}

// Events 返回事件 channel，关闭表示事件流结束（Done 或 Error 已产生）。
func (p *StreamingJSONParser) Events() <-chan JSONParseEvent {
	return p.events
}

// Result 返回完整解析结果。仅在 Done 事件后可用。
func (p *StreamingJSONParser) Result() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// drainTokens 重新解析整个 buffer，仅 emit lastEmitted 之后的新 token。
// 调用方需持有 p.mu。
func (p *StreamingJSONParser) drainTokens() error {
	dec := json.NewDecoder(bytes.NewReader(p.buffer))

	var pathStack []pathFrame
	var containerStack []any
	var topResult any

	containerPath := func() string {
		if len(pathStack) == 0 {
			return ""
		}
		return pathStack[len(pathStack)-1].path
	}

	valuePath := func() string {
		if len(pathStack) == 0 {
			return ""
		}
		top := pathStack[len(pathStack)-1]
		if top.isArr {
			return fmt.Sprintf("%s[%d]", top.path, top.index)
		}
		if top.path == "" {
			return top.key
		}
		return top.path + "." + top.key
	}

	consumeParentSlot := func() {
		if len(pathStack) == 0 {
			return
		}
		top := &pathStack[len(pathStack)-1]
		if top.isArr {
			top.index++
		} else {
			top.key = ""
		}
	}

	addToParent := func(value any) {
		if len(containerStack) == 0 {
			topResult = value
			return
		}
		top := containerStack[len(containerStack)-1]
		frame := &pathStack[len(pathStack)-1]
		switch t := top.(type) {
		case map[string]any:
			t[frame.key] = value
		case []any:
			containerStack[len(containerStack)-1] = append(t, value)
		}
	}

	emitIfNew := func(postOffset int, evt JSONParseEvent) {
		if postOffset > p.lastEmitted {
			p.events <- evt
			p.lastEmitted = postOffset
		}
	}

	for {
		tok, err := dec.Token()
		postOffset := int(dec.InputOffset())

		if err == io.ErrUnexpectedEOF {
			// 不完整 token
			if p.closed {
				// 输入已结束但仍不完整 → error
				p.done = true
				p.err = fmt.Errorf("unexpected EOF: incomplete JSON")
				p.events <- JSONParseEvent{
					Type:  ParseError,
					Value: "unexpected EOF: incomplete JSON",
					Path:  containerPath(),
				}
				close(p.events)
				return p.err
			}
			// 等待更多数据
			return nil
		}
		if err == io.EOF {
			// io.EOF 可能在两种情况下出现：
			// 1. JSON 完整解析完毕（pathStack 为空，所有容器已闭合）
			// 2. JSON 结构不完整但 decoder 恰好在 token 之间（如 `{"a":"Bob` 后等待更多数据）
			// 需要通过 pathStack 区分。
			if len(pathStack) > 0 {
				// 结构不完整（仍有未闭合的容器）
				if p.closed {
					p.done = true
					p.err = fmt.Errorf("unexpected EOF: incomplete JSON (unclosed containers)")
					p.events <- JSONParseEvent{
						Type:  ParseError,
						Value: "unexpected EOF: incomplete JSON (unclosed containers)",
						Path:  containerPath(),
					}
					close(p.events)
					return p.err
				}
				// 等待更多数据
				return nil
			}
			// pathStack 为空：可能是干净结束，也可能是空 buffer
			if p.lastEmitted == 0 {
				// 没有消费任何 token（空 buffer 或仅空白）
				if p.closed && !p.done {
					p.done = true
					p.result = nil
					p.events <- JSONParseEvent{Type: ParseDone, Value: nil}
					close(p.events)
				}
				return nil
			}
			// 干净结束
			if !p.done {
				p.done = true
				p.result = topResult
				p.events <- JSONParseEvent{
					Type:  ParseDone,
					Value: topResult,
					Path:  containerPath(),
				}
				close(p.events)
			}
			return nil
		}
		if err != nil {
			p.done = true
			p.err = err
			p.events <- JSONParseEvent{
				Type:  ParseError,
				Value: err.Error(),
				Path:  containerPath(),
			}
			close(p.events)
			return err
		}

		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				// 开容器时不加入父节点（避免 slice aliasing 问题），关闭时再加入
				path := valuePath()
				m := make(map[string]any)
				containerStack = append(containerStack, m)
				pathStack = append(pathStack, pathFrame{isArr: false, path: path})
				emitIfNew(postOffset, JSONParseEvent{Type: ParseObjectStart, Path: path})

			case '}':
				path := containerPath()
				var m map[string]any
				if len(containerStack) > 0 {
					m, _ = containerStack[len(containerStack)-1].(map[string]any)
					containerStack = containerStack[:len(containerStack)-1]
				}
				if len(pathStack) > 0 {
					pathStack = pathStack[:len(pathStack)-1]
				}
				// 关闭时加入父节点（此时 parent 在栈顶）
				if len(containerStack) == 0 {
					topResult = m
				} else {
					addToParent(m)
					consumeParentSlot()
				}
				emitIfNew(postOffset, JSONParseEvent{Type: ParseObjectEnd, Path: path})

			case '[':
				path := valuePath()
				arr := make([]any, 0)
				containerStack = append(containerStack, arr)
				pathStack = append(pathStack, pathFrame{isArr: true, path: path, index: 0})
				emitIfNew(postOffset, JSONParseEvent{Type: ParseArrayStart, Path: path})

			case ']':
				path := containerPath()
				var arr []any
				if len(containerStack) > 0 {
					arr, _ = containerStack[len(containerStack)-1].([]any)
					containerStack = containerStack[:len(containerStack)-1]
				}
				if len(pathStack) > 0 {
					pathStack = pathStack[:len(pathStack)-1]
				}
				// 关闭时加入父节点，确保父节点持有最终完整的 slice
				if len(containerStack) == 0 {
					topResult = arr
				} else {
					addToParent(arr)
					consumeParentSlot()
				}
				emitIfNew(postOffset, JSONParseEvent{Type: ParseArrayEnd, Path: path})
			}

		case string:
			// 判断是 key 还是 value
			topIsObj := len(pathStack) > 0 && !pathStack[len(pathStack)-1].isArr
			if topIsObj && pathStack[len(pathStack)-1].key == "" {
				// 这是一个 key
				pathStack[len(pathStack)-1].key = t
				emitIfNew(postOffset, JSONParseEvent{Type: ParseKey, Key: t, Path: containerPath()})
			} else {
				// 这是一个 string value
				path := valuePath()
				key := ""
				if len(pathStack) > 0 && !pathStack[len(pathStack)-1].isArr {
					key = pathStack[len(pathStack)-1].key
				}
				addToParent(t)
				consumeParentSlot()
				emitIfNew(postOffset, JSONParseEvent{Type: ParseValue, Key: key, Value: t, Path: path})
			}

		default:
			// number (float64) / bool / nil
			path := valuePath()
			key := ""
			if len(pathStack) > 0 && !pathStack[len(pathStack)-1].isArr {
				key = pathStack[len(pathStack)-1].key
			}
			// M-HIGH-11: numbers are ambiguous under incremental feeding.
			// json.Decoder returns a number as soon as it sees a complete
			// prefix, so feeding "3" then "0" produces two value events
			// (3 and 30) for the same logical slot. Defer emitting a number
			// when the buffer ends right after it (no confirming delimiter)
			// or when the next byte could extend the number, unless input
			// has been closed (in which case the number is final).
			if _, isNumber := t.(float64); isNumber && !p.closed {
				if postOffset >= len(p.buffer) || isNumberExtender(p.buffer[postOffset]) {
					return nil
				}
			}
			addToParent(t)
			consumeParentSlot()
			emitIfNew(postOffset, JSONParseEvent{Type: ParseValue, Key: key, Value: t, Path: path})
		}
	}
}

// String 便于调试的事件表示
func (e JSONParseEvent) String() string {
	var sb strings.Builder
	sb.WriteString(string(e.Type))
	if e.Key != "" {
		sb.WriteString(" key=")
		sb.WriteString(e.Key)
	}
	if e.Value != nil {
		sb.WriteString(" value=")
		fmt.Fprintf(&sb, "%v", e.Value)
	}
	if e.Path != "" {
		sb.WriteString(" path=")
		sb.WriteString(e.Path)
	}
	return sb.String()
}

// isNumberExtender reports whether b could extend a JSON number token
// (digit, decimal point, exponent marker, or sign). Used by drainTokens
// to decide whether a number value returned by json.Decoder is final or
// could grow as more bytes arrive.
//
// M-HIGH-11: incremental parsing must not emit duplicate value events
// for the same logical slot when a number is fed character-by-character.
func isNumberExtender(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return true
	case b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-':
		return true
	}
	return false
}
