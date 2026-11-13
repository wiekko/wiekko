## Nexmap (product definition, backlog and QA)

This repository's product is managed in Nexmap. Use the `nexmap` CLI to read
the product definition, requirements, constraints, test cases and defects
before and while you change code. Server and product are configured in
`.nexmap.json`; credentials come from `nexmap login` or `NEXMAP_TOKEN`.

### Start here

- `nexmap context -o md` — product briefing: purpose, components and
  capabilities, constraints, work in progress, open defects, testing status.
  Read it once at the start of a task.
- `nexmap req show FR-12 -o md` — one requirement or task: description,
  definition of done, children, linked capabilities, test cases, defects,
  comments. Read it before implementing the item.

If your client has the `nexmap` MCP server configured (`.mcp.json`), the same
reads are available as tools: `get_context`, `get_requirement`,
`list_requirements`, `get_constraint`, `list_constraints`, `get_structure`,
`get_capability`, `list_defects`, `get_defect`, `list_test_cases`,
`get_test_case` and `campaign_status`. Unless the server runs with
`--read-only`, the writes below are tools too: `set_requirement_status`,
`set_defect_status`, `create_backlog_item`, `create_defect` and `add_comment`.
Prefer the tools over the CLI.

### Identifiers

- Requirements, tasks and constraints have identifiers like `FR-12`, `MT-3`,
  `AC-42` (category prefix + number). Use them in commands, commit messages
  and code comments.
- Defects have identifiers like `DF-17`; use them like the ones above.
- Test cases are referenced by their short ID (first 8 characters of the ID,
  shown in lists) or by their test identifier, e.g. the Go test function name.
- Capabilities are referenced by name.

### Commands

| Command | Purpose |
| --- | --- |
| `nexmap req list [--open] [--active] [--kind task] [--status NAME] [--search TEXT]` | Backlog items |
| `nexmap req show <FR-12>` | One requirement or task |
| `nexmap constraint list [--compliance violated]` / `constraint show <AC-42>` | Rules the code must follow |
| `nexmap component list` / `capability show <name>` | Product structure and traceability |
| `nexmap defect list [--open] [--criticality high]` / `defect show <DF-17>` | Known bugs |
| `nexmap test list [--automatic]` / `test show <test identifier>` | Test cases and their results |
| `nexmap campaign status` | Active testing campaign: verdict and failing tests |
| `nexmap export [--tasks]` | The whole specification as one Markdown document |
| `nexmap api [METHOD] <path>` | Raw REST call for anything else (`{product}` is replaced) |

### Changing things

Writes are limited by your role in the product; a refusal (exit 2, "role")
means the user has to do it or grant the role, not that you should retry.
Developers may only change the status of items a product owner has activated
(in kanban, by moving them into an active column; in scrum, by planning them
into a sprint). An item's `active` field says whether it is.

| Command | Purpose | Role |
| --- | --- | --- |
| `nexmap req status FR-12 "In Progress"` | Move an item through the workflow when you start or finish it | developer, on active items only |
| `nexmap defect status DF-17 Done` | Same for a defect you fixed | developer, on active defects only |
| `nexmap comment add FR-12 --body-file -` | Record a decision, question or finding (stdin avoids quoting trouble; works for `DF-17` too; `--type test_case` etc. for test cases and capabilities) | developer (tester on defects and test cases) |
| `nexmap defect create --title … [--test <test identifier>] [--req FR-12] [--criticality high] --body-file -` | Report a bug you found; check `defect list` first | tester |
| `nexmap task create --title … --parent FR-12 [--body-file -] [--dod …]` | Split a requirement into tasks | owner |
| `go test -json ./... \| nexmap test sync [--req FR-12] [--capability <name>] [--pyramid-level unit] --dry-run` | Register new tests as test cases (drop `--dry-run` once the list looks right) | developer |
| `nexmap test link <test identifier> --req FR-12 [--capability <name>]` / `test unlink …` | Record which requirement a test verifies and which capability it belongs to | developer |
| `nexmap constraint update AC-3 --compliance met\|violated` | Record whether a constraint holds after checking it | owner |
| `nexmap constraint create --title … [--category SC] [--compliance …]` | Add a rule the product must follow | owner |
| `nexmap component create --name …` / `component update <name> …` | Change the product structure | owner |
| `nexmap capability create --name … [--component <name>]` / `capability update <name> …` | Same for capabilities | owner |
| `nexmap capability link <name> FR-12 AC-3` / `capability unlink …` | Trace requirements and constraints to a capability | owner |

`component delete` and `capability delete` exist too and need `--yes`; never
delete unless the user asked for exactly that.

Only change what the task at hand calls for, and tell the user what you
changed.

### Output

- `-o md` gives compact markdown (best for reading), `-o json` gives stable
  machine-readable fields (best for scripting), the default is a plain table.
- Exit codes: 0 ok, 1 usage or configuration error, 2 API or authentication
  error, 3 not found.

### Working rules

- Treat constraints (`AC-…`, `SC-…`, `UC-…`) as non-negotiable unless the
  user says otherwise; check `nexmap constraint list` when designing a change.
- A requirement is done when its definition of done holds; the linked test
  cases show how it is verified.
- Prefer the CLI over guessing: if a requirement is unclear, read its
  comments and parent, and ask the user rather than inventing scope.
