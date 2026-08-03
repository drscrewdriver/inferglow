package flowdef

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"text/template"
)

// evalWhen evaluates a `when` template expression against data and returns
// whether the condition is true. An empty expression always evaluates to
// true (i.e. the step always runs).
//
// The expression uses Go text/template syntax with these custom functions:
//   - all:  true if every element of a slice is truthy
//   - any:  true if at least one element of a slice is truthy
//   - len:  length of a slice/map/string (built-in)
//   - eq:   equality (built-in)
//   - ne:   inequality (built-in)
//   - gt:   greater-than (built-in)
//
// The template is executed and its textual output is interpreted: "true"
// (case-insensitive, trimmed) means true, anything else means false.
func evalWhen(expr string, data map[string]any) (bool, error) {
	if strings.TrimSpace(expr) == "" {
		return true, nil
	}
	tmpl, err := template.New("when").Funcs(whenFuncs()).Parse(expr)
	if err != nil {
		return false, fmt.Errorf("flowdef: parse when expression %q: %w", expr, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return false, fmt.Errorf("flowdef: eval when expression %q: %w", expr, err)
	}
	out := strings.TrimSpace(buf.String())
	return strings.EqualFold(out, "true"), nil
}

// whenFuncs returns the custom template function map used by evalWhen.
func whenFuncs() template.FuncMap {
	return template.FuncMap{
		"all": func(v any) bool {
			rv := reflect.ValueOf(v)
			if rv.Kind() != reflect.Slice {
				return false
			}
			for i := 0; i < rv.Len(); i++ {
				if !truthyValue(rv.Index(i)) {
					return false
				}
			}
			return true
		},
		"any": func(v any) bool {
			rv := reflect.ValueOf(v)
			if rv.Kind() != reflect.Slice {
				return false
			}
			for i := 0; i < rv.Len(); i++ {
				if truthyValue(rv.Index(i)) {
					return true
				}
			}
			return false
		},
	}
}

// truthyValue reports whether a reflect.Value is truthy. Bools use their
// own value; nil/zero values are false; everything else is true.
func truthyValue(rv reflect.Value) bool {
	if !rv.IsValid() {
		return false
	}
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.String:
		s := rv.String()
		return s != "" && s != "false"
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return false
		}
		return truthyValue(rv.Elem())
	default:
		return true
	}
}
