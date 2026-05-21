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

// Package tools generates LLM tool definitions from Go function signatures.
package tools

import "strings"

// DocInfo holds the parsed components of a Go docstring.
type DocInfo struct {
	Summary     string
	Description string
	Params      map[string]string
}

// paramEntry preserves the order of @param tags as they appear in the
// docstring, which is necessary for matching them to positional function
// parameters (DocInfo.Params is a map and therefore unordered).
type paramEntry struct {
	Name string
	Desc string
}

// ParseDocstring parses a Go docstring into its components.
//
// The first non-empty, non-@param line becomes Summary. Subsequent
// non-empty, non-@param lines are joined into Description. Lines
// matching "@param name description" populate Params.
func ParseDocstring(doc string) *DocInfo {
	info := &DocInfo{Params: map[string]string{}}
	if doc == "" {
		return info
	}

	var descLines []string
	summaryFound := false
	for _, raw := range strings.Split(doc, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if name, desc, ok := parseParamLine(line); ok {
			info.Params[name] = desc
			continue
		}

		if !summaryFound {
			info.Summary = line
			summaryFound = true
		}
		descLines = append(descLines, line)
	}

	info.Description = strings.Join(descLines, "\n")
	return info
}

// ExtractParamDesc extracts the description of a single parameter from
// a docstring. Returns an empty string if the parameter is not documented.
func ExtractParamDesc(doc string, paramName string) string {
	info := ParseDocstring(doc)
	return info.Params[paramName]
}

// parseParamEntries returns the @param tags in the order they appear,
// so they can be matched positionally to a function's input parameters.
func parseParamEntries(doc string) []paramEntry {
	var entries []paramEntry
	for _, raw := range strings.Split(doc, "\n") {
		line := strings.TrimSpace(raw)
		name, desc, ok := parseParamLine(line)
		if !ok {
			continue
		}
		entries = append(entries, paramEntry{Name: name, Desc: desc})
	}
	return entries
}

// parseParamLine parses a single "@param name description" line.
// Returns ok=false if the line is not a @param tag.
func parseParamLine(line string) (name, desc string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "@param" {
		return "", "", false
	}
	name = fields[1]
	if len(fields) > 2 {
		desc = strings.Join(fields[2:], " ")
	}
	return name, desc, true
}
