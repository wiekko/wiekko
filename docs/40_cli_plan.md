# CLI Implementation Plan

This plan covers the `nexmap` command line client (AC-222, AC-224, SC-105). It has two jobs, in priority order:

1. **Agent entry point** (primary) — give AI coding agents working in a project repository read (and limited write) access to the product definition, QA state and backlog: requirements, tasks, constraints, capabilities, test cases, defects. Both as CLI commands and as an MCP server in the first release.
2. **Repository / CI integration** — ingest automated test results into Nexmap from a build (FR-018, FR-409), without anyone touching the web UI.

## 1. Starting point

What exists today and matters for the CLI:

| Area | State | Consequence for the CLI |
|---|---|---|
| REST API `/api/v1` (`internal/router/api_v1.go`) | Full CRUD for products, components, capabilities, requirements, constraints, defects, test cases, campaigns, comments, attachments, audit | The CLI is a pure API client; no business logic in the CLI (AC-201, AC-224) |
| Auth (`internal/middleware/authenticate.go`, `csrf.go`) | Bearer PATs accepted; PAT requests are CSRF-exempt | CLI auth works today with `Authorization: Bearer <pat>` |
| Response envelope (`internal/httputil`) | `{"data": …}` / `{"error": {"code","message"}}` | One generic decode path in the client |
| Test cases (`model.TestCase`) | `execution_mode=automatic` + `test_identifier` | The mapping key for ingestion already exists, but nothing looks it up |
| Campaign results (`TestingService.SetResultStatus`) | Per-result PATCH; tester role; **failed requires a defect** (FR-502); **only in active campaigns** (FR-505) | Ingestion can't be built from the existing per-result endpoint alone (see §3.1) |
| JSON shapes (`requirementToJSON` etc.) | UUIDs only; no `FR-12` identifier, no status / category names | Agents and humans think in `FR-12` and "In Progress", not UUIDs (see §3.2) |
| `cmd/nexmap` | Is the **server**; `config.Load(os.Args[1:])` owns the flag namespace | Naming collision with the `nexmap utest` command from FR-409 (see §2.1) |

## 2. Architecture

### 2.1 Binary layout

**Decided:** two binaries from one module.

- `cmd/nexmapd` — the server (renamed from `cmd/nexmap`; Justfile, nixpacks, `.gitignore` and docs follow).
- `cmd/nexmap` — the CLI, matching the `go test -v ./... | nexmap utest` wording of FR-409.

Rationale: the server binary embeds web assets, templ output, both SQL drivers and the migration set; CI runners and agent sandboxes should download a small, static, cgo-free client. AC-101 ("server ships as single executable") stays satisfied. The CLI must **not** import `internal/store`, `internal/service`, `internal/database` or `web/` — enforce with a test that inspects `go list -deps ./cmd/nexmap`.

### 2.2 Packages

```
cmd/nexmap/                 main: build info, signal context, exit codes
internal/client/            typed REST client for /api/v1 (DTOs, errors, retries)
internal/cli/               command tree, global flags, config resolution
internal/cli/output/        table / markdown / json renderers
internal/cli/resolve/       FR-12 / status-name / category-prefix ↔ UUID resolution, per-invocation cache
internal/ingest/            test report parsers → []ingest.Result
internal/ingest/gotest/     `go test -json` and `go test -v` text
internal/ingest/junit/      JUnit XML (pytest, jest, maven, gotestsum, …)
```

- `internal/client` defines its own DTOs mirroring the JSON contract instead of reusing `internal/model` — the wire format is the contract, and this keeps the client free of server internals. Request structs used by both controller and client can move to a shared `internal/api` package later if duplication hurts.
- Command dispatch uses the standard library (`flag.FlagSet` per subcommand + a small router) to stay in line with the dependency-light codebase. Revisit only if shell completion becomes a requirement.

### 2.3 Configuration and credentials

