package tools

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func addInts(a, b int) int { return a + b }

func TestSchemaFromFunc_BasicTypes(t *testing.T) {
	td, err := SchemaFromFunc(addInts, "add", "Compute sum")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	if td.Name != "add" {
		t.Errorf("Name = %q, want %q", td.Name, "add")
	}
	if td.Description != "Compute sum" {
		t.Errorf("Description = %q, want %q", td.Description, "Compute sum")
	}
	if td.Parameters["type"] != "object" {
		t.Errorf("type = %v, want object", td.Parameters["type"])
	}
	props, ok := td.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties not map[string]any: %T", td.Parameters["properties"])
	}
	for _, pname := range []string{"arg0", "arg1"} {
		p, ok := props[pname]
		if !ok {
			t.Errorf("missing property %q", pname)
			continue
		}
		ps, ok := p.(map[string]any)
		if !ok {
			t.Errorf("property %q not map[string]any: %T", pname, p)
			continue
		}
		if ps["type"] != "integer" {
			t.Errorf("property %q type = %v, want integer", pname, ps["type"])
		}
	}
	required, ok := td.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("required not []string: %T", td.Parameters["required"])
	}
	if len(required) != 2 {
		t.Errorf("len(required) = %d, want 2", len(required))
	}
}

func TestSchemaFromFunc_MixedBasicTypes(t *testing.T) {
	type fnSig func(string, int, float64, bool) (string, error)
	var fn fnSig = func(s string, n int, f float64, b bool) (string, error) { return s, nil }

	td, err := SchemaFromFunc(fn, "mixed", "mixed types")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)

	cases := []struct {
		name string
		want string
	}{
		{"arg0", "string"},
		{"arg1", "integer"},
		{"arg2", "number"},
		{"arg3", "boolean"},
	}
	for _, c := range cases {
		p, ok := props[c.name].(map[string]any)
		if !ok {
			t.Errorf("property %q missing or wrong type: %T", c.name, props[c.name])
			continue
		}
		if p["type"] != c.want {
			t.Errorf("property %q type = %v, want %v", c.name, p["type"], c.want)
		}
	}
}

func TestSchemaFromFunc_ContextSkip(t *testing.T) {
	type fnSig func(context.Context, string) error
	var fn fnSig = func(ctx context.Context, s string) error { return nil }

	td, err := SchemaFromFunc(fn, "ctx_fn", "has context")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	if _, exists := props["arg0"]; !exists {
		t.Errorf("context parameter should be skipped; expected arg0 to be the string param")
	}
	if p, ok := props["arg0"].(map[string]any); ok {
		if p["type"] != "string" {
			t.Errorf("arg0 type = %v, want string (context should be skipped)", p["type"])
		}
	}
	required := td.Parameters["required"].([]string)
	if len(required) != 1 {
		t.Errorf("len(required) = %d, want 1 (context skipped)", len(required))
	}
}

func TestSchemaFromFunc_StructParam(t *testing.T) {
	type Address struct {
		Street string `json:"street"`
		City   string `json:"city"`
	}
	type Person struct {
		Name    string  `json:"name"`
		Age     int     `json:"age"`
		Address Address `json:"address"`
	}

	fn := func(p Person) error { return nil }

	td, err := SchemaFromFunc(fn, "create_person", "create a person")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	p0, ok := props["arg0"].(map[string]any)
	if !ok {
		t.Fatalf("arg0 missing or wrong type: %T", props["arg0"])
	}
	if p0["type"] != "object" {
		t.Errorf("arg0 type = %v, want object", p0["type"])
	}
	p0Props, ok := p0["properties"].(map[string]any)
	if !ok {
		t.Fatalf("arg0.properties missing or wrong type: %T", p0["properties"])
	}

	nameField, ok := p0Props["name"].(map[string]any)
	if !ok {
		t.Fatalf("name field missing")
	}
	if nameField["type"] != "string" {
		t.Errorf("name type = %v, want string", nameField["type"])
	}

	ageField, ok := p0Props["age"].(map[string]any)
	if !ok {
		t.Fatalf("age field missing")
	}
	if ageField["type"] != "integer" {
		t.Errorf("age type = %v, want integer", ageField["type"])
	}

	addrField, ok := p0Props["address"].(map[string]any)
	if !ok {
		t.Fatalf("address field missing")
	}
	if addrField["type"] != "object" {
		t.Errorf("address type = %v, want object", addrField["type"])
	}
	addrProps, ok := addrField["properties"].(map[string]any)
	if !ok {
		t.Fatalf("address.properties missing")
	}
	if _, ok := addrProps["street"]; !ok {
		t.Errorf("address.properties missing street")
	}
	if _, ok := addrProps["city"]; !ok {
		t.Errorf("address.properties missing city")
	}

	p0Required, ok := p0["required"].([]string)
	if !ok {
		t.Fatalf("arg0.required missing or wrong type")
	}
	if len(p0Required) != 3 {
		t.Errorf("len(arg0.required) = %d, want 3", len(p0Required))
	}
}

