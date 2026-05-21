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

package action

import (
	"context"
	"errors"
	"fmt"

	"github.com/inferglow/action/mcp"
)

// mcpToolCaller is the subset of *mcp.Client that MCPExecutor needs.
// It is defined as an interface so tests in this package can swap in
// a stub caller without spinning up a real mcp.Client + Transport.
type mcpToolCaller interface {
	CallTool(ctx context.Context, name string, arguments map[string]any) ([]mcp.Content, error)
}

// MCPExecutor is an ActionExecutor that proxies Execute calls to an
// MCP server via the tools/call method.
//
// Each MCPExecutor is bound to a single tool name; the binding is
// established once by NewMCPExecutor (or NewMCPAction) and reused for
// every subsequent Execute. The MCP response content array is mapped
// to an ActionResult per the spec:
//
//   - TextContent  → ActionResult.Result is the text string.
//   - ImageContent → ActionResult.Result is the base64 payload.
//   - ResourceLink → collected into ActionResult.Metadata["resource_links"].
type MCPExecutor struct {
	caller   mcpToolCaller
	toolName string
}

// NewMCPExecutor binds client to toolName and returns an executor
// whose Execute calls issue tools/call against that tool.
//
// client must be non-nil; toolName must be non-empty. The returned
// executor is safe for concurrent use because the underlying Client
// is.
func NewMCPExecutor(client *mcp.Client, toolName string) *MCPExecutor {
	if client == nil {
		panic("mcp: NewMCPExecutor called with nil client")
	}
	if toolName == "" {
		panic("mcp: NewMCPExecutor called with empty toolName")
	}
	return &MCPExecutor{caller: client, toolName: toolName}
}

// Execute invokes the bound MCP tool with input as the arguments
// map and translates the returned Content array into an ActionResult.
//
// If the underlying CallTool fails (transport error, RPC error, etc.)
// the executor returns an error-shaped ActionResult and a nil error —
// mirroring the convention used by LocalFunctionExecutor and the
// SandboxExecutor so registry.Execute always produces a structured
// result.
func (e *MCPExecutor) Execute(ctx context.Context, input map[string]any) (*ActionResult, error) {
	if e == nil || e.caller == nil {
		return &ActionResult{
			OK:     false,
			Status: "error",
			Error:  "mcp executor not initialized",
		}, nil
	}

	contents, err := e.caller.CallTool(ctx, e.toolName, input)
	if err != nil {
		return &ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("mcp call %q: %s", e.toolName, err.Error()),
		}, nil
	}

	var text string
	var imageB64 string
	var resourceLinks []map[string]string
	for _, c := range contents {
		switch c.Type {
		case "text":
			if text == "" {
				// Prefer the first text content; subsequent text
				// blocks are concatenated to avoid silent loss.
				text = c.Text
			} else {
				text += "\n" + c.Text
			}
		case "image":
			// c.Data is already base64-encoded per MCP spec.
			if imageB64 == "" {
				imageB64 = c.Data
			}
		case "resource_link":
			if resourceLinks == nil {
				resourceLinks = make([]map[string]string, 0, 4)
			}
			resourceLinks = append(resourceLinks, map[string]string{
				"uri":  c.URI,
				"name": c.Name,
			})
		}
	}

	// Pick the primary Result payload: text takes precedence over
	// image so that mixed responses surface a human-readable string.
	var primary any
	if text != "" {
		primary = text
	} else if imageB64 != "" {
		primary = imageB64
	}

	res := &ActionResult{
		OK:     true,
		Status: "success",
		Result: primary,
	}
	if len(resourceLinks) > 0 {
		res.Metadata = map[string]any{"resource_links": resourceLinks}
	}
	return res, nil
}

// NewMCPAction wraps an MCP Tool as an Action suitable for
// registration with an ActionRegistry.
//
// The returned Action carries a copy of tool.InputSchema so later
// mutations to the Tool descriptor do not leak into the Action.
// tool must be non-nil.
func NewMCPAction(client *mcp.Client, tool *mcp.Tool) (*Action, error) {
	if client == nil {
		return nil, errors.New("action: NewMCPAction called with nil client")
	}
	if tool == nil {
		return nil, errors.New("action: NewMCPAction called with nil tool")
	}
	if tool.Name == "" {
		return nil, errors.New("action: NewMCPAction called with tool.Name empty")
	}

	schema := copySchema(tool.InputSchema)
	return &Action{
		Name:        tool.Name,
		Description: tool.Description,
		Schema:      schema,
		Executor:    &MCPExecutor{caller: client, toolName: tool.Name},
	}, nil
}

// copySchema returns a deep-enough copy of in: a fresh top-level map
// with freshly-allocated child maps for known keys. We don't need a
// generic deep-copy because the JSON Schema fragment produced by MCP
// servers only ever contains JSON-shaped data (maps / slices /
// scalars) and we serialize through JSON whenever it matters. The
// intent of the copy is just to prevent callers from accidentally
// mutating the schema after registration via the same pointer.
func copySchema(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// DiscoverMCPTools lists every tool exposed by client, wraps each as
// an Action via NewMCPAction, and registers them with registry.
//
// Returns the sorted list of registered Action names. Registration
// failures (duplicate name, etc.) abort the loop and return the
// error along with the names that were registered successfully
// before the failure.
func DiscoverMCPTools(ctx context.Context, client *mcp.Client, registry *ActionRegistry) ([]string, error) {
	tools, err := mcp.DiscoverAll(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("action: discover mcp tools: %w", err)
	}

	registered := make([]string, 0, len(tools))
	for _, tool := range tools {
		a, err := NewMCPAction(client, tool)
		if err != nil {
			return registered, fmt.Errorf("action: build mcp action for %q: %w", tool.Name, err)
		}
		if err := registry.Register(a); err != nil {
			return registered, fmt.Errorf("action: register mcp action %q: %w", tool.Name, err)
		}
		registered = append(registered, a.Name)
	}
	return registered, nil
}
