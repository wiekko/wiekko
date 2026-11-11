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
the build only the working tree without `.git`, so there the build clones the repository over HTTPS
(`DOCS_GIT_URL`, by default `https://forge.my-universe.com/daemotron/nexmap.git`). The repository is
private, so this needs a read-only access token, `DOCS_GIT_TOKEN`: preferably a BuildKit secret, which
the build reads from `/run/secrets/DOCS_GIT_TOKEN`; otherwise a build argument. A build argument stays
in the build stage, which is not part of the published image. Without `.git` and without a token, the
build publishes `/latest/` only and says so; with a token that fails, the build fails.

### Deployment on Coolify

The site is a second Coolify application next to the server, built from the same repository:

1. Create an application from the repository, in the same way as the server's, on branch `main`.
2. Choose the **Dockerfile** build pack, with base directory `/` and Dockerfile location
   `/docs-site/Dockerfile`. The base directory has to be the repository root: the build needs `docs/`,
   not only `docs-site/`.
3. Set the exposed port to `8080` and the domain to `https://docs.nexmap.net`. Point the DNS record of
   `docs.nexmap.net` at the Coolify server.
4. Enable automatic deployment. The repository is not on GitHub, so add a webhook in Forgejo
   (**Settings → Webhooks**, type Gitea or Forgejo) with the URL and secret from the application's webhook
   settings in Coolify, triggered by push events on `main`.
5. If your Coolify version offers watch paths, set them to `docs/**` and `docs-site/**`, so changes to
   the server code do not rebuild the site.
6. Give the build read access to the repository for the release tags:
   1. In Forgejo, create a user for it (e.g. `nexmap-docs`) and add it to the repository as a
      collaborator with **Read** access. Signed in as that user, create an access token under
      **Settings → Applications** with only the `read:repository` scope. A dedicated user keeps the
      token from reaching any other repository.
   2. In the Coolify application, add the environment variable `DOCS_GIT_TOKEN` with the token, available
      during build, and lock it so the value stays hidden.
   3. Set the application's build secrets to **Docker BuildKit secrets**, so the token reaches the build
      as a secret rather than a build argument.

Every push to `main` then rebuilds and deploys the site. In the debug log (**Show Debug Logs**), the
build prints `docs-site: release tags: …` and ends with a line such as
`docs-site: built latest v0.2.0 v0.1.0 (stable: v0.2.0)`. Check it after setting up and after each
release:

- **Releases need a forced rebuild.** A release tag usually points at a commit that is already
  deployed, and Coolify skips the build for a commit it has built before. After tagging, redeploy the
  site with the option to force a rebuild without cache.
- **`no .git and no DOCS_GIT_TOKEN`** means the token did not reach the build: check the variable and
  that it is available during build.
- **`cannot clone`** means the token was refused: check that it is valid, has the `read:repository`
  scope, and that its user can read the repository.
