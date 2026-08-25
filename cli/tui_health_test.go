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

package cli

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsLocalHost(t *testing.T) {
	local := []string{"localhost", "127.0.0.1", "::1", "127.5.5.5", "10.1.2.3", "192.168.1.1", "172.16.0.1", "172.31.255.255"}
	for _, h := range local {
		if !isLocalHost(h) {
			t.Errorf("isLocalHost(%s) = false, want true", h)
		}
	}
	remote := []string{"api.openai.com", "172.32.0.1", "8.8.8.8", "example.org"}
	for _, h := range remote {
		if isLocalHost(h) {
			t.Errorf("isLocalHost(%s) = true, want false", h)
		}
	}
}

func TestEndpointHostPort(t *testing.T) {
	host, port, ok := endpointHostPort("http://localhost:11434")
	if !ok || host != "localhost" || port != "11434" {
		t.Fatalf("got %s:%s ok=%v", host, port, ok)
	}
	// Default port by scheme.
	host, port, ok = endpointHostPort("https://api.deepseek.com/v1")
	if !ok || host != "api.deepseek.com" || port != "443" {
		t.Fatalf("got %s:%s ok=%v", host, port, ok)
	}
	// Invalid URL.
	if _, _, ok := endpointHostPort("://bad"); ok {
		t.Fatal("invalid endpoint should fail")
	}
}

func TestHealthCheckerIntervalClamp(t *testing.T) {
	h := newHealthChecker(DefaultCLIConfig())
	if h.interval != healthDefaultInterval {
		t.Fatalf("default interval = %v, want %v", h.interval, healthDefaultInterval)
	}
	iv := h.setInterval(3) // below min → clamped to 10s
	if iv != healthMinInterval {
		t.Fatalf("clamped interval = %v, want %v", iv, healthMinInterval)
	}
	iv = h.setInterval(9999) // above max → clamped to 600s
	if iv != healthMaxInterval {
		t.Fatalf("clamped interval = %v, want %v", iv, healthMaxInterval)
	}
	iv = h.setInterval(30)
	if iv != 30*time.Second {
		t.Fatalf("interval = %v, want 30s", iv)
	}
}

func TestHealthCheckerConfigOff(t *testing.T) {
	cfg := DefaultCLIConfig()
	cfg.TUI.HealthProbeMode = "off"
	h := newHealthChecker(cfg)
	if h.active {
		t.Fatal("health_probe_mode=off should deactivate the checker")
	}
	cfg2 := DefaultCLIConfig()
	cfg2.Features.HealthCheck = false
	if newHealthChecker(cfg2).active {
		t.Fatal("health_check=false should deactivate the checker")
	}
}

func TestProbeOneLocalTCP(t *testing.T) {
	// Start a real local TCP listener to probe.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	port := addr[strings.LastIndex(addr, ":")+1:]

	m := &chatTUI{cfg: DefaultCLIConfig()}
	res := m.probeOne(ModelRoute{Provider: "local", Endpoint: "http://127.0.0.1:" + port})
	if !res.ok {
		t.Fatalf("local probe should succeed: %+v", res)
	}
	if res.latency < 0 {
		t.Fatal("latency should never be negative")
	}
}

func TestProbeOneLocalTCPDown(t *testing.T) {
	// Find a free port, close it, then probe → connection refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	port := addr[strings.LastIndex(addr, ":")+1:]
	ln.Close()

	m := &chatTUI{cfg: DefaultCLIConfig()}
	res := m.probeOne(ModelRoute{Provider: "local", Endpoint: "http://127.0.0.1:" + port})
	if res.ok {
		t.Fatal("probe of a closed port should fail")
	}
	if res.err == "" {
		t.Fatal("failure should carry an error message")
	}
}

func TestProbeOneInvalidEndpoint(t *testing.T) {
	m := &chatTUI{cfg: DefaultCLIConfig()}
	res := m.probeOne(ModelRoute{Provider: "x", Endpoint: "://bad"})
	if res.ok {
		t.Fatal("invalid endpoint should fail")
	}
}

func TestProbeOneRemoteConstructOnly(t *testing.T) {
	m := &chatTUI{cfg: DefaultCLIConfig()}
	res := m.probeOne(ModelRoute{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		Endpoint: "https://api.deepseek.com/v1",
		APIKey:   "sk-test",
	})
	if !res.ok {
		t.Fatalf("remote construct-only probe should succeed: %+v", res)
	}
	// Remote with no endpoint → constructor failure.
	res2 := m.probeOne(ModelRoute{Provider: "nope", Model: "m", Endpoint: "https://x.invalid/v1"})
	if res2.ok {
		t.Fatal("unconstructable remote route should fail")
	}
}

func TestCheckAllGuard(t *testing.T) {
	m := &chatTUI{cfg: DefaultCLIConfig()}
	m.health.checking = true
	// checkAll must not run while checking (no panic, no entries).
	m.checkAll()
	if len(m.health.entries) != 0 {
		t.Fatal("checkAll should no-op while checking")
	}
}

func TestHealthEntryRender(t *testing.T) {
	m := &chatTUI{cfg: DefaultCLIConfig(), health: newHealthChecker(DefaultCLIConfig())}
	m.route = ModelRoute{Provider: "deepseek", Model: "deepseek-chat"}
	m.health.entries = map[string]*providerHealth{
		"deepseek": {key: "deepseek", ok: true, latency: 42 * time.Millisecond},
	}
	out := m.renderHealth()
	if !strings.Contains(out, "●") || !strings.Contains(out, "42ms") {
		t.Fatalf("online render = %q", out)
	}
	m.health.entries["deepseek"] = &providerHealth{key: "deepseek", ok: false, err: "timeout"}
	out2 := m.renderHealth()
	if !strings.Contains(out2, "○") || !strings.Contains(out2, "offline") {
		t.Fatalf("offline render = %q", out2)
	}
}