func TestSchemaFromFunc_SliceParam(t *testing.T) {
	fn := func(items []string) string { return "" }

	td, err := SchemaFromFunc(fn, "join", "join strings")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	p0, ok := props["arg0"].(map[string]any)
	if !ok {
		t.Fatalf("arg0 missing")
	}
	if p0["type"] != "array" {
		t.Errorf("arg0 type = %v, want array", p0["type"])
	}
	items, ok := p0["items"].(map[string]any)
	if !ok {
		t.Fatalf("arg0.items missing")
	}
	if items["type"] != "string" {
		t.Errorf("arg0.items.type = %v, want string", items["type"])
	}
}

func TestSchemaFromFunc_ErrorReturnSkipped(t *testing.T) {
	fn := func(s string) (string, error) { return s, nil }

	td, err := SchemaFromFunc(fn, "echo", "echo string")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	if len(props) != 1 {
		t.Errorf("len(properties) = %d, want 1 (return values should not appear)", len(props))
	}
	if _, ok := props["arg0"]; !ok {
		t.Errorf("arg0 missing")
	}
}

func TestSchemaFromFunc_MultipleReturns(t *testing.T) {
	fn := func(s string) (string, int, error) { return s, 0, nil }

	td, err := SchemaFromFunc(fn, "multi", "multi return")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	if len(props) != 1 {
		t.Errorf("len(properties) = %d, want 1 (return values should not appear)", len(props))
	}
}

func TestSchemaFromFunc_WithDocstringParams(t *testing.T) {
	doc := "Add computes the sum of two integers.\n@param a the first integer\n@param b the second integer"
	td, err := SchemaFromFunc(addInts, "add", doc)
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	if td.Description != "Add computes the sum of two integers." {
		t.Errorf("Description = %q, want summary", td.Description)
	}
	props := td.Parameters["properties"].(map[string]any)

	a, ok := props["a"].(map[string]any)
	if !ok {
		t.Fatalf("property a missing")
	}
	if a["type"] != "integer" {
		t.Errorf("a type = %v, want integer", a["type"])
	}
	if a["description"] != "the first integer" {
		t.Errorf("a description = %v, want %q", a["description"], "the first integer")
	}

	b, ok := props["b"].(map[string]any)
	if !ok {
		t.Fatalf("property b missing")
	}
	if b["type"] != "integer" {
		t.Errorf("b type = %v, want integer", b["type"])
	}
	if b["description"] != "the second integer" {
		t.Errorf("b description = %v, want %q", b["description"], "the second integer")
	}

	required := td.Parameters["required"].([]string)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(required, want) {
		t.Errorf("required = %v, want %v", required, want)
	}
}

func TestSchemaFromFunc_NilFn(t *testing.T) {
	_, err := SchemaFromFunc(nil, "nil_fn", "")
	if err == nil {
		t.Errorf("expected error for nil fn")
	}
}

