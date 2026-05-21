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

package actions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/inferglow/action"
)

// URLFetchActionID is the registered Action name for the URL fetcher.
const URLFetchActionID = "url_fetch"

// DefaultURLFetchLimit is the default maximum response size (1 MiB).
const DefaultURLFetchLimit = 1 << 20

// DefaultURLFetchTimeout is the default HTTP client timeout.
const DefaultURLFetchTimeout = 30 * time.Second

// URLFetchConfig configures a URLFetch Action instance.
type URLFetchConfig struct {
	// MaxBytes caps the response body size. Zero means DefaultURLFetchLimit.
	MaxBytes int64
	// Timeout bounds the HTTP request. Zero means DefaultURLFetchTimeout.
	Timeout time.Duration
	// AllowedSchemes restricts which URL schemes are permitted. An empty
	// slice defaults to {"http", "https"}.
	AllowedSchemes []string
}

// urlFetchExecutor is the ActionExecutor for URL fetching.
type urlFetchExecutor struct {
	cfg    URLFetchConfig
	client *http.Client
}

// URLFetchResult is the structured payload returned by the url_fetch
// Action.
type URLFetchResult struct {
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
	BytesRead   int64  `json:"bytes_read"`
}

// URLFetchSpec is the ActionSpec for url_fetch: read-only network I/O,
// no approval, no sandbox.
var URLFetchSpec = &action.ActionSpec{
	ActionID:         URLFetchActionID,
	Name:             "URLFetch",
	Description:      "Fetch the body of an HTTP(S) URL with a size cap to prevent DoS.",
	SideEffectLevel:  action.SideEffectRead,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       false,
	ExposeToModel:    true,
	Tags:             []string{"web", "http", "builtin"},
	Kwargs: map[string]any{
		"url":       map[string]any{"type": "string", "required": true},
		"max_bytes": map[string]any{"type": "integer", "required": false},
	},
	Returns: map[string]any{"type": "object"},
	DefaultPolicy: &action.ActionPolicy{
		Timeout:        DefaultURLFetchTimeout,
		MaxOutputBytes: DefaultURLFetchLimit,
		NetworkAccess:  "enabled",
	},
}

// NewURLFetchAction builds an Action that fetches URL contents with the
// given configuration. The returned Action is safe for concurrent use.
func NewURLFetchAction(cfg URLFetchConfig) *action.Action {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = DefaultURLFetchLimit
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultURLFetchTimeout
	}
	if len(cfg.AllowedSchemes) == 0 {
		cfg.AllowedSchemes = []string{"http", "https"}
	}
	return &action.Action{
		Name:        URLFetchActionID,
		Description: "Fetch the contents of a URL.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":       map[string]any{"type": "string"},
				"max_bytes": map[string]any{"type": "integer"},
			},
			"required": []string{"url"},
		},
		Executor: &urlFetchExecutor{
			cfg:    cfg,
			client: &http.Client{Timeout: cfg.Timeout},
		},
		Tags: []string{"web", "http", "builtin"},
	}
}

// Execute fetches the URL and returns a URLFetchResult.
func (e *urlFetchExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	rawURL, _ := input["url"].(string)
	if rawURL == "" {
		return &action.ActionResult{
			OK: false, Status: "error", Error: "url_fetch: url is required",
		}, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("url_fetch: invalid url: %s", err),
		}, nil
	}
	if !schemeAllowed(parsed.Scheme, e.cfg.AllowedSchemes) {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("url_fetch: scheme %q not allowed", parsed.Scheme),
		}, nil
	}

	maxBytes := e.cfg.MaxBytes
	if mb, ok := input["max_bytes"]; ok {
		switch v := mb.(type) {
		case float64:
			if v > 0 && v < float64(maxBytes) {
				maxBytes = int64(v)
			}
		case int:
			if v > 0 && int64(v) < maxBytes {
				maxBytes = int64(v)
			}
		case int64:
			if v > 0 && v < maxBytes {
				maxBytes = v
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("url_fetch: build request: %s", err),
		}, nil
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("url_fetch: %s", err),
		}, nil
	}
	defer resp.Body.Close()

	// LimitedReader enforces the size cap regardless of Content-Length.
	limited := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("url_fetch: read body: %s", err),
		}, nil
	}
	if int64(len(body)) > maxBytes {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("url_fetch: response exceeds max_bytes=%d", maxBytes),
		}, nil
	}

	return &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: URLFetchResult{
			URL:         rawURL,
			StatusCode:  resp.StatusCode,
			ContentType: resp.Header.Get("Content-Type"),
			Content:     string(body),
			BytesRead:   int64(len(body)),
		},
	}, nil
}

// schemeAllowed reports whether scheme is in allowed.
func schemeAllowed(scheme string, allowed []string) bool {
	s := strings.ToLower(scheme)
	for _, a := range allowed {
		if strings.ToLower(a) == s {
			return true
		}
	}
	return false
}
