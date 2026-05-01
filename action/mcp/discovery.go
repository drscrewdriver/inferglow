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
