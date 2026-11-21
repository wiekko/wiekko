// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"flag"

	"wiekko.net/internal/cli/mcp"
)

func mcpCommand() *command {
	var readOnly bool
	return &command{
		name:    "mcp",
		summary: "run a Model Context Protocol server on stdin/stdout for AI agents",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&readOnly, "read-only", false, "expose only tools that do not change data (recommended)")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("mcp", args); err != nil {
				return err
			}
			// Configuration problems (no server, product or token) are not
			// fatal here: they are reported on each tool call, where the
			// agent sees them, instead of killing the server before the
			// client can show anything.
			return mcp.Serve(ctx, mcp.Options{NewSession: a.session, ReadOnly: readOnly}, a.io.Stdin, a.io.Stdout)
		},
	}
}
