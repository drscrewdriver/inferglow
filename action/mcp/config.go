package mcp

import "errors"

// ErrUnsupportedTransport is returned by NewTransportFromConfig when
// cfg.Transport names a transport this package does not implement.
//
// P0 supports only "stdio"; HTTP and SSE are P1.
var ErrUnsupportedTransport = errors.New("mcp: unsupported transport")

// NewTransportFromConfig builds a Transport from an MCPServerConfig.
//
// In P0 only the "stdio" transport is implemented: it returns a
// freshly-constructed *StdioTransport wired to cfg.Command / cfg.Args
// / cfg.Env. Callers must still invoke Start on the returned
// transport before using it.
func NewTransportFromConfig(cfg MCPServerConfig) (Transport, error) {
	switch cfg.Transport {
	case "stdio", "":
		// Treat empty Transport as stdio to make hand-written
		// configs more forgiving.
		return &StdioTransport{
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
		}, nil
	default:
		return nil, ErrUnsupportedTransport
	}
}
