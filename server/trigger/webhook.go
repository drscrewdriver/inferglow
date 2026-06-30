// Copyright 2026 InferGlow Authors

package trigger

import (
	"context"
	"encoding/json"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// WebhookTrigger creates a run when an HTTP endpoint is called.
type WebhookTrigger struct {
	cfg     Config
	starter RunStarter
	enabled bool
	mu      sync.Mutex
}

// NewWebhookTrigger creates a webhook trigger from config.
func NewWebhookTrigger(cfg Config, starter RunStarter) (*WebhookTrigger, error) {
	if cfg.Flow == "" {
		return nil, fmt.Errorf("webhook trigger requires a flow name")
	}
	return &WebhookTrigger{
		cfg:     cfg,
		starter: starter,
		enabled: cfg.Enabled,
	}, nil
}

func (w *WebhookTrigger) ID() string       { return w.cfg.ID }
func (w *WebhookTrigger) Type() string     { return "webhook" }
func (w *WebhookTrigger) FlowName() string { return w.cfg.Flow }
func (w *WebhookTrigger) Enabled() bool    { return w.enabled }

func (w *WebhookTrigger) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.enabled = true
	return nil
}

func (w *WebhookTrigger) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.enabled = false
	return nil
}

// ServeHTTP handles the webhook HTTP request.
// This is called by the server's HTTP handler when the webhook route is hit.
func (w *WebhookTrigger) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if !w.enabled {
		http.Error(rw, "trigger disabled", http.StatusServiceUnavailable)
		return
	}

	// Read body ONCE to avoid double-read bug with HMAC verification.
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Verify HMAC signature if secret is configured.
	if w.cfg.Webhook != nil && w.cfg.Webhook.Secret != "" {
		if !w.verifySignature(r.Header, body) {
			http.Error(rw, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse request body as inputs.
	inputs := make(map[string]any)
	if len(body) > 0 {
		if err := json.Unmarshal(body, &inputs); err != nil {
			http.Error(rw, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Merge with default inputs.
	if w.cfg.Defaults != nil {
		for k, v := range w.cfg.Defaults {
			if _, exists := inputs[k]; !exists {
				inputs[k] = v
			}
		}
	}
	if w.cfg.Webhook != nil && w.cfg.Webhook.Inputs != nil {
		for k, v := range w.cfg.Webhook.Inputs {
			if _, exists := inputs[k]; !exists {
				inputs[k] = v
			}
		}
	}

	// Add webhook metadata.
	inputs["_trigger"] = map[string]any{
		"type":       "webhook",
		"trigger_id": w.cfg.ID,
		"source_ip":  r.RemoteAddr,
	}

	// Create and start the run.
	handle, err := w.starter.Start(w.cfg.Flow, inputs, "trigger:"+w.cfg.ID)
	if err != nil {
		http.Error(rw, "start run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusAccepted)
	json.NewEncoder(rw).Encode(map[string]any{
		"run_id": handle.GetID(),
		"status": handle.GetStatus(),
	})
}

// verifySignature checks the X-Signature-256 header against HMAC-SHA256.
// body is the raw request body (read once by the caller).
func (w *WebhookTrigger) verifySignature(h http.Header, body []byte) bool {
	sig := h.Get("X-Signature-256")
	if sig == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(w.cfg.Webhook.Secret))
	mac.Write(body)
	expected := fmt.Sprintf("sha256=%x", mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(sig))
}
