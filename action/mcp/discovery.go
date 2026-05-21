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

package mcp

import "context"

// DiscoverAll retrieves every tool exposed by the server reachable
// through client and returns them in the order the server reported
// them.
//
// This is the lower-level half of MCP tool discovery: it returns the
// raw Tool descriptors without wrapping them in action.Action. The
// action package's DiscoverMCPTools function consumes this list,
// invokes action.NewMCPAction for each tool, and registers the
// resulting Actions. The split exists to avoid an import cycle:
// the action package imports mcp for *Client / *Tool, so mcp
// cannot import action for *Action.
//
// The returned slice borrows from the client's ListTools response
// and is safe for the caller to retain.
func DiscoverAll(ctx context.Context, client *Client) ([]*Tool, error) {
	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Tool, 0, len(tools))
	for i := range tools {
		out = append(out, &tools[i])
	}
	return out, nil
}
