package action

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// Signature type identifiers supported by LocalFunctionExecutor.
const (
	sigCtxInpOutErr = 1 // func(ctx, InputT) (OutputT, error)
	sigInpOutErr    = 2 // func(InputT) (OutputT, error)
	sigCtxInpOut    = 3 // func(ctx, InputT) OutputT
)

var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// LocalFunctionExecutor wraps an ordinary Go function as an ActionExecutor.
//
// It supports three signatures (identified by sigType):
//
//   1. func(ctx context.Context, in InputT) (OutputT, error)
//   2. func(in InputT) (OutputT, error)
//   3. func(ctx context.Context, in InputT) OutputT
//
// At Execute time the input map[string]any is converted to InputT via
// JSON marshal/unmarshal, and any panic is recovered into an error-shaped
// ActionResult.
type LocalFunctionExecutor struct {
	fn        any
	inputType reflect.Type // resolved InputT (first non-context In parameter)
	sigType   int           // 1 / 2 / 3
}

// New builds an Action whose Executor wraps fn.
//
// fn must match one of the three supported signatures; otherwise
// ErrUnsupportedFunctionSignature is returned. A best-effort JSON Schema
// is generated from InputT (struct fields derive names from `json` tags).
func New(name, description string, fn any) (*Action, error) {
	if fn == nil {
		return nil, fmt.Errorf("%w: fn is nil", ErrUnsupportedFunctionSignature)
	}
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("%w: expected func, got %s", ErrUnsupportedFunctionSignature, t.Kind())
	}

	sigType, inputType, err := classifySignature(t)
	if err != nil {
		return nil, err
	}

	schema := buildSchema(inputType)

	return &Action{
		Name:        name,
		Description: description,
		Schema:      schema,
		Executor: &LocalFunctionExecutor{
			fn:        fn,
			inputType: inputType,
			sigType:   sigType,
		},
	}, nil
}

// classifySignature inspects a func Type and decides which of the three
// supported signatures it matches, returning the sigType constant, the
// resolved InputT, or an error.
func classifySignature(t reflect.Type) (int, reflect.Type, error) {
	numIn := t.NumIn()
	numOut := t.NumOut()

	switch {
	case numIn == 2 && numOut == 2:
		// sigCtxInpOutErr: func(ctx, InputT) (OutputT, error)
		if !t.In(0).Implements(contextType) {
			return 0, nil, fmt.Errorf("%w: first parameter must be context.Context", ErrUnsupportedFunctionSignature)
		}
		if !t.Out(1).Implements(errorType) {
			return 0, nil, fmt.Errorf("%w: second return value must be error", ErrUnsupportedFunctionSignature)
		}
		return sigCtxInpOutErr, t.In(1), nil

	case numIn == 1 && numOut == 2:
		// sigInpOutErr: func(InputT) (OutputT, error)
		if !t.Out(1).Implements(errorType) {
			return 0, nil, fmt.Errorf("%w: second return value must be error", ErrUnsupportedFunctionSignature)
		}
		return sigInpOutErr, t.In(0), nil

	case numIn == 2 && numOut == 1:
		// sigCtxInpOut: func(ctx, InputT) OutputT
		if !t.In(0).Implements(contextType) {
			return 0, nil, fmt.Errorf("%w: first parameter must be context.Context", ErrUnsupportedFunctionSignature)
		}
		return sigCtxInpOut, t.In(1), nil

	default:
		return 0, nil, fmt.Errorf(
			"%w: expected func(ctx, InputT) (OutputT, error) | func(InputT) (OutputT, error) | func(ctx, InputT) OutputT, got NumIn=%d NumOut=%d",
			ErrUnsupportedFunctionSignature, numIn, numOut,
		)
	}
}

