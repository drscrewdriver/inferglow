package mcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// StdioTransport is a Transport that speaks newline-delimited
// JSON-RPC 2.0 over the stdin/stdout pipes of a child process.
//
// It is the only Transport implementation provided by this package
// and is the foundation for connecting to MCP servers launched as
// local subprocesses (e.g. `npx -y @modelcontextprotocol/server-filesystem /tmp`).
type StdioTransport struct {
	// Command is the executable to run (e.g. "npx", "node", "python").
	Command string
	// Args is the argument list passed to Command.
	Args []string
	// Env, if non-nil, replaces the child process environment.
	// Strings are in the form "KEY=VALUE".
	Env []string

	startOnce sync.Once
	stopOnce  sync.Once

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader
	reader *bufio.Reader
}

// Start launches the configured child process and stores the stdin
// and stdout pipes used by subsequent Send / Recv calls.
//
// The provided ctx is used as the command context via
// exec.CommandContext: if ctx is canceled, the child process is
// signaled to terminate. Start itself does not block on the process
// lifetime — that is the responsibility of Stop.
func (t *StdioTransport) Start(ctx context.Context) error {
	if t.Command == "" {
		return errors.New("mcp: StdioTransport.Command is empty")
	}
	if t.cmd != nil {
		return errors.New("mcp: transport already started")
	}

	cmd := exec.CommandContext(ctx, t.Command, t.Args...)
	if t.Env != nil {
		cmd.Env = t.Env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.reader = bufio.NewReaderSize(stdout, 1024*1024) // 1 MiB max line
	return nil
}

// Send writes msg followed by a newline to the child process stdin.
// The write is flushed before returning because the pipe is unbuffered.
func (t *StdioTransport) Send(ctx context.Context, msg []byte) error {
	if t.stdin == nil {
		return errors.New("mcp: transport not started")
	}
	// Abort early if the caller has already given up.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if _, err := t.stdin.Write(append(msg, '\n')); err != nil {
		return err
	}
	return nil
}

// Recv reads a single newline-terminated line from the child process
// stdout and returns it without the trailing newline. ctx cancellation
// aborts a pending read: the underlying pipe read runs in a goroutine
// and the call returns ctx.Err() as soon as ctx is done. The leaked
// reader goroutine exits when stdout is closed by Stop.
func (t *StdioTransport) Recv(ctx context.Context) ([]byte, error) {
	if t.reader == nil {
		return nil, errors.New("mcp: transport not started")
	}
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			if line != "" {
				// A final partial line without newline is still
				// meaningful — return it alongside the error so the
				// caller can drain buffered output before EOF.
				ch <- result{[]byte(strings.TrimRight(line, "\r\n")), err}
				return
			}
			ch <- result{nil, err}
			return
		}
		ch <- result{[]byte(strings.TrimRight(line, "\r\n")), nil}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.data, r.err
	}
}

// Stop closes the child process stdin and waits for it to exit,
// bounded by a 5 second timeout derived from ctx. If the process
// does not exit within the timeout it is killed and the timeout
// error is returned. Calling Stop more than once is a no-op.
func (t *StdioTransport) Stop(ctx context.Context) error {
	if t.cmd == nil {
		return nil
	}

	var stopErr error
	t.stopOnce.Do(func() {
		if t.stdin != nil {
			_ = t.stdin.Close()
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- t.cmd.Wait()
		}()

		select {
		case <-waitCtx.Done():
			_ = t.cmd.Process.Kill()
			<-done
			stopErr = waitCtx.Err()
		case err := <-done:
			stopErr = err
		}
	})
	return stopErr
}
