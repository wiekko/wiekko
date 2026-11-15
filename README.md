# Nexmap

Nexmap Product Management Solution

Nexmap keeps a product's requirements, constraints, backlog, test cases, testing campaigns and defects in
one place. It consists of a server (`nexmapd`) with a web UI and REST API, and a command line client
(`nexmap`) for repositories, CI pipelines and AI agents.

- Using Nexmap: [docs/guide/concepts.md](docs/guide/concepts.md), then
  [getting started](docs/guide/getting-started.md)
- Deploying and running Nexmap: [docs/admin/installation.md](docs/admin/installation.md)
- Setting up the CLI: [docs/cli/setup.md](docs/cli/setup.md)
- Recording test results from CI: [docs/cli/ci-integration.md](docs/cli/ci-integration.md)
- Giving AI agents access: [docs/cli/ai-agents.md](docs/cli/ai-agents.md)
- All documentation: [docs.nexmap.net](https://docs.nexmap.net), or [docs/index.md](docs/index.md) in the repository
- Building and testing Nexmap itself: [CONTRIBUTING.md](CONTRIBUTING.md)

## CLI quick start

```sh
curl -fsSL https://nexmap.net/install.sh | sh       # Windows: irm https://nexmap.net/install.ps1 | iex
nexmap login --server https://nexmap.example.com   # paste a token from /user/tokens
nexmap init                                         # writes .nexmap.json (no secrets)
nexmap whoami
nexmap api /products/{product}/requirements -o json
```

In CI, set `NEXMAP_SERVER`, `NEXMAP_PRODUCT` and `NEXMAP_TOKEN` instead of logging in, and pipe test output into
`nexmap utest` to record it in the active testing campaign (`go test -json ./... | nexmap utest --env linux`).

For AI agents working in a repository, `nexmap agent-guide` prints a usage guide to paste into
`AGENTS.md` / `CLAUDE.md`; `nexmap context -o md` gives a product briefing, and `nexmap req show FR-12`,
`constraint show`, `defect list --open`, `test show <test identifier>` and `campaign status` cover the details.

MCP-capable agents (Claude Code and others) can use the same reads as tools: `nexmap init --agents`
registers `nexmap mcp --read-only` in the repository's `.mcp.json` and appends the agent guide to
`CLAUDE.md` (or `AGENTS.md`). The server reuses the CLI's configuration, so nothing else is needed.
Agents can also change things within their user's role: `req status`, `defect status`, `comment add`,
`defect create` and `task create` (CLI), and the matching MCP tools unless the server runs with `--read-only`.
