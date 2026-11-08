# nexmap

Nexmap Product Management Solution

## Building

nexmap needs Go 1.26 or newer (the `go` line in `go.mod`); the `toolchain` line names the release it is
built and tested with. Older distribution packages work as well: since Go 1.21, `go` reads that line,
fetches the named toolchain once (from the Go module proxy, verified against the Go checksum database)
and uses it for this repository, `gopls` included. This needs automatic toolchain switching, which some
distributions turn off:

```sh
go env GOTOOLCHAIN              # "auto" or "path+auto": nothing to do
go env -w GOTOOLCHAIN=auto      # if it prints "local"
```

Local builds need `just` and the Tailwind CSS v4 standalone CLI (`tailwindcss`) on the `PATH`. The templ
generator needs no install: the `justfile` runs it at the version pinned in `go.mod`.

```sh
just build     # vet and build nexmapd and nexmap
just dev       # regenerate templates and CSS on change, run the server
just test      # SQLite only: PostgreSQL subtests skip without a server
just test-pg   # also PostgreSQL, on a throwaway server in a container
```

Integration tests run on both SQLite and PostgreSQL to catch dialect differences. The PostgreSQL runs need
a server named by `NEXMAP_TEST_POSTGRES_DSN` (a `postgres://` URL whose role may create databases);
`just test-pg` starts one with podman or docker and removes it afterwards. Without a server those subtests
are skipped, not failed; set `NEXMAP_TEST_POSTGRES=required` where PostgreSQL coverage must not be
skipped silently, such as CI.

For release binaries, `just container-build [os] [arch]` builds in a container (podman, else docker;
override with `CONTAINER_ENGINE`) with Go, templ and Tailwind pinned in the `Dockerfile`, and
writes the binaries to `build/<os>-<arch>/`. The host needs only the container engine and `just`.

## Deployment

The `Dockerfile`'s default stage is the server image: `nexmapd` alone on a distroless base, running as a
non-root user, listening on port 8080 on all interfaces, with a `HEALTHCHECK` (`nexmapd healthcheck`).
It keeps the SQLite database and attachments in `/data`, so mount persistent storage there. Migrations
run automatically when the server starts.

On Coolify, create the application with the **Dockerfile** build pack and:

- add persistent storage (a volume) with destination path `/data`;
- set the environment variables, at least `NEXMAP_ENCRYPTION_KEY`, and `NEXMAP_ADMIN_USER` /
  `NEXMAP_ADMIN_PASSWORD` for the first start; for PostgreSQL, `NEXMAP_DB_DRIVER=postgres` and
  `NEXMAP_DB_DSN`;
- optionally make `SOURCE_COMMIT` available at build time, so the binaries report `dev-<commit>`
  instead of `dev`.

Ports (8080) and the health check come from the image. When building with podman, pass
`--format docker`: the default OCI image format drops the `HEALTHCHECK`.

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

In CI, set `NEXMAP_SERVER`, `NEXMAP_PRODUCT` and `NEXMAP_TOKEN` instead of logging in, and pipe test output into
`nexmap utest` to record it in the active testing campaign (`go test -json ./... | nexmap utest --env linux`);
see `docs/50_ci_integration.md` for setup and CI snippets.

For AI agents working in a repository, `nexmap agent-guide` prints a usage guide to paste into
`AGENTS.md` / `CLAUDE.md`; `nexmap context -o md` gives a product briefing, and `nexmap req show FR-12`,
`constraint show`, `defect list --open`, `test show <test identifier>` and `campaign status` cover the details.

MCP-capable agents (Claude Code and others) can use the same reads as tools: `nexmap init --agents`
registers `nexmap mcp --read-only` in the repository's `.mcp.json` and appends the agent guide to
`CLAUDE.md` (or `AGENTS.md`). The server reuses the CLI's configuration, so nothing else is needed.
Agents can also change things within their user's role: `req status`, `defect status`, `comment add`,
`defect create` and `task create` (CLI), and the matching MCP tools unless the server runs with `--read-only`.