func TestSchemaFromFunc_NonFunc(t *testing.T) {
	_, err := SchemaFromFunc(42, "not_fn", "")
	if err == nil {
		t.Errorf("expected error for non-func")
	}
}

func TestSchemaFromFunc_NoParams(t *testing.T) {
	fn := func() (int, error) { return 0, nil }
	td, err := SchemaFromFunc(fn, "noargs", "no args")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	if len(props) != 0 {
		t.Errorf("len(properties) = %d, want 0", len(props))
	}
	if _, hasRequired := td.Parameters["required"]; hasRequired {
		t.Errorf("required should be absent when there are no params")
	}
}

func TestParseDocstring_Summary(t *testing.T) {
	doc := "Add computes the sum of two integers."
	info := ParseDocstring(doc)
	if info.Summary != "Add computes the sum of two integers." {
		t.Errorf("Summary = %q, want %q", info.Summary, "Add computes the sum of two integers.")
	}
	if info.Description != "Add computes the sum of two integers." {
		t.Errorf("Description = %q, want %q", info.Description, "Add computes the sum of two integers.")
	}
}

func TestParseDocstring_SummaryAndDescription(t *testing.T) {
	doc := "Add computes the sum of two integers.\n\nThis is a longer description.\nIt spans multiple lines."
	info := ParseDocstring(doc)
	if info.Summary != "Add computes the sum of two integers." {
		t.Errorf("Summary = %q", info.Summary)
	}
	want := "Add computes the sum of two integers.\nThis is a longer description.\nIt spans multiple lines."
	if info.Description != want {
		t.Errorf("Description = %q, want %q", info.Description, want)
	}
}

func TestParseDocstring_ParamTags(t *testing.T) {
	doc := "Add computes sum.\n@param a the first integer\n@param b the second integer"
	info := ParseDocstring(doc)
	if info.Summary != "Add computes sum." {
		t.Errorf("Summary = %q", info.Summary)
	}
	if len(info.Params) != 2 {
		t.Fatalf("len(Params) = %d, want 2", len(info.Params))
	}
	if info.Params["a"] != "the first integer" {
		t.Errorf("Params[a] = %q", info.Params["a"])
	}
	if info.Params["b"] != "the second integer" {
		t.Errorf("Params[b] = %q", info.Params["b"])
	}
	if info.Description != "Add computes sum." {
		t.Errorf("Description = %q, want summary only (param lines excluded)", info.Description)
	}
}

func TestParseDocstring_ParamNoDescription(t *testing.T) {
	doc := "Fn does something.\n@param a"
	info := ParseDocstring(doc)
	if _, ok := info.Params["a"]; !ok {
		t.Errorf("Params[a] missing")
	}
	if info.Params["a"] != "" {
		t.Errorf("Params[a] = %q, want empty", info.Params["a"])
	}
}

func TestParseDocstring_Empty(t *testing.T) {
	info := ParseDocstring("")
	if info.Summary != "" {
		t.Errorf("Summary = %q, want empty", info.Summary)
	}
	if info.Description != "" {
		t.Errorf("Description = %q, want empty", info.Description)
	}
	if len(info.Params) != 0 {
		t.Errorf("len(Params) = %d, want 0", len(info.Params))
	}
}

func TestParseDocstring_OnlyParams(t *testing.T) {
	doc := "@param a first\n@param b second"
	info := ParseDocstring(doc)
	if info.Summary != "" {
		t.Errorf("Summary = %q, want empty", info.Summary)
	}
	if len(info.Params) != 2 {
		t.Fatalf("len(Params) = %d, want 2", len(info.Params))
	}
}

func TestExtractParamDesc(t *testing.T) {
	doc := "Add computes sum.\n@param a the first integer\n@param b the second integer"
	if got := ExtractParamDesc(doc, "a"); got != "the first integer" {
		t.Errorf("ExtractParamDesc(a) = %q", got)
	}
	if got := ExtractParamDesc(doc, "b"); got != "the second integer" {
		t.Errorf("ExtractParamDesc(b) = %q", got)
	}
	if got := ExtractParamDesc(doc, "missing"); got != "" {
		t.Errorf("ExtractParamDesc(missing) = %q, want empty", got)
	}
}

