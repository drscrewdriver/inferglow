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

// Command inferglow-server starts the InferGlow REST API server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/inferglow/messagebus"
	"github.com/inferglow/server"
)

func main() {
	var (
		addr    = flag.String("addr", ":8080", "Listen address")
		apiKey  = flag.String("api-key", "", "API key for Bearer auth (empty = disabled)")
		cors    = flag.String("cors", "", "Comma-separated CORS origins (empty = disabled)")
		timeout = flag.Duration("timeout", 30*time.Second, "Request read timeout")
	)
	flag.Parse()

	cfg := server.DefaultConfig()
	cfg.Addr = *addr
	cfg.APIKey = *apiKey
	cfg.ReadTimeout = *timeout

	if *cors != "" {
		cfg.CORSOrigins = splitComma(*cors)
	}

	srv := server.NewServer(cfg, nil) // TODO: wire real AgentStore

	// C-3: demo wiring of the in-memory message bus (out-of-the-box).
	srv.SetMessageBus(messagebus.NewInMemoryMessageBus())

	// C-4~C-7: default in-memory wiring for the management backend, zero-config.
	srv.SetSessionStore(server.NewSessionStore())
	srv.SetScheduleStore(server.NewScheduleStore())
	srv.SetCredentialStore(server.NewCredentialStore())
	srv.SetWorkspaceProvider(server.NewWorkspaceProvider())
	srv.SetMessageStore(server.NewMessageStore())

	// C-10: Skill Hub store (backed by action.ActionRegistry). Skills are
	// installed by Go-side registration via SkillStore.Install; see the
	// server tests for an example.
	srv.SetSkillStore(server.NewSkillStore())

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("InferGlow server starting on %s", cfg.Addr)
		if err := srv.Start(); err != nil {
			log.Printf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
		os.Exit(1)
	}

	fmt.Println("Server stopped gracefully")
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range split(s, ',') {
		if part = trim(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func split(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
