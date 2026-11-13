# CLI decisions

Decisions behind the `nexmap` command line client and its MCP server: what was chosen, and why.

## Two binaries: `nexmap` is the CLI, `nexmapd` the server

The server binary embeds the web assets, templ output, both SQL drivers and the migrations. CI runners and
agent sandboxes should download a small client instead, so the CLI is a separate binary. The CLI name
matches the `nexmap utest` wording of FR-409; the server, previously `cmd/nexmap`, became `nexmapd`.
AC-101 (the server ships as a single executable) still holds.

The CLI is a pure REST client (AC-224) with no business logic of its own (AC-201). It must not import the
server's `internal/store`, `internal/service`, `internal/database` or `web/` packages; `cmd/nexmap/deps_test.go`
enforces this.

## Agent access first, CI ingestion second

The CLI has two jobs: giving AI coding agents access to the product definition, QA state and backlog, and
recording test results from CI. Agent access is the primary use case, so it was built first, and the MCP
server shipped in the first release rather than later.

## Configuration in JSON, tokens never in the repository

The repository file `.nexmap.json` holds only the server and product and is meant to be committed. Tokens
live in the user configuration file (mode 0600) or in `NEXMAP_TOKEN`. JSON keeps the CLI free of
dependencies; TOML can be added later as a second decoder for the same structure if JSON proves
impractical.

There is no `--token` flag, since a token on the command line leaks through shell history and process
listings; `nexmap login` reads it from a hidden prompt or stdin. Password login is not offered: the CLI
authenticates with personal access tokens only (SC-105).

## Plain HTTP only to the local machine

`http://` servers are refused unless they are on the local machine, so a token never crosses the network
unencrypted (SC-001). `--allow-http` overrides this deliberately.

## Output does not depend on the terminal

The default output is a table even when stdout is not a terminal. Scripts and agents must ask for
`-o json` (or `-o md`) explicitly, so a command's output never changes with the environment it runs in.
Exit codes (0 ok, 1 usage, 2 API, 3 not found, 4 tests failed) are part of the same contract.

## One operation layer, two front ends

Each agent operation exists once, in `internal/cli/ops`, as an input struct and a result struct on top of
the REST client. CLI commands and MCP tools are thin adapters over it, so they cannot drift apart, and the
MCP tools' input schemas are generated from the same structs.

The MCP server uses the official Go SDK (`github.com/modelcontextprotocol/go-sdk`) rather than a
hand-rolled protocol implementation, and it is confined to `internal/cli/mcp`. Each tool call gets a fresh session, so a long-running server never serves stale
status or category names.

## Read-only MCP by default

`nexmap init --agents` registers the MCP server with `--read-only`, so writes through an agent are
opt-in. This is a default, not a guarantee: an agent that runs shell commands can call the CLI's write
commands. The guarantee is a read-only token ([access tokens](access-tokens.md)), which the server
enforces. The CLI enforces nothing on its own.

## Identifiers instead of UUIDs

Agents and people think in `FR-12` and "In Progress", not in UUIDs. The API therefore returns identifiers
alongside IDs and accepts them wherever a requirement, constraint or defect ID appears in a path. An
identifier only resolves when its prefix matches the item's category (for defects, the product's defect
prefix), so `MT-12` never aliases `FR-12`. Test cases have no human identifier; the CLI references them
by the first 8 characters of their ID or by their test identifier.

Defects number from a counter of their own rather than the backlog's: a defect is not backlog
specification, and automated test runs can file many of them, which would otherwise eat into the
requirement numbering. Their prefix is a product setting, not a category, because defects have no
categories; it shares the category prefixes' namespace so that no identifier is ambiguous.

## The specification export is rendered by the server

`nexmap export` fetches a finished Markdown document from `GET /products/{p}/specification` rather than
assembling it from the list endpoints. Choosing what goes in and in which order is product logic (AC-201),
one request is cheaper than dozens, and other clients of the REST API get the same document.

The document is made for version control: categories in their configured order, items by number, the
structure by name (compared in Go, not by the database's collation), normalised line endings and
whitespace, and no timestamp, status, priority or compliance state, all of which change without the
specification changing. An item is nested below its parent only within one category; across categories
it stands alone and says which item it refines, so every item stays in its own category's section.

There is no import (FR-906). Turning prose into formal requirements needs judgement that an AI agent
brings and a parser does not; the agent creates items through the CLI or REST API, and the identifiers
in an export let it update items instead of duplicating them.
