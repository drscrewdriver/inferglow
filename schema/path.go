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

package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// PathPart 表示路径中的一个部分
type PathPart struct {
	Key   string
	Index int
	Wild  bool
}

// PathStyle 路径风格
type PathStyle string

// ParsePath 解析路径字符串为路径部分
func ParsePath(path string) []PathPart {
	if path == "" {
		return nil
	}

	var parts []PathPart
	remaining := path

	for len(remaining) > 0 {
		// 查找下一个分隔点
		nextDot := strings.Index(remaining, ".")
		nextBracket := strings.Index(remaining, "[")

		var segment string

		if nextDot == -1 && nextBracket == -1 {
			// 没有分隔符，取剩余全部
			segment = remaining
			remaining = ""
		} else if nextBracket != -1 && (nextDot == -1 || nextBracket < nextDot) {
			// 先遇到 [，取 [ 之前的部分
			segment = remaining[:nextBracket]
			remaining = remaining[nextBracket:]
		} else {
			// 先遇到 . 或没有 [
			segment = remaining[:nextDot]
			remaining = remaining[nextDot+1:]
		}

		// 创建 key 部分
		parts = append(parts, PathPart{Key: segment})

		// 检查是否有 [index] 或 [*] - 作为单独的 Part
		if len(remaining) > 0 && remaining[0] == '[' {
			closeBracket := strings.Index(remaining, "]")
			if closeBracket > 0 {
				middle := remaining[1:closeBracket]
				wildPart := PathPart{}
				if middle == "*" {
					wildPart.Wild = true
				} else {
					idx, err := strconv.Atoi(middle)
					if err == nil {
						wildPart.Index = idx
					}
				}
				remaining = remaining[closeBracket+1:]
				// 跳过可能的 .
				if len(remaining) > 0 && remaining[0] == '.' {
					remaining = remaining[1:]
				}
				// 将通配符/索引作为单独的部分
				parts = append(parts, wildPart)
			}
		}
	}

	return parts
}

// LocatePathInDict 在字典中通过路径定位值
func LocatePathInDict(data any, path string) (any, bool) {
	if path == "" {
		return data, true
	}

	parts := ParsePath(path)
	result := locateWithPath(data, parts)
	return result, result != nil
}

// locateWithPath 递归定位路径
func locateWithPath(data any, parts []PathPart) any {
	if len(parts) == 0 {
		return data
	}

	if data == nil {
		return nil
	}

	part := parts[0]
	remaining := parts[1:]

	if part.Wild {
		// 通配符：遍历数组
		arr, ok := data.([]any)
		if !ok {
			return nil
		}

		results := make([]any, 0, len(arr))
		for _, item := range arr {
			result := locateWithPath(item, remaining)
			if result != nil {
				results = append(results, result)
			}
		}

		if len(results) == 0 {
			return nil
		}
		return results
	}

	switch v := data.(type) {
	case map[string]any:
		child, ok := v[part.Key]
		if !ok {
			return nil
		}
		return locateWithPath(child, remaining)
	case []any:
		if part.Index < 0 || part.Index >= len(v) {
			return nil
		}
		return locateWithPath(v[part.Index], remaining)
	default:
		return nil
	}
}

// String 返回路径的字符串表示
func (pp PathPart) String() string {
	if pp.Wild {
		return "[*]"
	}
	if pp.Index >= 0 {
		return fmt.Sprintf("%s[%d]", pp.Key, pp.Index)
	}
	return pp.Key
}
