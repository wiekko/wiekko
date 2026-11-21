# Wiekko

Wiekko Product Management Solution

> Wiekko is developed in a private repository; the public repository on GitHub is a read-only mirror.
> Pull requests and issues are not accepted at this time.

Wiekko keeps a product's requirements, constraints, backlog, test cases, testing campaigns and defects in
one place. It consists of a server (`wiekkod`) with a web UI and REST API, and a command line client
(`wiekko`) for repositories, CI pipelines and AI agents.

- Using Wiekko: [docs/guide/concepts.md](docs/guide/concepts.md), then
  [getting started](docs/guide/getting-started.md)
- Deploying and running Wiekko: [docs/admin/installation.md](docs/admin/installation.md)
- Setting up the CLI: [docs/cli/setup.md](docs/cli/setup.md)
- Recording test results from CI: [docs/cli/ci-integration.md](docs/cli/ci-integration.md)
- Giving AI agents access: [docs/cli/ai-agents.md](docs/cli/ai-agents.md)
- All documentation: [docs.wiekko.net](https://docs.wiekko.net), or [docs/index.md](docs/index.md) in the repository
- Building and testing Wiekko itself: [CONTRIBUTING.md](CONTRIBUTING.md)

## CLI quick start

```sh
curl -fsSL https://wiekko.net/install.sh | sh       # Windows: irm https://wiekko.net/install.ps1 | iex
wiekko login --server https://wiekko.example.com   # paste a token from /user/tokens
wiekko init                                         # writes .wiekko.json (no secrets)
wiekko whoami
wiekko api /products/{product}/requirements -o json
```

In CI, set `WIEKKO_SERVER`, `WIEKKO_PRODUCT` and `WIEKKO_TOKEN` instead of logging in, and pipe test output into
`wiekko utest` to record it in the active testing campaign (`go test -json ./... | wiekko utest --env linux`).

For AI agents working in a repository, `wiekko agent-guide` prints a usage guide to paste into
`AGENTS.md` / `CLAUDE.md`; `wiekko context -o md` gives a product briefing, and `wiekko req show FR-12`,
`constraint show`, `defect list --open`, `test show <test identifier>` and `campaign status` cover the details.

MCP-capable agents (Claude Code and others) can use the same reads as tools: `wiekko init --agents`
registers `wiekko mcp --read-only` in the repository's `.mcp.json` and appends the agent guide to
`CLAUDE.md` (or `AGENTS.md`). The server reuses the CLI's configuration, so nothing else is needed.
Agents can also change things within their user's role: `req status`, `defect status`, `comment add`,
`defect create` and `task create` (CLI), and the matching MCP tools unless the server runs with `--read-only`.

## License

Wiekko is licensed under the [European Union Public Licence v1.2](LICENSE) (`EUPL-1.2`). Outside contributions
are not accepted at this time; see [CONTRIBUTING.md](CONTRIBUTING.md).
