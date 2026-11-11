# AI agents

AI coding agents working in a repository can read the product definition, backlog and QA state from
nexmap, and, within their user's role, update it. They use either the CLI or, for agents that speak the
Model Context Protocol (Claude Code and others), the same operations as MCP tools.

## Setting up a repository

After [connecting the repository](setup.md#connecting-a-repository) and logging in:

```sh
nexmap init --agents
```

This does two things on top of `nexmap init` (an existing `.nexmap.json` is kept):

- registers the MCP server in the repository's `.mcp.json`, keeping any other servers in it:

  ```json
  {
    "mcpServers": {
      "nexmap": {"command": "nexmap", "args": ["mcp", "--read-only"]}
    }
  }
  ```

  An existing `nexmap` entry is left alone unless you pass `--force`.
- appends the agent guide (`nexmap agent-guide`) to `CLAUDE.md` if that exists, otherwise to
  `AGENTS.md`, once.

The MCP server uses the same configuration and token as the CLI, so nothing else is needed. If the
configuration or token is missing, the server still starts and every tool call returns the reason
(e.g. "run `nexmap login`"), which the agent can pass on to you.

## What agents can do

`nexmap agent-guide` prints the guide agents are given: the commands and tools, how items are
referenced, and working rules such as treating constraints as binding. Run it to see exactly what an
agent is told. In short:

- **Read:** `nexmap context -o md` (or the `get_context` tool) for a product briefing, then requirements,
  tasks, constraints, structure, defects, test cases and the campaign status as needed.
- **Write:** set the status of requirements, tasks and defects, add comments, report defects, and create
  tasks, each only as far as the user's role allows (see below).

## Allowing writes

`init --agents` registers the MCP server with `--read-only`, which leaves out all tools that change
data. To let agents write through MCP, remove `--read-only` from the entry in `.mcp.json`. The CLI's
write commands are always available to an agent that can run shell commands.

Writes are checked by the server against the token user's role in the product, exactly as in the web UI:

| Action | Role needed |
|---|---|
| Set the status of a requirement, task or defect | developer, and only on items a product owner has activated (directly or by planning them into a sprint) |
| Comment | developer; tester on defects and test cases |
| Report a defect | tester |
| Create a task | product owner |

Each role includes the ones below it (product owner, developer, tester, viewer), and administrators may
do everything.

A token carries every right of its user, and tokens cannot be narrowed yet. Give agents their own user
with the lowest role that covers what they should do, rather than your personal token.
