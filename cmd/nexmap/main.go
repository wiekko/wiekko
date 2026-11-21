// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Command nexmap is the Nexmap command line client: repository and CI
// integration, and an entry point for AI agents. It talks to a Nexmap server
// (nexmapd) through the REST API only.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"nexmap.net/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], processIO())
	stop()
	os.Exit(code)
}

func processIO() cli.IO {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nexmap: %v\n", err)
		os.Exit(cli.ExitUsage)
	}
	stdinFD := int(os.Stdin.Fd())
	return cli.IO{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Getenv:      os.Getenv,
		Cwd:         cwd,
		StdinIsTTY:  term.IsTerminal(stdinFD),
		StderrIsTTY: term.IsTerminal(int(os.Stderr.Fd())),
		Executable:  os.Executable,
		ReadSecret: func() (string, error) {
			b, err := term.ReadPassword(stdinFD)
			return string(b), err
		},
	}
}