Resolution order (later wins): user config → repo config → environment → flags.

| Source | Content | Notes |
|---|---|---|
| `<repo>/.nexmap.json` (found by walking up from cwd) | `server`, `product` (ID), ingestion mapping options | Committed to the repo; **never** contains a token |
| `os.UserConfigDir()/nexmap/config.json` | Per-server tokens, default output format | File mode 0600; written by `nexmap login` |
| Env | `NEXMAP_SERVER`, `NEXMAP_PRODUCT`, `NEXMAP_TOKEN` | Primary path for CI secrets |
| Flags | `--server`, `--product`, `-o/--output`, `--allow-http` | No `--token <value>` flag: it leaks via process listings and shell history. `nexmap login` reads the token from a hidden prompt or, when piped, from stdin |

**Decided:** JSON, keeping the CLI dependency-free. TOML can be added later (same struct, second decoder keyed by file name) if JSON proves impractical.

`nexmap login` takes a PAT pasted on stdin (created in the web UI per SC-106), verifies it with `GET /user/me`, and stores it. Password login is deliberately not offered (SC-105).

Plain `http://` servers are refused unless they are on the local machine (`localhost`, loopback IPs, `*.localhost`), so a token never crosses the network unencrypted (SC-001); `--allow-http` / `NEXMAP_ALLOW_HTTP=1` overrides this deliberately.

### 2.4 Output and exit-code contract

- `-o table` (default on a TTY), `-o md` (markdown, best for agent context windows), `-o json` (stable, machine-readable; default when stdout is not a TTY is **still table** — agents and scripts must ask for json explicitly so behaviour never depends on the environment).
- JSON output is the `data` payload enriched with resolved fields (`identifier`, `status_name`, …) and is versioned via a top-level `schema` field once it stabilises.
- Exit codes: `0` ok, `1` usage/config error, `2` API/auth error, `3` not found, `4` tests failed (only with `utest --exit-code`). Errors go to stderr as one line; with `-o json` as `{"error":{…}}`.

## 3. Server-side prerequisites

The CLI must use the same REST API as everything else (AC-224), so gaps get closed on the server, not worked around in the client.

### 3.1 Test result ingestion endpoint (blocking for CI integration)

New: `POST /api/v1/products/{productId}/test-runs`

```json
{
  "source": "go-test",
  "campaign_id": "",            // optional; default = the product's active campaign (FR-322)
  "environment": "linux-amd64", // optional runtime environment name
  "revision": "4161949",        // optional VCS metadata, stored on events for traceability
  "branch": "main",
  "results": [
    {"test_identifier": "nexmap.net/internal/service.TestSetResultStatus", "status": "passed", "duration_ms": 12},
    {"test_identifier": "…TestCampaignVerdict", "status": "failed", "output": "…truncated…"}
  ]
}
```

Response: per-identifier outcome (`updated`, `unchanged`, `unknown_test_case`, `not_in_campaign`, `defect_created`, `defect_reused`) plus a summary (`recorded`, counts per status). Service method `TestingService.IngestRun` does, in one transaction:

1. **Resolve the campaign**: explicit ID or the single active one. **No active campaign → the upload is accepted and discarded**: respond 200 with `recorded: false`, the summary and per-test outcomes, but write nothing — no results, no events, no defects. CI does not break between campaigns, and the data model stays unchanged (FR-505 intact).
2. Map identifiers to automatic test cases of the product (new store lookup `TestCaseStore.ListByIdentifiers`). Subtests (`TestX/case_1`) roll up to `TestX` when only the parent is registered; the parent takes the worst status and the concatenated output of its failing subtests.
3. Map the environment name to a runtime environment and pick the matching result row (test cases without an environment matrix ignore it).
4. Skip results whose status is unchanged (keeps the FR-042 event log meaningful under repeated CI runs).
5. **Failures get an auto-created defect (FR-502).** Reuse an open defect already linked to that test case and recorded on the result; otherwise create one linked to the test case:
   - title `Automated test failed: <identifier>`;
   - description: run metadata (source, environment, revision, branch) followed by the captured test output in a fenced code block, truncated to a server-side limit (e.g. 64 KiB, keeping the tail where the assertion failure usually is) with a truncation note;
   - criticality `medium`, environment set from step 3.

   If the output is too large even after truncation or the parser could not capture it, the defect is still created with metadata only. This keeps FR-504 working: closing the defect moves the result to re-test, and the next green CI run marks it passed.
