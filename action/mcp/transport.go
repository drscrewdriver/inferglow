package mcp

import "context"

// Transport is the wire-level contract a Client uses to exchange
// newline-delimited JSON-RPC 2.0 frames with an MCP server.
//
// Each frame is a single JSON object serialized to one line: Send
// appends a "\n" terminator, Recv reads up to and excluding the next
// "\n". Implementations are expected to be safe for concurrent use
// by at most one reader and one writer (the Client's readLoop and
// request-sending goroutine respectively).
type Transport interface {
	// Start establishes the underlying connection / spawns the
	// subprocess. It must be idempotent-ish: calling Start on an
	// already-started transport may return an error, but must not
	// corrupt state.
	Start(ctx context.Context) error

	// Send writes a single JSON-RPC frame followed by a newline to
	// the transport's write side. It must not return before the
	// frame has been flushed.
	Send(ctx context.Context, msg []byte) error

	// Recv blocks until a complete JSON-RPC frame is available and
	// returns it without the trailing newline. ctx cancellation
	// should abort a pending read where the underlying transport
	// permits it.
	Recv(ctx context.Context) ([]byte, error)

	// Stop tears down the transport: closing stdin / sockets,
	// waiting for the subprocess to exit (with a bounded timeout),
	// and releasing any OS resources.
	Stop(ctx context.Context) error
}
