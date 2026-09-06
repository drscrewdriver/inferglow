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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package server

// Persistent PTY terminals for the webui (v2 of the terminal panel).
//
// Design mirrors DSH-better-sidebar's pty-manager: one long-lived shell
// process per workspace key; processes survive WebSocket disconnects (page
// refresh, panel switch) and reconnect to the same process; output is
// mirrored into a bounded transcript ring so a new connection replays
// history before live data.
//
// Security posture differs from the allowlisted /v1/exec: a PTY is a
// FULL-PERMISSION interactive shell confined only to its spawn directory
// (a real shell can cd anywhere). It therefore sits behind its own gate —
// Config.PTYEnabled (-pty) — on top of the global API key requirement, and
// never registers when either is missing (fail-closed, 404 like /v1/exec).
// Every spawn/exit is appended to the exec audit log.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-pty"
	"github.com/gorilla/websocket"
)

// ptyTranscriptLimit is the per-session replay bound (bytes), matching
// DSH-better-sidebar's TRANSCRIPT_LIMIT.
const ptyTranscriptLimit = 1 << 20

// ptyMaxSessions bounds concurrent live shells server-wide.
const ptyMaxSessions = 8

// ptySession is one persistent shell.
type ptySession struct {
	key  string // workspace name the session is bound to
	root string // spawn directory

	pty pty.Pty
	cmd *pty.Cmd

	mu         sync.Mutex
	transcript []byte
	exited     bool
	exitCode   int
	subs       map[chan []byte]struct{}
}

// ptyRegistry owns all live sessions.
type ptyRegistry struct {
	mu       sync.Mutex
	sessions map[string]*ptySession
	shell    string // Config.PTYShell
}

func newPtyRegistry(shell string) *ptyRegistry {
	return &ptyRegistry{sessions: make(map[string]*ptySession), shell: shell}
}

// shellCommand resolves the shell + args for a new session. Windows defaults
// to cmd.exe with an inline UTF-8 codepage switch (OEM codepages would
// otherwise garble non-ASCII output inside ConPTY); POSIX uses $SHELL or
// bash, login-style. The program is resolved to an absolute path: go-pty's
// Windows spawner joins cmd.Dir onto the program name, so a bare name that
// only exists on PATH would fail once Dir is set.
func (r *ptyRegistry) shellCommand() (string, []string) {
	resolve := func(name string) string {
		if filepath.IsAbs(name) {
			return name
		}
		if abs, err := exec.LookPath(name); err == nil {
			return abs
		}
		return name
	}
	if r.shell != "" {
		// Flag-provided shells may carry arguments ("bash -i").
		parts := strings.Fields(r.shell)
		return resolve(parts[0]), parts[1:]
	}
	if runtime.GOOS == "windows" {
		return resolve("cmd.exe"), []string{"/k", "chcp 65001>nul"}
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return resolve(shell), []string{"-i"}
}

// getOrCreate returns the live session for the workspace key, spawning a new
// shell when none exists or the previous one already exited.
func (r *ptyRegistry) getOrCreate(key, root string) (*ptySession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.sessions[key]; s != nil {
		s.mu.Lock()
		exited := s.exited
		s.mu.Unlock()
		if !exited {
			return s, nil
		}
		s.close()
		delete(r.sessions, key)
	}
	if len(r.sessions) >= ptyMaxSessions {
		return nil, fmt.Errorf("too many live terminal sessions (max %d); close one first", ptyMaxSessions)
	}
	s, err := r.spawn(key, root)
	if err != nil {
		return nil, err
	}
	r.sessions[key] = s
	return s, nil
}

func (r *ptyRegistry) spawn(key, root string) (*ptySession, error) {
	name, args := r.shellCommand()
	p, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("create pty: %w", err)
	}
	s := &ptySession{
		key:  key,
		root: root,
		pty:  p,
		subs: make(map[chan []byte]struct{}),
	}
	cmd := p.Command(name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	s.cmd = cmd
	if err := cmd.Start(); err != nil {
		p.Close()
		return nil, fmt.Errorf("spawn %s: %w", name, err)
	}
	go s.readLoop()
	go s.waitLoop()
	return s, nil
}

func (s *ptySession) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.broadcast(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *ptySession) waitLoop() {
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		code = 1
	}
	s.mu.Lock()
	s.exited = true
	s.exitCode = code
	frame := ptyEvent("exit", nil, code)
	for ch := range s.subs {
		select {
		case ch <- frame:
		default:
		}
	}
	s.mu.Unlock()
}

// broadcast appends to the transcript ring and fans the chunk out to all
// connected WebSocket clients.
func (s *ptySession) broadcast(chunk []byte) {
	s.mu.Lock()
	s.transcript = append(s.transcript, chunk...)
	if over := len(s.transcript) - ptyTranscriptLimit; over > 0 {
		s.transcript = append(s.transcript[:0], s.transcript[over:]...)
	}
	frame := ptyEvent("o", chunk, 0)
	for ch := range s.subs {
		select {
		case ch <- frame:
		default: // slow client: drop rather than block the pty reader
		}
	}
	s.mu.Unlock()
}

