// Package actions provides InferGlow's built-in Action implementations.
//
// Each Action in this package wraps a concrete capability (calculator,
// web search, URL fetch, file I/O, code execution, JSON processing)
// behind the action.ActionExecutor contract and declares an ActionSpec
// with the appropriate SideEffectLevel / ApprovalRequired / SandboxRequired
// fields so the runtime can apply the correct safety gates.
package actions

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"

	"github.com/inferglow/action"
)

// CalculatorActionID is the registered Action name for the calculator.
const CalculatorActionID = "calculator"

// CalculateInput is the strongly-typed input for the calculator Action.
type CalculateInput struct {
	Expression string `json:"expression"`
}

// Calculate evaluates a mathematical expression and returns its numeric
// result.
//
// Supported operators: + - * / %, unary -, parentheses, and ** (power).
// The expression is parsed with go/parser and walked with go/ast — no
// eval or reflect-based execution is performed, so the input cannot
// invoke functions, access variables, or escape the arithmetic grammar.
func Calculate(expression string) (float64, error) {
	if expression == "" {
		return 0, fmt.Errorf("calculator: expression is empty")
	}
	// Rewrite the power operator (** is not valid Go syntax) to XOR (^),
	// which go/parser accepts and the evaluator reinterprets as math.Pow.
	rewritten, err := rewritePower(expression)
	if err != nil {
		return 0, err
	}
	return evalGoExpr(rewritten)
}

// rewritePower converts "**" into "^" so the expression can be parsed by
// go/parser. It rejects malformed sequences such as trailing "**".
func rewritePower(expr string) (string, error) {
	var out []byte
	for i := 0; i < len(expr); i++ {
		if i+1 < len(expr) && expr[i] == '*' && expr[i+1] == '*' {
			out = append(out, '^')
			i++
			continue
		}
		out = append(out, expr[i])
	}
	return string(out), nil
}

// evalGoExpr parses expr as a Go expression and evaluates it recursively.
//
// Only the following AST nodes are accepted:
//   - *ast.BasicLit       (numeric literals)
//   - *ast.ParenExpr      (grouped sub-expressions)
//   - *ast.UnaryExpr      (unary + / -)
//   - *ast.BinaryExpr     (+ - * / % ^)
//
// Any other node (identifiers, calls, selectors, composite literals, …)
// is rejected so the expression grammar stays firmly arithmetic.
func evalGoExpr(expr string) (float64, error) {
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, fmt.Errorf("calculator: parse error: %w", err)
	}
	return walkAST(node)
}

// walkAST recursively evaluates an ast.Node into a float64.
func walkAST(node ast.Node) (float64, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		v, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return 0, fmt.Errorf("calculator: invalid literal %q: %w", n.Value, err)
		}
		return v, nil
	case *ast.ParenExpr:
		return walkAST(n.X)
	case *ast.UnaryExpr:
		v, err := walkAST(n.X)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.SUB:
			return -v, nil
		case token.ADD:
			return v, nil
		default:
			return 0, fmt.Errorf("calculator: unsupported unary operator %q", n.Op)
		}
	case *ast.BinaryExpr:
		lhs, err := walkAST(n.X)
		if err != nil {
			return 0, err
		}
		rhs, err := walkAST(n.Y)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.ADD:
			return lhs + rhs, nil
		case token.SUB:
			return lhs - rhs, nil
		case token.MUL:
			return lhs * rhs, nil
		case token.QUO:
			if rhs == 0 {
				return 0, fmt.Errorf("calculator: division by zero")
			}
			return lhs / rhs, nil
		case token.REM:
			if rhs == 0 {
				return 0, fmt.Errorf("calculator: modulo by zero")
			}
			return math.Mod(lhs, rhs), nil
		case token.XOR:
			// Reinterpreted from "**" power rewrite.
			return math.Pow(lhs, rhs), nil
		default:
			return 0, fmt.Errorf("calculator: unsupported binary operator %q", n.Op)
		}
	default:
		return 0, fmt.Errorf("calculator: unsupported expression node %T", node)
	}
}

// CalculatorSpec is the ActionSpec declaring the calculator's safety
// properties: no side effects, no approval, no sandbox.
var CalculatorSpec = &action.ActionSpec{
	ActionID:         CalculatorActionID,
	Name:             "Calculator",
	Description:      "Evaluate a mathematical expression (add, subtract, multiply, divide, modulo, power, parentheses).",
	SideEffectLevel:  action.SideEffectNone,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"math", "builtin"},
	Kwargs: map[string]any{
		"expression": map[string]any{"type": "string", "required": true},
	},
	Returns: map[string]any{"type": "number"},
}

// calculatorExecutor adapts Calculate to the action.ActionExecutor
// interface. It is exported indirectly via NewCalculatorAction so
// callers can register it with an ActionRegistry.
type calculatorExecutor struct{}

func (calculatorExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	expr, _ := input["expression"].(string)
	result, err := Calculate(expr)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  err.Error(),
		}, nil
	}
	return &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: result,
	}, nil
}

// NewCalculatorAction builds a registered-ready Action for the
// calculator built-in.
func NewCalculatorAction() *action.Action {
	return &action.Action{
		Name:        CalculatorActionID,
		Description: "Evaluate a mathematical expression.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{"type": "string"},
			},
			"required": []string{"expression"},
		},
		Executor: calculatorExecutor{},
		Tags:     []string{"math", "builtin"},
	}
}
