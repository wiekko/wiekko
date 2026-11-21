# AI agents

AI coding agents working in a repository can read the product definition, backlog and QA state from
Wiekko, and, within their user's role, update it. They use either the CLI or, for agents that speak the
Model Context Protocol (Claude Code and others), the same operations as MCP tools.

## Setting up a repository

After [connecting the repository](setup.md#connecting-a-repository) and logging in:

```sh
wiekko init --agents
```

This does two things on top of `wiekko init` (an existing `.wiekko.json` is kept):

- registers the MCP server in the repository's `.mcp.json`, keeping any other servers in it:

  ```json
  {
    "mcpServers": {
      "wiekko": {"command": "wiekko", "args": ["mcp", "--read-only"]}
    }
  }
  ```

  An existing `wiekko` entry is left alone unless you pass `--force`.
- appends the agent guide (`wiekko agent-guide`) to `CLAUDE.md` if that exists, otherwise to
  `AGENTS.md`, once.

The MCP server uses the same configuration and token as the CLI, so nothing else is needed. If the
configuration or token is missing, the server still starts and every tool call returns the reason
(e.g. "run `wiekko login`"), which the agent can pass on to you.

## What agents can do

`wiekko agent-guide` prints the guide agents are given: the commands and tools, how items are
referenced, and working rules such as treating constraints as binding. Run it to see exactly what an
agent is told. In short:

- **Read:** `wiekko context -o md` (or the `get_context` tool) for a product briefing, then requirements,
  tasks, constraints, structure, defects, test cases and the campaign status as needed. `wiekko export`
  (or the `export_specification` tool) gives the whole specification in one Markdown document, which an
  agent can keep a spec file in the repository in sync with. The tool also returns the document's SHA-256,
  so the agent can tell whether the file is current without comparing it. The tool is available on a
  read-only server too.
- **Work from documents:** protocols, schemas and other definitions live in Wiekko as documents
  (`wiekko doc show DOC-3`, the `get_document` tool). A requirement, constraint or test case names the
  documents linked to it, so an agent finds the definition it has to implement or check code against;
  `doc diff` (the `document_diff` tool) shows what changed in a definition since the code was written.
- **Turn documents into requirements:** Wiekko has no import. An agent can read a document you give it
  and create the requirements with the CLI, and, using the identifiers from `wiekko export`, update
  existing ones instead of duplicating them.
- **Write:** set the status of requirements, tasks and defects, add comments, report defects, create
  tasks and move items below others, create and update constraints (including whether they are met), change the product structure,
  write documents and restore their earlier revisions, link requirements, constraints and test cases to
  capabilities and documents, and test cases to requirements, each
  only as far as the user's role allows (see below). The MCP server offers none of the CLI's deletions.

## Allowing writes

`init --agents` registers the MCP server with `--read-only`, which leaves out all tools that change
data. To let agents write through MCP, remove `--read-only` from the entry in `.mcp.json`. The CLI's
write commands are always available to an agent that can run shell commands, so `--read-only` is a
convenience, not a guarantee: give an agent that should only read a token created with the *Agent,
read-only* preset, and the server refuses its writes. The *Agent, spec author* preset suits an agent that
maintains the specification: it writes the structure, the backlog, documents and comments, reads defects
and tests, and cannot change the product's settings.

Writes are checked by the server against the token user's role in the product, exactly as in the web UI:

| Action | Role needed |
|---|---|
| Set the status of a requirement, task or defect | developer, and only on items a product owner has activated (kanban: by moving them into an active column; scrum: by planning them into a sprint) |
| Comment | developer; tester on defects and test cases |
| Report a defect | tester |
| Link a test case to a requirement, capability or component; create test cases from a test run | developer |
| Create a task | product owner |
| Create or update a constraint, component or capability; link capabilities | product owner |
| Create, update, link or restore a document | product owner |

Each role includes the ones below it (product owner, developer, tester, viewer), and administrators may
do everything.

A token's restriction applies on top: the server also refuses writes outside the token's scopes, above
its role cap, or in products it is not restricted to. Give an agent its own token, narrowed to what it
should do, rather than the token you use yourself.
