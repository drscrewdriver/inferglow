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

package server

import (
	"net/http"
	"strings"
)

// identity derives the caller's resource-access principal from the request's
// Authorization header. When auth is disabled or the header is absent, it
// falls back to the empty string, which the ResourceAccessPolicy interprets as
// the anonymous principal. This keeps the server request-identity model open
// (spec C-2) without rewriting the auth middleware.
func (s *Server) identity(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	// Accept both "Bearer <token>" and bare-token forms. In the absence of a
	// tenant registry that maps tokens to IDs we treat the token itself as the
	// principal, mirroring the single-global-APIKey deployment default.
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return strings.TrimSpace(h)
}

// canAccess is the single authorization entry point used by the C-6 and
// resource-scoped handlers. It activates the injectable resourcePolicy that
// was previously unused (server.go SetResourceAccessPolicy): nil falls back to
// DefaultResourceAccessPolicy (allow-all), so behaviour is unchanged until a
// concrete policy is wired in.
func (s *Server) canAccess(r *http.Request, resourceType, resourceID, action string) bool {
	p := ResourceAccessPolicy(s.resourcePolicy)
	if p == nil {
		p = DefaultResourceAccessPolicy{}
	}
	return p.CanAccess(r.Context(), s.identity(r), resourceType, resourceID, action)
}