func TestSchemaFromFunc_PointerParam(t *testing.T) {
	fn := func(s *string) error { return nil }
	td, err := SchemaFromFunc(fn, "ptr", "pointer param")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	p0, ok := props["arg0"].(map[string]any)
	if !ok {
		t.Fatalf("arg0 missing")
	}
	if p0["type"] != "string" {
		t.Errorf("arg0 type = %v, want string (pointer dereferenced)", p0["type"])
	}
}

func TestSchemaFromFunc_ContextNotFirst(t *testing.T) {
	fn := func(s string, ctx context.Context) error { return nil }
	td, err := SchemaFromFunc(fn, "ctx_not_first", "context not first")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	if len(props) != 2 {
		t.Errorf("len(properties) = %d, want 2 (context only skipped when first)", len(props))
	}
}

func TestSchemaFromFunc_MapParam(t *testing.T) {
	fn := func(m map[string]int) error { return nil }
	td, err := SchemaFromFunc(fn, "map_fn", "map param")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	p0, ok := props["arg0"].(map[string]any)
	if !ok {
		t.Fatalf("arg0 missing")
	}
	if p0["type"] != "object" {
		t.Errorf("arg0 type = %v, want object", p0["type"])
	}
	ap, ok := p0["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("additionalProperties missing")
	}
	if ap["type"] != "integer" {
		t.Errorf("additionalProperties.type = %v, want integer", ap["type"])
	}
}

func TestSchemaFromFunc_Int64Param(t *testing.T) {
	fn := func(n int64) error { return nil }
	td, err := SchemaFromFunc(fn, "int64_fn", "int64 param")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	p0, ok := props["arg0"].(map[string]any)
	if !ok {
		t.Fatalf("arg0 missing")
	}
	if p0["type"] != "integer" {
		t.Errorf("arg0 type = %v, want integer", p0["type"])
	}
}

func TestSchemaFromFunc_ByteSliceParam(t *testing.T) {
	fn := func(data []byte) error { return nil }
	td, err := SchemaFromFunc(fn, "bytes_fn", "bytes param")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	p0, ok := props["arg0"].(map[string]any)
	if !ok {
		t.Fatalf("arg0 missing")
	}
	if p0["type"] != "string" {
		t.Errorf("arg0 type = %v, want string ([]byte marshals as base64)", p0["type"])
	}
}

func TestSchemaFromFunc_StructWithOmitEmpty(t *testing.T) {
	type Item struct {
		Name string `json:"name"`
		Note string `json:"note,omitempty"`
	}
	fn := func(item Item) error { return nil }

	td, err := SchemaFromFunc(fn, "item", "item with omitempty")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	p0 := props["arg0"].(map[string]any)
	p0Props := p0["properties"].(map[string]any)
	if _, ok := p0Props["name"]; !ok {
		t.Errorf("name field missing")
	}
	if _, ok := p0Props["note"]; !ok {
		t.Errorf("note field missing")
	}
}

func TestSchemaFromFunc_PartialDocstringParams(t *testing.T) {
	doc := "Three params.\n@param first the first one"
	fn := func(a, b, c int) error { return nil }

	td, err := SchemaFromFunc(fn, "three", doc)
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)

	if p, ok := props["first"].(map[string]any); !ok || p["description"] != "the first one" {
		t.Errorf("first param should use @param name and description")
	}
	if _, ok := props["arg1"]; !ok {
		t.Errorf("second param should default to arg1")
	}
	if _, ok := props["arg2"]; !ok {
		t.Errorf("third param should default to arg2")
	}
}

