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

// Package main is the entry point for the InferGlow CLI agent.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/inferglow/cli"
)

func main() {
	// Subcommand dispatch: "inferglow team ..." / "inferglow memory ..."
	// If the first positional arg is a known subcommand, dispatch and return.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			if err := cli.RunInitWizard(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "team":
			cfg, _, err := cli.LoadOrDefaultConfig("")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not load config: %v (using defaults)\n", err)
			}
			cli.ApplyEnvOverrides(&cfg)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := runTeam(ctx, cfg, os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "memory":
			cfg, _, err := cli.LoadOrDefaultConfig("")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not load config: %v (using defaults)\n", err)
			}
			cli.ApplyEnvOverrides(&cfg)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := runMemory(ctx, cfg, os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	workspace := flag.String("workspace", ".", "Working directory for the agent")
	modelName := flag.String("model", "", "Model name to use")
	configPath := flag.String("config", "", "Path to config file")
	resumeID := flag.String("resume", "", "Resume a previous session by ID")
	unsafeMode := flag.Bool("unsafe", false, "Allow bash execution without confirmation")
	flag.Bool("tui", true, "Enable full-screen TUI mode (default, ignored if --cli is set)")
	cliMode := flag.Bool("cli", false, "Use single-output REPL mode instead of TUI")
	oneshotPrompt := flag.String("z", "", "One-shot mode: send prompt, print final response to stdout, exit")
	oneshotLong := flag.String("oneshot", "", "Same as -z (one-shot mode)")
	flag.Parse()

	cfg, loadedPath, err := cli.LoadOrDefaultConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config: %v (using defaults)\n", err)
	}
	_ = loadedPath // available for future /config reload

	// Ensure all required data directories exist.
	if err := cli.EnsureDataDirs(cfg.DataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create data dirs: %v\n", err)
	}

	// Apply environment variable overrides.
	cli.ApplyEnvOverrides(&cfg)

	// Override config with flags.
	if *workspace != "." {
		cfg.WorkspaceDir = *workspace
	}
	if *modelName != "" {
		cfg.LLM.Model = *modelName
	}
	cfg.UnsafeMode = *unsafeMode

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Mode dispatch — priority: oneshot > tui > cli(REPL).
	//
	// OneShot (-z / --oneshot): single prompt → stdout → exit.
	// TUI (default): full-screen Bubble Tea alt-screen.
	// REPL (--cli): line-based interactive loop.
	prompt := *oneshotPrompt
	if prompt == "" {
		prompt = *oneshotLong
	}

	if prompt != "" {
		if err := cli.RunOneShot(ctx, cfg, prompt); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if !*cliMode {
		if err := cli.RunTUI(ctx, cfg, *resumeID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := cli.RunREPL(ctx, cfg, *resumeID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