// Execute adapts a map[string]any input to the wrapped fn's signature,
// invokes it via reflection, and converts the outcome (including panics)
// into a structured ActionResult.
func (e *LocalFunctionExecutor) Execute(ctx context.Context, input map[string]any) (*ActionResult, error) {
	if e == nil || e.fn == nil {
		return &ActionResult{
			OK:     false,
			Status: "error",
			Error:  "executor not initialized",
		}, nil
	}

	// Decode map[string]any into a strongly-typed InputT instance via JSON.
	inputValue, err := decodeInput(input, e.inputType)
	if err != nil {
		return &ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("decode input: %s", err.Error()),
		}, nil
	}

	// Build reflect.Value call arguments based on signature.
	var args []reflect.Value
	switch e.sigType {
	case sigCtxInpOutErr, sigCtxInpOut:
		args = []reflect.Value{reflect.ValueOf(ctx), inputValue}
	case sigInpOutErr:
		args = []reflect.Value{inputValue}
	default:
		return &ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("unknown signature type: %d", e.sigType),
		}, nil
	}

	fnValue := reflect.ValueOf(e.fn)

	var outputs []reflect.Value
	var panicked bool
	var panicMsg string
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicMsg = fmt.Sprintf("%v", r)
			}
		}()
		outputs = fnValue.Call(args)
	}()

	if panicked {
		return &ActionResult{
			OK:     false,
			Status: "error",
			Error:  "panic: " + panicMsg,
		}, nil
	}

	switch e.sigType {
	case sigCtxInpOutErr, sigInpOutErr:
		// outputs[0] = OutputT, outputs[1] = error
		outVal := outputs[0].Interface()
		errVal := outputs[1].Interface()
		if errVal != nil {
			if err, ok := errVal.(error); ok {
				return &ActionResult{
					OK:     false,
					Status: "error",
					Error:  err.Error(),
				}, nil
			}
			return &ActionResult{
				OK:     false,
				Status: "error",
				Error:  fmt.Sprintf("%v", errVal),
			}, nil
		}
		return &ActionResult{
			OK:     true,
			Status: "success",
			Result: outVal,
		}, nil

	case sigCtxInpOut:
		// No error return value — always success.
		return &ActionResult{
			OK:     true,
			Status: "success",
			Result: outputs[0].Interface(),
		}, nil
	}

	return &ActionResult{
		OK:     false,
		Status: "error",
		Error:  fmt.Sprintf("unknown signature type: %d", e.sigType),
	}, nil
}

// decodeInput marshals the input map to JSON and unmarshals it into a fresh
// instance of inputType. Non-struct input types (e.g. map/slice/scalar)
// are still supported via the same JSON round-trip.
func decodeInput(input map[string]any, inputType reflect.Type) (reflect.Value, error) {
	target := reflect.New(inputType).Interface()
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("marshal input: %w", err)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return reflect.Value{}, fmt.Errorf("unmarshal input: %w", err)
		}
	}
	return reflect.ValueOf(target).Elem(), nil
}

// buildSchema produces a best-effort JSON Schema describing inputType.
//
// Struct types generate an "object" schema whose "properties" are derived
// from exported fields and their `json` tags. All other types fall back to
// the placeholder {"type": "object"}.
func buildSchema(inputType reflect.Type) map[string]any {
	if inputType == nil {
		return map[string]any{"type": "object"}
	}
	t := inputType
	// Dereference pointer types so *InputT is treated as InputT.
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return map[string]any{"type": "object"}
	}

	properties := make(map[string]any)
	var required []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("json")
		if name == "" {
			name = field.Name
		} else {
			// Strip ",omitempty" / ",string" options from json tag.
			for i := 0; i < len(name); i++ {
				if name[i] == ',' {
					name = name[:i]
					break
				}
			}
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
		}
		properties[name] = jsonTypeOf(field.Type)
		required = append(required, name)
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// jsonTypeOf maps a reflect.Type to a minimal JSON Schema fragment.
func jsonTypeOf(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// []byte marshals as a base64 string.
			return map[string]any{"type": "string"}
		}
		return map[string]any{
			"type":  "array",
			"items": jsonTypeOf(t.Elem()),
		}
	case reflect.Map:
		return map[string]any{
			"type": "object",
			"additionalProperties": jsonTypeOf(t.Elem()),
		}
	case reflect.Struct:
		return buildSchema(t)
	case reflect.Interface:
		return map[string]any{}
	default:
		return map[string]any{"type": "object"}
	}
}
