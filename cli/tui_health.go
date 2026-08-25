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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	healthDefaultInterval = 60 * time.Second
	healthMinInterval     = 10 * time.Second
	healthMaxInterval     = 600 * time.Second
	healthDialTimeout     = 2 * time.Second
	healthHTTPTimeout     = 2 * time.Second
	// healthCoolDown silences repeated failure hints per provider (5 min).
	healthCoolDown = 5 * time.Minute
)

// providerHealth holds one provider's health-check result (RF-10).
type providerHealth struct {
	key       string
	endpoint  string
	ok        bool
	latency   time.Duration
	lastCheck time.Time
	err       string
	lastWarn  time.Time
}

// healthChecker is the periodic API health-check state (RF-10).
type healthChecker struct {
	active   bool
	interval time.Duration
	entries  map[string]*providerHealth
	checking bool
}

// healthTickMsg drives the periodic health check.
type healthTickMsg struct{}

// newHealthChecker initializes the checker from config (best-effort).
func newHealthChecker(cfg CLIConfig) healthChecker {
	interval := time.Duration(cfg.TUI.HealthCheckInterval) * time.Second
	if interval <= 0 {
		interval = healthDefaultInterval
	}
	if interval < healthMinInterval {
		interval = healthMinInterval
	}
	if interval > healthMaxInterval {
		interval = healthMaxInterval
	}
	active := cfg.Features.HealthCheck && cfg.TUI.HealthProbeMode != "off"
	return healthChecker{
		active:   active,
		interval: interval,
		entries:  map[string]*providerHealth{},
	}
}

// setInterval clamps and stores a new check interval (seconds).
func (h *healthChecker) setInterval(sec int) time.Duration {
	if sec < int(healthMinInterval.Seconds()) {
		sec = int(healthMinInterval.Seconds())
	}
	if sec > int(healthMaxInterval.Seconds()) {
		sec = int(healthMaxInterval.Seconds())
	}
	h.interval = time.Duration(sec) * time.Second
	return h.interval
}

// tickCmd returns a tea.Cmd that fires healthTickMsg after the interval.
func (m *chatTUI) healthTickCmd() tea.Cmd {
	if !m.health.active {
		return nil
	}
	return tea.Tick(m.health.interval, func(_ time.Time) tea.Msg { return healthTickMsg{} })
}

// routesForHealth lists the provider routes to probe: the active route plus
// every configured providers.list entry (deduplicated by key).
func (m *chatTUI) routesForHealth() []ModelRoute {
	seen := map[string]bool{}
	var routes []ModelRoute
	if m.route.Provider != "" && m.route.Endpoint != "" {
		routes = append(routes, m.route)
		seen[m.route.Provider] = true
	}
	for key, p := range m.cfg.Providers.List {
		if seen[key] || p.Endpoint == "" {
			continue
		}
		seen[key] = true
		routes = append(routes, ModelRoute{Provider: key, Model: p.Model, Endpoint: p.Endpoint, APIKey: p.APIKey})
	}
	return routes
}

// isLocalHost reports whether a host is a local/LAN address.
func isLocalHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	if strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "192.168.") {
		return true
	}
	if strings.HasPrefix(host, "172.") {
		// 172.16.0.0 – 172.31.255.255
		parts := strings.Split(host, ".")
		if len(parts) == 4 {
			if v, err := strconv.Atoi(parts[1]); err == nil && v >= 16 && v <= 31 {
				return true
			}
		}
	}
	return false
}

// endpointHostPort extracts host:port from an endpoint URL.
func endpointHostPort(endpoint string) (host string, port string, ok bool) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", false
	}
	host = u.Hostname()
	if host == "" {
		return "", "", false
	}
	port = u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	return host, port, true
}