func TestSchemaFromFunc_FullFlow(t *testing.T) {
	type GreeterInput struct {
		Name    string   `json:"name"`
		Tags    []string `json:"tags"`
		Verbose bool     `json:"verbose"`
	}

	greet := func(ctx context.Context, input GreeterInput) (string, error) {
		return "hi " + input.Name, nil
	}

	doc := "Greet generates a greeting.\n@param input the greeter configuration"

	td, err := SchemaFromFunc(greet, "greet", doc)
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	if td.Name != "greet" {
		t.Errorf("Name = %q", td.Name)
	}
	if td.Description != "Greet generates a greeting." {
		t.Errorf("Description = %q", td.Description)
	}
	props := td.Parameters["properties"].(map[string]any)

	inputSchema, ok := props["input"].(map[string]any)
	if !ok {
		t.Fatalf("input property missing")
	}
	if inputSchema["type"] != "object" {
		t.Errorf("input type = %v, want object", inputSchema["type"])
	}
	if inputSchema["description"] != "the greeter configuration" {
		t.Errorf("input description = %v", inputSchema["description"])
	}

	inputProps := inputSchema["properties"].(map[string]any)
	if inputProps["name"].(map[string]any)["type"] != "string" {
		t.Errorf("name type wrong")
	}
	if inputProps["tags"].(map[string]any)["type"] != "array" {
		t.Errorf("tags type wrong")
	}
	if inputProps["verbose"].(map[string]any)["type"] != "boolean" {
		t.Errorf("verbose type wrong")
	}

	required := td.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "input" {
		t.Errorf("required = %v, want [input]", required)
	}
}

func TestSchemaFromFunc_Variadic(t *testing.T) {
	fn := func(prefix string, parts ...string) string { return prefix }
	td, err := SchemaFromFunc(fn, "join_variadic", "join with variadic")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	if len(props) != 2 {
		t.Errorf("len(properties) = %d, want 2", len(props))
	}
	p1, ok := props["arg1"].(map[string]any)
	if !ok {
		t.Fatalf("arg1 missing")
	}
	if p1["type"] != "array" {
		t.Errorf("variadic arg type = %v, want array", p1["type"])
	}
}

func TestParseDocstring_PreservesParamOrder(t *testing.T) {
	doc := "Fn.\n@param zeta last\n@param alpha first\n@param mid middle"
	entries := parseParamEntries(doc)
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	if entries[0].Name != "zeta" || entries[1].Name != "alpha" || entries[2].Name != "mid" {
		t.Errorf("order = %v %v %v, want zeta alpha mid",
			entries[0].Name, entries[1].Name, entries[2].Name)
	}
}

func TestParseDocstring_GoStyleDocstring(t *testing.T) {
	doc := "Calculate evaluates a mathematical expression. The expression parameter is the input formula."
	info := ParseDocstring(doc)
	if info.Summary == "" {
		t.Errorf("Summary should not be empty for Go-style docstring")
	}
	if !strings.Contains(info.Summary, "Calculate") {
		t.Errorf("Summary should contain function name: %q", info.Summary)
	}
}

func TestSchemaFromFunc_OnlyErrorReturn(t *testing.T) {
	fn := func(s string) error { return nil }
	td, err := SchemaFromFunc(fn, "only_err", "only error return")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	props := td.Parameters["properties"].(map[string]any)
	if len(props) != 1 {
		t.Errorf("len(properties) = %d, want 1", len(props))
	}
}

func TestSchemaFromFunc_NoReturn(t *testing.T) {
	fn := func(s string) {}
	td, err := SchemaFromFunc(fn, "no_ret", "no return")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	if td.Name != "no_ret" {
		t.Errorf("Name = %q", td.Name)
	}
	props := td.Parameters["properties"].(map[string]any)
	if len(props) != 1 {
		t.Errorf("len(properties) = %d, want 1", len(props))
	}
}

func TestSchemaFromFunc_StandardErrorType(t *testing.T) {
	fn := func(s string) error { return errors.New("x") }
	td, err := SchemaFromFunc(fn, "std_err", "")
	if err != nil {
		t.Fatalf("SchemaFromFunc error: %v", err)
	}
	if td == nil {
		t.Fatal("td is nil")
	}
}
