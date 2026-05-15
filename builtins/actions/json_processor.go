package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/inferglow/action"
)

// JSONProcessorActionID is the registered Action name for the JSON processor.
const JSONProcessorActionID = "json_processor"

// JSONOp is the operation kind for the JSON processor.
type JSONOp string

const (
	// JSONOpQuery parses the JSON document and returns the value at the
	// given JSONPath expression.
	JSONOpQuery JSONOp = "query"
	// JSONOpParse parses the JSON document and returns it as-is (used
	// for validation / normalization).
	JSONOpParse JSONOp = "parse"
)

// JSONProcessorInput is the strongly-typed input for json_processor.
type JSONProcessorInput struct {
	JSON  string `json:"json"`
	Path  string `json:"path"`
	Op    string `json:"op"` // "query" (default) or "parse"
}

// JSONProcessorSpec is the ActionSpec for json_processor: no side
// effects, no approval, no sandbox.
var JSONProcessorSpec = &action.ActionSpec{
	ActionID:         JSONProcessorActionID,
	Name:             "JSONProcessor",
	Description:      "Parse JSON and extract values via JSONPath-style queries.",
	SideEffectLevel:  action.SideEffectNone,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"json", "data", "builtin"},
	Kwargs: map[string]any{
		"json": map[string]any{"type": "string", "required": true},
		"path": map[string]any{"type": "string", "required": false},
		"op":   map[string]any{"type": "string", "required": false},
	},
	Returns: map[string]any{"type": "any"},
}

// jsonProcessorExecutor is the ActionExecutor for JSON processing.
type jsonProcessorExecutor struct{}

// JSONQuery evaluates a JSONPath-style expression against data and
// returns the matched value(s).
//
// Supported syntax:
//   - $                    — root document
//   - .field               — object field access
//   - ['field'] / ["field"]— quoted field access
//   - [N]                  — array index (negative indexes from end)
//   - [start:end]          — array slice (Python-style, end-exclusive)
//   - .*                   — wildcard (all object values / array items)
//
// Expressions can be chained, e.g. $.store.book[0].title.
func JSONQuery(data any, path string) (any, error) {
	steps, err := parseJSONPath(path)
	if err != nil {
		return nil, err
	}
	cur := data
	for _, step := range steps {
		cur, err = applyStep(cur, step)
		if err != nil {
			return nil, err
		}
	}
	return cur, nil
}

// jsonPathStep is a single navigation step. Exactly one field is set.
type jsonPathStep struct {
	field   string // object key (when kind == "field")
	index   int    // array index (when kind == "index")
	slice   bool   // marks a slice step
	start   int    // slice start (when slice == true)
	end     int    // slice end (when slice == true; -1 = open-ended)
	wildcard bool  // when true, return all children
}

// parseJSONPath tokenizes path into a sequence of steps. The leading
// "$" is optional.
func parseJSONPath(path string) ([]jsonPathStep, error) {
	if path == "" || path == "$" {
		return nil, nil
	}
	p := path
	if strings.HasPrefix(p, "$") {
		p = p[1:]
	}

	var steps []jsonPathStep
	for len(p) > 0 {
		switch p[0] {
		case '.':
			// Either ".field" or ".*"
			if len(p) > 1 && p[1] == '*' {
				steps = append(steps, jsonPathStep{wildcard: true})
				p = p[2:]
				continue
			}
			p = p[1:]
			field, rest, err := readField(p)
			if err != nil {
				return nil, err
			}
			if field == "" {
				return nil, fmt.Errorf("json: empty field after '.' in path %q", path)
			}
			steps = append(steps, jsonPathStep{field: field})
			p = rest
		case '[':
			// Bracketed: ['name'], ["name"], [N], [start:end], [*]
			end := strings.IndexByte(p, ']')
			if end < 0 {
				return nil, fmt.Errorf("json: unmatched '[' in path %q", path)
			}
			inner := strings.TrimSpace(p[1:end])
			p = p[end+1:]
			step, err := parseBracket(inner)
			if err != nil {
				return nil, fmt.Errorf("json: %w in path %q", err, path)
			}
			steps = append(steps, step)
		default:
			// Allow bare field names at the start of a path (e.g.
			// "user.name" is equivalent to "$.user.name"). Any other
			// character is rejected.
			if isIdentStart(p[0]) {
				field, rest, err := readField(p)
				if err != nil {
					return nil, err
				}
				steps = append(steps, jsonPathStep{field: field})
				p = rest
				continue
			}
			return nil, fmt.Errorf("json: unexpected character %q in path %q", p[0], path)
		}
	}
	return steps, nil
}

// isIdentStart reports whether c may start a bare JSONPath field name.
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// readField consumes a bare field name (alphanumeric + underscore)
// starting at s and returns (field, remaining).
func readField(s string) (string, string, error) {
	if s == "" {
		return "", "", fmt.Errorf("empty field")
	}
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return "", "", fmt.Errorf("invalid field character %q", s[0])
	}
	return s[:i], s[i:], nil
}

