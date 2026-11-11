# Setting up the CLI

`nexmap` is the command line client for a nexmap server. It talks to the server only through the
[REST API](../api/rest.md), so it needs nothing but the server's URL and a personal access token.

## Installing

Where your Go module proxy can reach the module:

```sh
go install nexmap.net/cmd/nexmap@<version>
```

Alternatively, build it from a checkout (`just build`, or `just container-build` for release binaries;
see [CONTRIBUTING.md](https://forge.my-universe.com/daemotron/nexmap/src/branch/main/CONTRIBUTING.md)). The binary has no runtime dependencies.

## Logging in

1. In the web UI, open **API Tokens** in the user menu (`/user/tokens`) and create a token.
2. Store it:

   ```sh
   nexmap login --server https://nexmap.example.com
   ```

   On a terminal, `login` prompts for the token without echoing it. Otherwise it reads the first line of
   stdin, which suits scripts: `printf '%s\n' "$TOKEN" | nexmap login --server …`. There is
   deliberately no flag for the token itself, as it would end up in shell history and process listings.

`login` checks the token with the server and stores it per server in the user configuration file
(`config.json` in the user configuration directory, see below), readable only by you. `nexmap logout`
removes it again; `nexmap whoami` shows the user, server and product in effect and where each value
came from.

A token carries all the rights of the user who created it. For CI jobs and AI agents, create a dedicated
user with only the product role the job needs.

## Connecting a repository

In the repository, run:

```sh
nexmap init
```

This writes `.nexmap.json` at the repository root (or into `--dir`) with the server and product:

```json
{
  "server": "https://nexmap.example.com",
  "product": "<product id>"
}
```

The product comes from `--product` if given; if you are a member of only one product, that one is used;
otherwise `init` lists the products and asks. Without a terminal it fails and lists the choices instead,
so pass `--product` in scripts. `--force` overwrites an existing file, and `--agents` additionally sets
up AI agents (see [AI agents](ai-agents.md)).

Commit `.nexmap.json`: it never contains a token, and every command run anywhere in the repository finds
it by walking up from the current directory.

## Configuration

Each setting is taken from the first source that has it, in this order:

1. Command line flags
2. Environment variables
3. `.nexmap.json` in the current directory or the nearest parent
4. The user configuration file

| Setting | Flag | Environment | `.nexmap.json` | User configuration |
|---|---|---|---|---|
| Server URL | `--server` | `NEXMAP_SERVER` | `server` | — |
| Product ID | `--product` | `NEXMAP_PRODUCT` | `product` | — |
| Output format | `-o`, `--output` | `NEXMAP_OUTPUT` | — | `output` |
| Token | — | `NEXMAP_TOKEN` | — | stored per server by `login` |
| Plain HTTP | `--allow-http` | `NEXMAP_ALLOW_HTTP` | — | — |

`NEXMAP_TOKEN` takes precedence over a stored token; `login` warns when it is set. Boolean variables
accept `1`, `true`, `yes` or `on`.

The user configuration directory is `nexmap` inside the operating system's configuration directory:
`$XDG_CONFIG_HOME/nexmap` (usually `~/.config/nexmap`) on Linux, `~/Library/Application Support/nexmap`
on macOS, `%AppData%\nexmap` on Windows. Set `NEXMAP_CONFIG_DIR` to use another directory.

### Plain HTTP

The CLI refuses `http://` servers unless they run on the local machine (`localhost`, `*.localhost` or a
loopback address), so a token never crosses the network unencrypted. `--allow-http` (or
`NEXMAP_ALLOW_HTTP=1`) lifts this deliberately, e.g. on a trusted internal network.

## Commands

| Command | Purpose |
|---|---|
| `login`, `logout`, `whoami` | Manage the stored token, show what is in effect |
| `init` | Write `.nexmap.json`; with `--agents`, set up AI agents |
| `version` | Client version and, if a server is configured, the server version |
| `context` | Product briefing: structure, constraints, work in progress, defects, testing status |
| `req`, `task` | Requirements and tasks: `list`, `show`, `status`, `create` |
| `constraint` | `list`, `show` |
| `component`, `capability` | Product structure |
| `defect` | `list`, `show`, `status`, `create` |
| `test` | Test cases: `list`, `show`, `sync` |
| `campaign` | `status` of the active testing campaign |
| `comment` | `add` a comment to any commentable item |
| `utest` | Record test results from CI, see [CI integration](ci-integration.md) |
| `api` | Raw REST call, see [Scripting](scripting.md#raw-api-calls) |
| `agent-guide`, `mcp` | For AI agents, see [AI agents](ai-agents.md) |

`nexmap help <command>` (e.g. `nexmap help req show`) lists a command's arguments and flags.

Requirements, tasks and constraints are referenced by their identifier (`FR-12`, `MT-3`, `AC-42`),
defects and test cases by their short ID (the first 8 characters of the ID, shown in lists), test cases
also by their test identifier, and capabilities by name.
