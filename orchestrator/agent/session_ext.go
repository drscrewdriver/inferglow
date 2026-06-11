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

package agent

import (
	"github.com/inferglow/orchestrator/agent/internal/extension"
)

// This file re-exports SessionExtension (now living in the internal/extension
// subpackage) so the public agent API remains stable. The type alias makes
// agent.SessionExtension and extension.SessionExtension the identical type,
// so existing call sites (including Agent, Engine, and the agent constructors)
// compile unchanged.

// SessionExtension wraps a SessionBackend (either *session.Session or
// *session.ThreeZoneSession) and provides a simplified interface for the
// orchestrator to manage conversation history.
type SessionExtension = extension.SessionExtension

// NewSessionExtension creates a SessionExtension wrapping the given backend.
var NewSessionExtension = extension.NewSessionExtension