6. **Defects are only created for results that are actually recorded.** A failure that is discarded — no active campaign (step 1), unknown identifier, or test case not in the campaign's scope — creates no defect: a defect from automation exists to explain a campaign result, and without one it would be an unlinked duplicate created on every push between campaigns. These failures are still reported back in the response (and printed by the CLI), so the build log shows them.
7. Record result events with the PAT user, plus `revision`/`branch` (new nullable columns on `campaign_result_events`, migration for both dialects + parity test).

Role: tester (same as `SetResultStatus`).

### 3.2 Agent-friendly reads

Implemented in phases 0–1:

- `identifier` (`FR-12`, `MT-3`, `AC-42`) and `sequence_number` in requirement, task and constraint JSON.
- Identifiers are accepted wherever a requirement or constraint ID is in the path (`GET /products/{p}/requirements/FR-12`, also `{reqId}` in link routes). A route wrapper (`controller.ResolveRefs`) rewrites the path value via `BacklogService.ResolveRequirementRef` / `ConstraintService.ResolveConstraintRef`, so handlers stay unchanged. An identifier only resolves when its prefix matches the item's category (`MT-12` never aliases `FR-12`).
- New read routes: `requirements/{id}/test-cases`, `requirements/{id}/defects`, `test-cases/{id}/results`, `test-cases/{id}/defects`, `capabilities/{id}/test-cases`, `capabilities/{id}/defects`; `test-cases?test_identifier=…&execution_mode=…` filters; `username` in the members listing.
- `GET /api/v1/version` (public): server version + API level.
- Unknown IDs answer 404 instead of 400 with a database message.

Still open:

- Defects have no human identifier today. Add a per-product sequence (prefix e.g. `DF`) so agents and commit messages can say `DF-7`. Needs a migration + backfill; the CLI uses 8-character ID prefixes meanwhile.
- ~~Product scoping of entity IDs~~ — fixed: every service method that takes an entity ID (path IDs and references inside create/update payloads, link targets, audit queries) now checks that the entity belongs to the product in the URL via a shared ownership checker (`internal/service/ownership.go`), answering 404 otherwise. Covered by `TestCrossProductIsolation` (both engines) and an API-level e2e test.

### 3.3 Later (not blocking v1)

- `GET /products/{p}/search?q=` (FR-701) → `nexmap search`.
- PAT scopes (read-only, product-bound). Today a PAT carries the full power of its user; a CI token or an agent token should be narrower. Until then, document "use a dedicated tester-role CI user".
- Server-side pagination on API lists (`ListParams` already exists in `internal/store/list.go`) once products grow.
- Campaign scope endpoint (`AddCampaignTestCases` exists in the service but has no API route).

## 4. Command set

### 4.1 Setup and plumbing

| Command | Purpose |
|---|---|
| `nexmap init` | Create `.nexmap.json` interactively (pick server + product from `GET /products`) |
| `nexmap login` / `logout` / `whoami` | Token handling (§2.3) |
| `nexmap version` | Client version, server version, compatibility |
| `nexmap api <METHOD> <path> [--data @file]` | Raw authenticated call — escape hatch for scripts and agents |

### 4.2 CI integration

```
go test -json ./... | nexmap utest --format go-json --env linux-amd64
go test -v ./...    | nexmap utest                     # format auto-detected
nexmap utest --format junit reports/*.xml
```