// parseBracket parses the content between '[' and ']'.
func parseBracket(inner string) (jsonPathStep, error) {
	if inner == "*" {
		return jsonPathStep{wildcard: true}, nil
	}
	// Quoted string → field access.
	if len(inner) >= 2 && (inner[0] == '\'' || inner[0] == '"') && inner[len(inner)-1] == inner[0] {
		return jsonPathStep{field: inner[1 : len(inner)-1]}, nil
	}
	// Slice [start:end] (either side may be empty).
	if strings.Contains(inner, ":") {
		parts := strings.SplitN(inner, ":", 2)
		start, err := parseIntOrEmpty(parts[0], 0)
		if err != nil {
			return jsonPathStep{}, fmt.Errorf("invalid slice start %q", parts[0])
		}
		// Open-ended end (empty string) is represented by math.MaxInt32
		// so applySlice can clamp it to len(arr) without conflicting
		// with explicit negative indexes.
		end := math.MaxInt32
		if parts[1] != "" {
			end, err = strconv.Atoi(parts[1])
			if err != nil {
				return jsonPathStep{}, fmt.Errorf("invalid slice end %q", parts[1])
			}
		}
		return jsonPathStep{slice: true, start: start, end: end}, nil
	}
	// Bare integer → array index.
	idx, err := strconv.Atoi(inner)
	if err != nil {
		// Fall back to treating it as a bare field name (some
		// JSONPath dialects allow [field] without quotes).
		return jsonPathStep{field: inner}, nil
	}
	return jsonPathStep{index: idx}, nil
}

// parseIntOrEmpty parses "" as def, otherwise strconv.Atoi.
func parseIntOrEmpty(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	return strconv.Atoi(s)
}

// applyStep advances cur by one navigation step.
func applyStep(cur any, step jsonPathStep) (any, error) {
	if step.wildcard {
		return wildcardChildren(cur), nil
	}
	if step.slice {
		return applySlice(cur, step.start, step.end)
	}
	if step.field != "" {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("json: cannot access field %q on %T", step.field, cur)
		}
		v, exists := m[step.field]
		if !exists {
			return nil, fmt.Errorf("json: field %q not found", step.field)
		}
		return v, nil
	}
	// index step
	arr, ok := cur.([]any)
	if !ok {
		return nil, fmt.Errorf("json: cannot index into %T", cur)
	}
	idx := step.index
	if idx < 0 {
		idx = len(arr) + idx
	}
	if idx < 0 || idx >= len(arr) {
		return nil, fmt.Errorf("json: index %d out of range (len=%d)", step.index, len(arr))
	}
	return arr[idx], nil
}

// wildcardChildren returns all values of an object or all items of an
// array. For scalars it returns nil.
func wildcardChildren(cur any) []any {
	switch v := cur.(type) {
	case map[string]any:
		out := make([]any, 0, len(v))
		for _, val := range v {
			out = append(out, val)
		}
		return out
	case []any:
		return v
	default:
		return nil
	}
}

// applySlice returns a sub-slice of an array. Negative start/end are
// interpreted relative to the end (Python-style). An end of math.MaxInt32
// (the open-ended sentinel) is clamped to len(arr).
func applySlice(cur any, start, end int) (any, error) {
	arr, ok := cur.([]any)
	if !ok {
		return nil, fmt.Errorf("json: cannot slice %T", cur)
	}
	n := len(arr)
	if start < 0 {
		start = n + start
	}
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	// Clamp open-ended sentinel before negative-index adjustment.
	if end > n {
		end = n
	}
	if end < 0 {
		end = n + end
	}
	if end < 0 {
		end = 0
	}
	if end > n {
		end = n
	}
	if end < start {
		end = start
	}
	out := make([]any, end-start)
	copy(out, arr[start:end])
	return out, nil
}

// JSONProcess is the top-level entry: parse the JSON, optionally apply
// a JSONPath query, and return the result.
func JSONProcess(document, path, op string) (any, error) {
	var data any
	if err := json.Unmarshal([]byte(document), &data); err != nil {
		return nil, fmt.Errorf("json: parse error: %w", err)
	}
	if op == string(JSONOpParse) {
		return data, nil
	}
	// Default op is "query"; an empty path returns the root document.
	if path == "" {
		return data, nil
	}
	return JSONQuery(data, path)
}

func (jsonProcessorExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	document, _ := input["json"].(string)
	if document == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "json: json document is required"}, nil
	}
	path, _ := input["path"].(string)
	op, _ := input["op"].(string)
	result, err := JSONProcess(document, path, op)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: err.Error()}, nil
	}
	return &action.ActionResult{OK: true, Status: "success", Result: result}, nil
}

// NewJSONProcessorAction builds an Action that parses JSON and
// optionally applies a JSONPath-style query.
func NewJSONProcessorAction() *action.Action {
	return &action.Action{
		Name:        JSONProcessorActionID,
		Description: "Parse JSON and extract values via JSONPath.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"json": map[string]any{"type": "string"},
				"path": map[string]any{"type": "string"},
				"op":   map[string]any{"type": "string"},
			},
			"required": []string{"json"},
		},
		Executor: jsonProcessorExecutor{},
		Tags:     []string{"json", "data", "builtin"},
	}
}

// isJSONIdentChar reports whether c may appear unquoted inside a bare
// JSONPath field name. Kept for completeness / future tokenization.
func isJSONIdentChar(c byte) bool {
	return c == '_' || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c))
}
