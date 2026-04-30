// Command sandbox is a minimal CLI demonstrating the sandbox framework.
//
// It performs 6 steps:
//  1. Detects the current OS.
//  2. Lists providers supported on this OS (per the default matrix).
//  3. Creates a Manager and registers the 4 builtin providers.
//  4. Calls InspectAvailability on each registered provider.
//  5. Calls SelectSandbox(ModeAuto) and prints the chosen provider.
//  6. Creates a TrustedLocal handle and runs `echo hello` in it.
//
// Exit code is 0 on success, 1 on any error.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/inferglow/sandbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	// Step 1: Detect OS
	osName := sandbox.DetectOS()
	fmt.Println("Detected OS:", osName)

	// Step 2: List supported providers on this OS
	providers := sandbox.AvailableProvidersOnOS(osName)
	fmt.Printf("Providers supported on %s:\n", osName)
	for _, p := range providers {
		fmt.Println("  -", p)
	}

	// Step 3: Create Manager using ProviderBuilder (auto-register all providers)
	mgr, err := sandbox.NewProviderBuilder().Build()
	if err != nil {
		return fmt.Errorf("build providers: %w", err)
	}
	fmt.Println("\nRegistered providers:")
	for _, name := range mgr.List() {
		fmt.Println("  -", name)
	}

	// Step 4: Inspect availability of each
	fmt.Println("\nAvailability inspection:")
	for _, name := range mgr.List() {
		p, err := mgr.Get(name)
		if err != nil {
			return fmt.Errorf("get %s: %w", name, err)
		}
		avail, err := p.InspectAvailability()
		if err != nil {
			fmt.Printf("  %s: ERROR %v\n", name, err)
			continue
		}
		if avail.Available {
			binary := avail.BinaryPath
			if binary == "" {
				binary = "(no binary)"
			}
			fmt.Printf("  %s: available (platform=%s, binary=%s)\n", name, avail.Platform, binary)
		} else {
			msg := avail.ErrorMessage
			if msg == "" {
				msg = "(no reason)"
			}
			fmt.Printf("  %s: unavailable (%s)\n", name, msg)
		}
	}

	// Step 5: SelectSandbox(ModeAuto)
	selected, err := mgr.SelectSandbox(sandbox.ModeAuto)
	if err != nil {
		return fmt.Errorf("select sandbox (auto): %w", err)
	}
	fmt.Printf("\nSelectSandbox(ModeAuto) picked: %s\n", selected.Name())

	// Step 6: Create TrustedLocal handle and execute echo hello
	policy := sandbox.DefaultPolicy()
	h, err := mgr.CreateHandle(sandbox.ModeTrustedLocal, nil, &policy)
	if err != nil {
		return fmt.Errorf("create handle: %w", err)
	}
	if err := h.Start(context.Background()); err != nil {
		return fmt.Errorf("start handle: %w", err)
	}
	defer func() { _ = h.Stop(context.Background()) }()

	result, err := h.Execute(context.Background(), &sandbox.Command{Argv: []string{"echo", "hello"}})
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	fmt.Printf("\nExecute(echo hello):\n")
	fmt.Printf("  ExitCode: %d\n", result.ExitCode)
	fmt.Printf("  Stdout:    %q\n", result.Stdout)
	fmt.Printf("  Stderr:    %q\n", result.Stderr)
	fmt.Printf("  Duration:  %v\n", result.Duration)

	return nil
}