- Formats: `go-json` (preferred, exact), `go-text` (the FR-409 example; best effort), `junit` (covers most other ecosystems). Auto-detection by sniffing the first bytes.
- Each parsed result carries its captured output for failures (go-json: the `output` events of that test; go-text: lines between `=== RUN` and `--- FAIL`; junit: `<failure>` message/body plus `system-out`/`system-err`). The CLI pre-truncates to the same limit as the server to keep uploads small.
- When the server reports `recorded: false`, the CLI prints one clear line ("no active campaign — results not recorded") and still exits 0 unless `--exit-code` applies.
- Reading from stdin, the input is passed through to stdout (`--quiet` to suppress), so it can sit at the end of an existing pipeline without hiding test output.
- `--dry-run` parses and prints the identifier → test case mapping without writing — the setup/debugging tool.
- `--exit-code` exits 4 if any parsed test failed (pipes swallow `go test`'s own exit status unless `pipefail` is set; document both).
- `--identifier-style package|name` controls how Go identifiers are built (`pkg/path.TestName` vs `TestName`), matching whatever convention the product uses in `test_identifier`.
- `--revision` / `--branch` default from `GITHUB_SHA`, `CI_COMMIT_SHA`, `GITHUB_REF_NAME`, … or `git rev-parse`.
- `nexmap test sync --format go-json` (developer role): create missing automatic test cases from a run, so a repo can bootstrap its test catalogue instead of typing identifiers into the UI. Opt-in only.

Ship copy-paste snippets for GitHub Actions, GitLab CI and Forgejo Actions in `docs/`.

### 4.3 Agent access (primary use case)

Read:

| Command | Output |
|---|---|
| `nexmap context [-o md]` | One-shot product briefing: description, components → capabilities tree, constraints with compliance state, active sprint / active items, open defects by criticality, active campaign verdict. Sized for a context window; the single most important agent command |
| `nexmap req list [--kind task] [--status …] [--active] [--sprint current]` / `req show FR-12` | `show` includes description, definition of done, parent/children, linked capabilities, linked test cases, comments |
| `nexmap constraint list` / `show AC-42` | |
| `nexmap capability list` / `show`, `nexmap component list` | |
| `nexmap defect list [--open] [--criticality high]` / `show` | Includes linked test case, environment, comments |
| `nexmap test list [--automatic]` / `show <id\|identifier>` | Includes latest campaign results |
| `nexmap campaign status` | Active campaign progress + verdict (FR-041) |
| `nexmap search <q>` | After §3.3 |

Write (all bounded by the PAT user's product role; nothing bypasses `authz`):

| Command | Purpose |
|---|---|
| `nexmap req status FR-12 "In Review"` | Status by name (FR-407) |
| `nexmap req create --kind task --category MT --title … [--parent FR-12] [--body-file -]` | Agents splitting work (FR-326) |
| `nexmap defect create --title … --test <identifier> [--body-file -]` | Report a bug found while coding |
| `nexmap comment add FR-12 --body-file -` | Leave notes on any commentable entity |

Agent ergonomics:

- `nexmap agent-guide` prints a short markdown usage guide (commands, identifier conventions, json output) meant to be pasted into `AGENTS.md` / `CLAUDE.md`; `nexmap init --agents` appends it.
- Long bodies are taken from `--body-file -` (stdin) to avoid shell-quoting pitfalls.
- Every list supports `-o json` with stable field names; markdown output uses the same headings every time.

### 4.4 MCP server (first release)

`nexmap mcp` runs a stdio Model Context Protocol server exposing the same read/write operations as tools (`get_context`, `get_requirement`, `list_requirements`, `list_defects`, `get_test_case`, `campaign_status`, `set_requirement_status`, `create_task`, `create_defect`, `add_comment`) on top of `internal/client`. This gives Claude Code and other MCP-capable agents typed tools instead of shelling out.

- **One operation layer, two front ends.** Each agent operation lives once in `internal/cli/ops` (input struct → result struct, using `internal/client` + `internal/cli/resolve`). CLI commands and MCP tools are thin adapters over it, so they cannot drift apart, and MCP tool input schemas are generated from the same input structs.
- Results are returned as markdown text (the `-o md` renderer) plus structured JSON content, so agents get readable context and exact fields.
- Config and token resolution are identical to the CLI (§2.3), so a repo with `.nexmap.json` needs only `{"command": "nexmap", "args": ["mcp"]}` in `.mcp.json`; `nexmap init --agents` writes that entry.
- `--read-only` flag hides all write tools — the recommended default until PAT scopes (§3.3) exist.
- Implementation: the official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`) — protocol details (capability negotiation, schema, transport) are not worth hand-rolling. It is the only new dependency of the CLI and is confined to `internal/cli/mcp` (enforced by `cmd/nexmap/deps_test.go`). It requires Go 1.25, so `go.mod` moved from 1.24.

Phase 2 as built:

- Read tools: `get_context`, `list_requirements`, `get_requirement`, `list_constraints`, `get_constraint`, `get_structure`, `get_capability`, `list_defects`, `get_defect`, `list_test_cases`, `get_test_case`, `campaign_status` — all annotated read-only. List tools return `{"items": [...]}` as structured content (MCP requires an object); show tools return the same object as `-o json`.
- Input schemas come from the `ops` query structs' `json`/`jsonschema` tags; show tools take `{"ref": "..."}`. Unknown arguments are rejected.
- Each tool call gets a fresh `ops.Session`, so a long-running server never serves stale status or category names.
- Missing configuration or credentials do not stop the server: every tool call returns the reason (e.g. "run `nexmap login`") as a tool error the agent can relay.
- Server instructions (sent on initialize) are a short tool-oriented version of the agent guide.
- `--read-only` is accepted but has nothing to hide until phase 3 adds write tools.
- `nexmap init --agents` additionally writes the `nexmap` entry (`nexmap mcp --read-only`) into `.mcp.json`, preserving other servers and keeping an existing `nexmap` entry unless `--force`, and appends the agent guide once to `CLAUDE.md` if present, else `AGENTS.md`. An existing `.nexmap.json` is kept with `--agents` instead of being an error.

Phase 3 as built:

- CLI: `req|task status <ref> <status>`, `req|task create`, `defect status <short ID> <status>`, `defect create [--test] [--req]`, `comment add <ref> [--type]`. Long texts via `--body`/`--body-file -` (and `--dod`/`--dod-file` for create). Status names may be unquoted (`req status FR-12 in review`).
- MCP write tools (left out by `--read-only`): `set_requirement_status`, `set_defect_status`, `create_backlog_item` (task by default; replaces the planned `create_task`), `create_defect`, `add_comment`. Annotated not read-only and not destructive; the status setters as idempotent. Inputs are the `ops` write structs.
- Every write returns the resulting detail view (same as `show`), so the agent sees the effect.
- Roles are entirely the server's: owner creates requirements/tasks (FR-308a), developer sets requirement and defect status (FR-407) and comments, tester creates defects and comments on defects/test cases. A 403 exits 2 with a hint about the role; MCP returns it as a tool error. Verified end to end with the demo viewer, tester and developer tokens plus an admin.
- References (parent, category, status, test case, requirement) are resolved before the first write, so bad input creates nothing. `defect create` links after creating; if a link fails, the error names the created defect.
- New items get the workflow's default status unless one is given: the server stores an empty status when none is sent and does not apply `is_default` itself (this also affects items created through the web UI and REST API; worth fixing in the service layer).
- `init --agents` still registers `nexmap mcp --read-only`; drop the flag in `.mcp.json` to allow writes.
- Not enforced by the server today: FR-407's "if they have been activated upfront" — developers can change the status of items that are not active.

### 4.5 Import / export (FR-319)

`nexmap req export > backlog.md` / `nexmap req import backlog.md [--dry-run]` in the To Do Markdown format, keyed by identifier so round-trips update instead of duplicating. Separate phase; needs a precise format spec first.

## 5. Phases

Ordered by priority: agent access first, CI ingestion after. Phases 0–4 make up the first release.

| Phase | Scope | Done when |
|---|---|---|
| **0 — Groundwork** ✅ | Binary split (§2.1); `internal/client` with auth, envelope decoding, typed errors, retry on 429/5xx for idempotent calls; config + `init/login/whoami/version/api`; `GET /version` | `nexmap whoami` works against a dev server with a PAT; dependency-guard test green |
| **1 — Agent reads** ✅ | §3.2 identifiers + ref resolution; `internal/cli/ops` read operations; `context`, `req/constraint/capability/component/defect/test/campaign` list+show; md/json renderers; `agent-guide` | An agent given only `agent-guide` can answer "what does FR-12 require and which tests cover it" |
| **2 — MCP (read-only)** ✅ | `nexmap mcp --read-only` over the phase-1 ops; `init --agents` writes `.mcp.json` | Claude Code in a repo with `.nexmap.json` can call `get_context` / `get_requirement` without further setup |
| **3 — Agent writes** ✅ | Write ops (`req status/create`, `defect create`, `comment add`) in CLI and MCP | Role checks verified for viewer/tester/developer tokens; `--read-only` hides them |
| **4 — CI ingestion** | §3.1 endpoint + service + migration; `internal/ingest` parsers with output capture; `nexmap utest` incl. `--dry-run`, `--exit-code`, env/revision detection; `test sync`; CI snippets | Nexmap's own `go test -json ./...` lands in a campaign of a dev-seed product; repeated runs produce no duplicate events; a failure creates/reuses a defect containing the test output; closing it and re-running green ends passed; with no active campaign nothing is written |
| **5 — Later** | FR-319 import/export, FR-701 search (server + CLI + MCP), PAT scopes, defect identifiers | — |

## 6. Testing

- **Parsers**: table tests over committed fixtures (`internal/ingest/testdata/`): real `go test -json` and `-v` output incl. subtests, panics, build failures, skips, parallel interleaving; JUnit from pytest, jest, maven surefire.
- **Client**: `httptest.Server` tests for envelope/error decoding, auth header, retries, timeouts.
- **Ingestion service**: integration tests on both dialects via `internal/database/dbtest` (as the store layer does): no active campaign writes nothing (no results, events or defects), unknown identifiers and out-of-scope test cases create no defects, subtest roll-up, environment matrix, unchanged-status no-op, defect reuse vs creation, captured output and truncation in the defect description, FR-504 retest loop.
- **MCP**: drive `nexmap mcp` in-process over an in-memory transport against the same e2e server; assert tool listing (with and without `--read-only`) and tool results match the CLI's json output for the same op.
- **End-to-end**: spin up `router.New` on an in-memory SQLite with a seeded product and a PAT, run CLI commands in-process (command entry point takes `args, stdin, stdout, stderr`), assert on json output. Covers AC-224 for real.
- **Dependency guard**: `cmd/nexmap` must not depend on `internal/store|service|database`, `web/`.

## 7. Decisions

| # | Topic | Decision |
|---|---|---|
| 1 | Binary naming | `nexmap` = CLI, `nexmapd` = server |
| 2 | Automated failures (FR-502) | Auto-create (or reuse) a defect per failing test, recording the captured test output |
| 3 | No active campaign | Model stays unchanged; the upload is accepted and discarded. No defects are created either — automated defects only exist for recorded campaign results (§3.1 step 6) |
| 4 | Repo config format | JSON; TOML may be added later |
| 5 | MCP | In the first release; agent access is the primary use case, ahead of CI capture, and the phases are ordered accordingly |
