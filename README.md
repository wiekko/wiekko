# nexmap

Nexmap Product Management Solution

## Binaries

| Binary    | Source          | Purpose                                                      |
|-----------|-----------------|--------------------------------------------------------------|
| `nexmapd` | `cmd/nexmapd`   | The server: web UI and REST API (`just build`, `just dev`)   |
| `nexmap`  | `cmd/nexmap`    | The command line client, talking to the server via REST only |

Quick start with the CLI (see `docs/40_cli_plan.md`):

```sh
nexmap login --server https://nexmap.example.com   # paste a token from /user/tokens
nexmap init                                         # writes .nexmap.json (no secrets)
nexmap whoami
nexmap api /products/{product}/requirements -o json
```

In CI, set `NEXMAP_SERVER`, `NEXMAP_PRODUCT` and `NEXMAP_TOKEN` instead of logging in.

For AI agents working in a repository, `nexmap agent-guide` prints a usage guide to paste into
`AGENTS.md` / `CLAUDE.md`; `nexmap context -o md` gives a product briefing, and `nexmap req show FR-12`,
`constraint show`, `defect list --open`, `test show <test identifier>` and `campaign status` cover the details.

MCP-capable agents (Claude Code and others) can use the same reads as tools: `nexmap init --agents`
registers `nexmap mcp --read-only` in the repository's `.mcp.json` and appends the agent guide to
`CLAUDE.md` (or `AGENTS.md`). The server reuses the CLI's configuration, so nothing else is needed.