func (s *ptySession) replay() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, len(s.transcript))
	copy(out, s.transcript)
	return out
}

// write forwards one input chunk into the shell.
func (s *ptySession) write(data []byte) error {
	_, err := s.pty.Write(data)
	return err
}

func (s *ptySession) resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > 500 || rows > 300 {
		return fmt.Errorf("invalid size %dx%d", cols, rows)
	}
	return s.pty.Resize(cols, rows)
}

// close kills the shell process and releases the pty.
func (s *ptySession) close() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.pty.Close()
}

// kill removes the session from the registry and kills its process.
func (r *ptyRegistry) kill(key string) bool {
	r.mu.Lock()
	s := r.sessions[key]
	delete(r.sessions, key)
	r.mu.Unlock()
	if s == nil {
		return false
	}
	s.mu.Lock()
	for ch := range s.subs {
		close(ch)
		delete(s.subs, ch)
	}
	s.mu.Unlock()
	s.close()
	return true
}

// ptyEvent renders one JSON wire frame. Output/exit payloads ride base64 so
// any codepage bytes survive JSON intact; the client decodes into a
// Uint8Array for xterm.
func ptyEvent(typ string, data []byte, code int) []byte {
	ev := map[string]any{"type": typ}
	if data != nil {
		ev["d"] = base64.StdEncoding.EncodeToString(data)
	}
	if typ == "exit" {
		ev["code"] = code
	}
	b, _ := json.Marshal(ev)
	return b
}

// clientFrame is one accepted client→server message. Input rides a plain
// `text` string (JSON transport is already UTF-8); output frames use base64
// instead because ConPTY bytes may be in a non-UTF-8 codepage.
type clientFrame struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	D    string `json:"d,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// handlePtyWS upgrades to WebSocket and bridges the connection to the
// workspace's persistent shell. Registered ONLY when Config.PTYEnabled and
// an API key are both set; auth happens in the router wrapper (Bearer header
// or ?token=, the browser WebSocket cannot set custom headers).
func (s *Server) handlePtyWS(w http.ResponseWriter, r *http.Request) {
	up := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		// Auth is the gate (API key required for the route to exist); ConPTY
		// clients are same-origin pages, but skins/proxies may strip Origin.
		CheckOrigin: func(*http.Request) bool { return true },
	}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInvalidFramePayloadData, err.Error()), time.Now().Add(time.Second))
		return
	}
	key := r.URL.Query().Get("workspace")
	if key == "" {
		key = "(default)"
	}

	sess, err := s.ptyReg.getOrCreate(key, root)
	if err != nil {
		log.Printf("[pty] session %q spawn failed: %v", key, err)
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInvalidFramePayloadData, err.Error()), time.Now().Add(time.Second))
		return
	}
	s.auditExec(execRequest{Argv: []string{"pty:" + key}, Workspace: key}, 0, "pty session attached")

	// Subscribe: buffered channel per connection; a dedicated goroutine is
	// the single writer (gorilla/websocket forbids concurrent writes).
	ch := make(chan []byte, 256)
	sess.mu.Lock()
	if sess.exited {
		sess.mu.Unlock()
		_ = conn.WriteMessage(websocket.TextMessage, ptyEvent("exit", nil, sess.exitCode))
		return
	}
	sess.subs[ch] = struct{}{}
	sess.mu.Unlock()
	defer func() {
		sess.mu.Lock()
		delete(sess.subs, ch)
		sess.mu.Unlock()
	}()

	// Replay history before live data so a reconnect lands mid-stream.
	if tr := sess.replay(); len(tr) > 0 {
		if err := conn.WriteMessage(websocket.TextMessage, ptyEvent("replay", tr, 0)); err != nil {
			return
		}
	}

	writeErr := make(chan error, 1)
	go func() {
		for frame := range ch {
			if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				writeErr <- err
				return
			}
		}
		writeErr <- nil
	}()

	conn.SetReadLimit(64 << 10)
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var f clientFrame
		if err := json.Unmarshal(raw, &f); err != nil {
			continue
		}
		switch f.Type {
		case "input":
			if f.Text == "" {
				continue
			}
			if werr := sess.write([]byte(f.Text)); werr != nil {
				// pty died mid-write: surface and stop reading.
				_ = conn.WriteMessage(websocket.TextMessage, ptyEvent("exit", nil, 1))
				goto done
			}
		case "resize":
			_ = sess.resize(f.Cols, f.Rows)
		case "kill":
			s.ptyReg.kill(key)
			goto done
		}
	}
done:
	sess.mu.Lock()
	delete(sess.subs, ch)
	sess.mu.Unlock()
}

// ShutdownTerminals kills every live PTY session (server shutdown path).
func (s *Server) ShutdownTerminals() {
	if s.ptyReg == nil {
		return
	}
	s.ptyReg.mu.Lock()
	keys := make([]string, 0, len(s.ptyReg.sessions))
	for k := range s.ptyReg.sessions {
		keys = append(keys, k)
	}
	s.ptyReg.mu.Unlock()
	for _, k := range keys {
		s.ptyReg.kill(k)
	}
}
