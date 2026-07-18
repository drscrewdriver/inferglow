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

//go:build desktop

// Package desktop provides the Wails-based desktop shell for InferGlow (OT-10).
// Build with: wails build
// This file is excluded from normal builds via the "desktop" build tag.
package desktop

// DesktopBridge defines the Go↔JS binding interface exposed to the WebView.
// Wails automatically generates JS bindings for exported methods.
type DesktopBridge struct {
	serverAddr string
	sessionID  string
}

// NewDesktopBridge creates the desktop bridge.
func NewDesktopBridge() *DesktopBridge {
	return &DesktopBridge{}
}

// StartSession initializes a new agent session and returns the session ID.
func (d *DesktopBridge) StartSession(workspace string) (string, error) {
	// TODO: wire to orchestrator/session management
	d.sessionID = "desktop-" + workspace
	return d.sessionID, nil
}

// SendChat sends a message to the agent and returns the response.
func (d *DesktopBridge) SendChat(message string) (string, error) {
	// TODO: wire to agent.Run
	return "Desktop chat not yet wired: " + message, nil
}

// GetStatus returns the current agent/server status.
func (d *DesktopBridge) GetStatus() map[string]string {
	return map[string]string{
		"session": d.sessionID,
		"server":  d.serverAddr,
		"status":  "idle",
	}
}

// GetDashboardURL returns the URL for the embedded observability dashboard.
func (d *DesktopBridge) GetDashboardURL() string {
	if d.serverAddr == "" {
		return ""
	}
	return "http://" + d.serverAddr + "/dashboard"
}
