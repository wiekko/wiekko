// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	_ "embed"
	"io"
)

//go:embed agent_guide.md
var agentGuide string

func agentGuideCommand() *command {
	return &command{
		name:    "agent-guide",
		summary: "print a usage guide for AI agents (paste it into AGENTS.md or CLAUDE.md)",
		run: func(_ context.Context, a *app, args []string) error {
			if err := noArgs("agent-guide", args); err != nil {
				return err
			}
			_, err := io.WriteString(a.io.Stdout, agentGuide)
			return err
		},
	}
}
