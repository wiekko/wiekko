# Contributing

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

| Binary    | Source        | Purpose                                                      |
|-----------|---------------|--------------------------------------------------------------|
| `nexmapd` | `cmd/nexmapd` | The server: web UI and REST API                              |
| `nexmap`  | `cmd/nexmap`  | The command line client, talking to the server via REST only |

For release binaries, `just container-build [os] [arch]` builds in a container (podman, else docker;
override with `CONTAINER_ENGINE`) with Go, templ and Tailwind pinned in the `Dockerfile`, and
writes the binaries to `build/<os>-<arch>/`. The host needs only the container engine and `just`.

## Testing

Integration tests run on both SQLite and PostgreSQL to catch dialect differences. The PostgreSQL runs need
a server named by `NEXMAP_TEST_POSTGRES_DSN` (a `postgres://` URL whose role may create databases);
`just test-pg` starts one with podman or docker and removes it afterwards. Without a server those subtests
are skipped, not failed; set `NEXMAP_TEST_POSTGRES=required` where PostgreSQL coverage must not be
skipped silently, such as CI.

## Specification and documentation

`docs/spec/` holds the specification: what nexmap *shall* do, with requirement identifiers (FR-…, AC-…).
The rest of `docs/` describes how nexmap behaves and how to use it, for administrators, users and CLI
users; see [docs/index.md](docs/index.md).

Implementation plans live in `docs/tmp/` while their work is in progress. When a plan is done, move what
stays true (decisions, configuration, behaviour) into the permanent docs and delete the plan. Permanent
docs never link into `docs/tmp/`.

## Documentation site

The user, administrator, CLI and API documentation is published at `docs.nexmap.net`, built with
[mdBook](https://rust-lang.github.io/mdBook/). The pages stay in `docs/`; `docs-site/` holds only the site
tooling, so none of it is published.

- **What gets published:** only the pages listed in [docs/SUMMARY.md](docs/SUMMARY.md), which is also the
  site's navigation. Add new pages there. `docs/spec/`, `docs/decisions/` and `docs/tmp/` are left out
  on purpose. The repository is private, so published pages must not link to it: mention such files by
  path (`docs/spec/`, `CONTRIBUTING.md`) instead.
- **Versions:** the site serves `main` as `/latest/` (with a banner saying it may describe unreleased
  features), every release tag `v*` as `/<tag>/`, and the newest release that is not a pre-release as
  `/stable/`, where the site's root redirects. A menu in the header switches between versions. Tags from
  before the site existed (without `docs-site/book.toml`) are skipped.
- **Preview:** `just docs` serves the working tree with live reload on port 3000 (needs `mdbook`).
  `just docs-build` builds all versions into `build/docs-site/`, and `just docs-container` builds and runs
  the deployed image on port 8080.

`docs-site/build.sh` builds the versions, `docs-site/Dockerfile` runs it (mdBook pinned by version and
checksum) and serves the result with nginx on port 8080.

The release tags come from git. Locally, the build uses the checkout's `.git`. Coolify, however, hands
the build only the working tree without `.git`, so there the build clones the repository over HTTPS.
The repository is private, so this needs a read-only access token, given to the container build as the
BuildKit secret `DOCS_GIT_TOKEN` (never as a build argument). Without `.git` and without a token, the
build publishes `/latest/` only and says so; with a token that fails, the build fails.

### Deployment

The site is deployed as its own Coolify application. [docs-site/DEPLOYMENT.md](docs-site/DEPLOYMENT.md)
describes the setup (Forgejo token, Coolify settings, webhook), what to check in the deployment log,
releases, and moving to another server.
