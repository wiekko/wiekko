# Setting up the CLI

`nexmap` is the command line client for a Nexmap server. It talks to the server only through the
[REST API](../api/rest.md), so it needs nothing but the server's URL and a personal access token.

## Installing

`nexmap` is a single binary without runtime dependencies, published for Linux, macOS and Windows on
amd64 and arm64 at [nexmap.net](https://nexmap.net/).

On Linux and macOS:

```sh
curl -fsSL https://nexmap.net/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://nexmap.net/install.ps1 | iex
```

The scripts download the latest release for your platform, verify it against the release's checksums
and install it:

- `install.sh` into `~/.local/bin`, or `/usr/local/bin` when run as root. It tells you when that directory
  is not on your `PATH`.
- `install.ps1` into `%LOCALAPPDATA%\Programs\nexmap`, which it adds to your user `PATH`.

They read these environment variables:

| Variable | Meaning |
|---|---|
| `NEXMAP_VERSION` | Version to install, e.g. `v1.0.0`; default: the latest release |
| `NEXMAP_INSTALL_DIR` | Directory to install into |

To pin a version on Linux or macOS: `curl -fsSL https://nexmap.net/install.sh | NEXMAP_VERSION=v1.0.0 sh`.
In PowerShell, set `$env:NEXMAP_VERSION = 'v1.0.0'` first.

To install by hand, download the archive for your platform from
[nexmap.net/releases](https://nexmap.net/releases/), check it against the release's `SHA256SUMS`
(on Linux: `sha256sum -c SHA256SUMS --ignore-missing`), and put the binary on your `PATH`.

## Updating

`nexmap update` replaces the running binary with the latest release; `nexmap update v1.0.0` installs a
given version, which may be an older one. `nexmap update --check` only reports whether a newer release
exists. The download is verified against the release's checksums before anything is replaced.

`update` refuses to install a release that needs a newer API than the configured server offers (see
`nexmap version`), since that client could not talk to it; `--force` installs it anyway, and also
reinstalls the current version. If the binary sits in a directory you cannot write to, run the update
with the rights to write there, e.g. `sudo nexmap update`.

Once a day, `nexmap` looks for a newer release in the background and prints a one-line notice on stderr
if there is one. The check never delays a command by more than two seconds and stays silent on errors.
It is off where an extra line could get in the way: when stderr is not a terminal (scripts, pipes), when
`CI` is set, for `nexmap mcp`, and for development builds. `NEXMAP_NO_UPDATE_CHECK=1` turns it off
altogether.

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

When you create a token, narrow it to what it is for: pick a preset, or set the scopes, a role cap and
the products yourself. *CI ingestion* suits `nexmap utest`, *Agent, spec author* an AI agent that maintains the
specification, and *Agent, read-only* suits an AI agent that only
reads. A token never grants more than its user holds; `nexmap whoami` shows its restriction. See
[Restricted tokens](../api/rest.md#restricted-tokens) for what each scope covers.

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

`NEXMAP_TOKEN` takes precedence over a stored token; `login` warns when it is set. Unlike a stored
token, which is kept per server, it is sent to whichever server is configured. The CLI warns when that
server comes from `.nexmap.json`; set `NEXMAP_SERVER` next to the token to pin it. Boolean variables
accept `1`, `true`, `yes` or `on`.

The user configuration directory is `nexmap` inside the operating system's configuration directory:
`$XDG_CONFIG_HOME/nexmap` (usually `~/.config/nexmap`) on Linux, `~/Library/Application Support/nexmap`
on macOS, `%AppData%\nexmap` on Windows. Set `NEXMAP_CONFIG_DIR` to use another directory.

Two more variables concern updates (see [Updating](#updating)): `NEXMAP_NO_UPDATE_CHECK` turns off the
daily check for a newer release, and `NEXMAP_RELEASES_URL` points `update` and the check at another
location than `https://nexmap.net/releases`, e.g. a mirror.

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
| `update` | Install the latest or a given release, see [Updating](#updating) |
| `context` | Product briefing: structure, constraints, work in progress, defects, testing status |
| `req`, `task` | Requirements and tasks: `list`, `show`, `status`, `create`, `update` (e.g. `--parent FR-3`) |
| `constraint` | `list`, `show`, `create`, `update` (e.g. `--compliance met`) |
| `doc` | Documents: `list`, `show`, `create`, `update`, `delete`, `link`/`unlink`, `revisions`, `diff`, `restore`, `get` |
| `component` | Product structure: `list`, `create`, `update`, `delete` |
| `capability` | `list`, `show`, `create`, `update`, `delete`, and `link`/`unlink` requirements and constraints |
| `defect` | `list`, `show`, `status`, `create` |
| `test` | Test cases: `list`, `show`, `sync`, and `link`/`unlink` them to a requirement, capability or component |
| `campaign` | `status` of the active testing campaign |
| `comment` | `add` a comment to any commentable item |
| `utest` | Record test results from CI, see [CI integration](ci-integration.md) |
| `export` | The product's specification as Markdown, see [below](#exporting-the-specification) |
| `api` | Raw REST call, see [Scripting](scripting.md#raw-api-calls) |
| `agent-guide`, `mcp` | For AI agents, see [AI agents](ai-agents.md) |

`nexmap help <command>` (e.g. `nexmap help req show`) lists a command's arguments and flags.

Requirements, tasks, constraints, documents and defects are referenced by their identifier (`FR-12`, `MT-3`,
`AC-42`, `DOC-3`, `DF-17`), test cases by their short ID (the first 8 characters of the ID, shown in lists) or
their test identifier, and capabilities by name.

## Changing the structure

Product owners can build the product structure from the command line, which helps when an AI agent
helps develop the product:

```sh
nexmap component create --name "Sync engine" --body "Keeps devices in step."
nexmap capability create --name "Offline queue" --component "Sync engine"
nexmap capability link Offline queue FR-12 AC-3
nexmap constraint update AC-3 --compliance violated
nexmap test link TestQueue_Replay --req FR-12 --capability "Offline queue"
```

Names with spaces need no quotes where they are the command's only argument. For `capability link`,
everything before the first identifier names the capability; a capability whose name ends in something
like `Step-1` has to be given by ID there. `update` commands change only the options given, and an
empty value clears a description or, for `capability update --component ""`, moves a capability to
product level. `component delete` and `capability delete` need `--yes`.

## Documents

Documents hold the protocols, schemas and other texts that define the product. Nexmap is where they are
written; a repository gets copies through the export below. A body can come from a file and goes back
out unchanged:

```sh
nexmap doc create --title "Sighting schema" --format json --state current --body-file schema.json
nexmap doc update DOC-3 --body-file schema.json
nexmap doc diff DOC-3
nexmap doc link DOC-3 FR-12 SC-2 TestSchema_Validates
nexmap doc get DOC-3 --file api/schema.json
```

Every change to a document's title, content or format is a new revision; `doc revisions` lists them,
`doc diff` compares two (`--from`, `--to`), `doc restore DOC-3 2` makes an earlier content current again
and `doc get --revision 2` prints it. Setting a body from a file is an ordinary update: Nexmap does not
watch the file. Only product owners change documents; `doc delete` needs `--yes`.

## Exporting the specification

`nexmap export` writes the product's structure (components and capabilities), constraints and
requirements as one Markdown document: grouped by category, each item with its identifier, its
description, definition of done and linked capabilities, and refinements nested below the item they
refine. `--tasks` adds the tasks, `--file docs/spec.md` writes the document to a file instead of stdout.

The specification also lists the current documents, and with `--drafts` the drafts. `--documents DIR`
writes each listed document's content to its own file there, byte for byte, named by its identifier and
format (`DOC-3.json`), so tools in the repository, such as code generators and schema validators, can use
it directly:

```sh
nexmap export --file docs/spec.md --documents docs/definitions
```

The order is fixed (categories as configured, items by number, structure by name) and the document
contains no date, status or priority, so exporting an unchanged product gives the same file, and a
committed export shows real changes in a diff. With `-o json` and `--file`, the output names the file and
the document's SHA-256. AI agents get the same document from the MCP tool `export_specification`. Anyone who can read the product can export it; a
restricted token needs `read` on both `structure` and `backlog`, and `documents` to include the
documents.

There is no import. To turn an existing document into requirements, let an AI agent read it and create
the items with `nexmap req create` or the REST API. Since the export carries identifiers, the agent can
change existing items by identifier instead of creating duplicates.