// probeOne probes a single route and returns the result.
func (m *chatTUI) probeOne(route ModelRoute) *providerHealth {
	res := &providerHealth{
		key:       route.Provider,
		endpoint:  route.Endpoint,
		lastCheck: time.Now(),
	}
	host, port, ok := endpointHostPort(route.Endpoint)
	if !ok {
		res.err = "invalid endpoint"
		return res
	}
	probeMode := m.cfg.TUI.HealthProbeMode
	if probeMode == "" {
		probeMode = "tcp"
	}

	if isLocalHost(host) && probeMode == "http" {
		// HTTP probe: GET {endpoint}/models, expect 2xx.
		probeURL := strings.TrimRight(route.Endpoint, "/") + "/models"
		start := time.Now()
		status, err := httpGetStatus(probeURL, route.APIKey, healthHTTPTimeout)
		res.latency = time.Since(start)
		if err != nil {
			res.err = err.Error()
			return res
		}
		if status < 200 || status >= 300 {
			res.err = fmt.Sprintf("HTTP %d", status)
			return res
		}
		res.ok = true
		return res
	}

	// TCP dial probe (local default) / remote construct-only probe.
	if isLocalHost(host) {
		start := time.Now()
		addr := net.JoinHostPort(host, port)
		conn, err := net.DialTimeout("tcp", addr, healthDialTimeout)
		res.latency = time.Since(start)
		if err != nil {
			res.err = err.Error()
			return res
		}
		conn.Close()
		res.ok = true
		return res
	}

	// Remote API: only construct the requester (no real request) to avoid
	// consuming quota. Mirrors compressModelAdapter.Available() semantics.
	start := time.Now()
	if _, err := buildModelRequester(route.routeConfig()); err != nil {
		res.err = "constructor: " + err.Error()
		return res
	}
	res.latency = time.Since(start)
	res.ok = true
	return res
}

// httpGetStatus performs a GET and returns the status code.
func httpGetStatus(probeURL, apiKey string, timeout time.Duration) (int, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, probeURL, nil)
	if err != nil {
		return 0, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// checkAll probes all configured routes (guarded against re-entry) and emits
// a one-time failure hint per provider with a cooldown.
func (m *chatTUI) checkAll() {
	if m.health.checking {
		return
	}
	m.health.checking = true
	defer func() { m.health.checking = false }()
	routes := m.routesForHealth()
	for _, r := range routes {
		res := m.probeOne(r)
		prev := m.health.entries[r.Provider]
		m.health.entries[r.Provider] = res
		if res.ok {
			continue
		}
		// One-time warn with cooldown (per provider).
		if prev == nil || time.Since(prev.lastWarn) > healthCoolDown {
			res.lastWarn = time.Now()
			m.commitLine("")
			m.commitLine(warnText(fmt.Sprintf("  ○ %s 不可达: %s", res.key, res.err)))
		}
	}
	m.transcriptDirty = true
}

// renderHealth returns the status-bar health segment (RF-10).
func (m *chatTUI) renderHealth() string {
	if !m.cfg.Features.HealthCheck || !m.health.active {
		return ""
	}
	e, ok := m.health.entries[m.route.Provider]
	if !ok {
		return dim("● probing")
	}
	if e.ok {
		return successText(fmt.Sprintf("● %s %dms", e.key, e.latency.Milliseconds()))
	}
	return errorText("○ " + e.key + " offline")
}

// tuiHandleHealth handles /health (RF-10):
//
//	/health            → probe all + report
//	/health <key>      → probe one provider
//	/health interval N → adjust interval (10–600s)
func tuiHandleHealth(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	fields := strings.Fields(args)
	m.commitLine("")

	if len(fields) >= 2 && fields[0] == "interval" {
		sec, err := strconv.Atoi(fields[1])
		if err != nil {
			m.commitLine(errorText("  ✗ interval must be seconds: " + fields[1]))
			return nil, false
		}
		iv := m.health.setInterval(sec)
		m.commitLine(successText(fmt.Sprintf("  ✓ 健康检查周期设为 %.0fs", iv.Seconds())))
		return m.healthTickCmd(), false
	}

	if len(fields) == 1 {
		key := fields[0]
		for _, r := range m.routesForHealth() {
			if r.Provider == key {
				res := m.probeOne(r)
				m.health.entries[key] = res
				m.reportHealth(res)
				return nil, false
			}
		}
		m.commitLine(errorText("  ✗ 未知 provider: " + key))
		return nil, false
	}

	// No args → probe all + report.
	m.checkAll()
	m.commitLine(accent("API health:"))
	if len(m.health.entries) == 0 {
		m.commitLine(dim("  No providers to probe."))
	}
	for key, e := range m.health.entries {
		m.reportHealth(e)
		_ = key
	}
	return m.healthTickCmd(), false
}

// reportHealth prints one provider health row.
func (m *chatTUI) reportHealth(e *providerHealth) {
	if e.ok {
		m.commitLine(dim("  ● ") + footerInfo(e.key) + dim(fmt.Sprintf(" (%dms)", e.latency.Milliseconds())))
	} else {
		m.commitLine(dim("  ○ ") + errorText(e.key) + dim(" offline · "+e.err))
	}
}
