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
  on purpose; published pages link to them, and to files outside `docs/`, with absolute URLs into the
  repository.
- **Versions:** the site serves `main` as `/latest/` (with a banner saying it may describe unreleased
  features), every release tag `v*` as `/<tag>/`, and the newest release that is not a pre-release as
  `/stable/`, where the site's root redirects. A menu in the header switches between versions. Tags from
  before the site existed (without `docs-site/book.toml`) are skipped.
- **Preview:** `just docs` serves the working tree with live reload on port 3000 (needs `mdbook`).
  `just docs-build` builds all versions into `build/docs-site/`, and `just docs-container` builds and runs
  the deployed image on port 8080.

`docs-site/build.sh` builds the versions, `docs-site/Dockerfile` runs it (mdBook pinned by version and
checksum) and serves the result with nginx on port 8080. The build reads the release tags from `.git`
in the build context, which `.dockerignore` keeps for this purpose.

### Deployment on Coolify

The site is a second Coolify application next to the server, built from the same repository:

1. Create an application from the repository, in the same way as the server's, on branch `main`.
2. Choose the **Dockerfile** build pack, with base directory `/` and Dockerfile location
   `/docs-site/Dockerfile`. The base directory has to be the repository root: the build needs `docs/`
   and `.git`, not only `docs-site/`.
3. Set the exposed port to `8080` and the domain to `https://docs.nexmap.net`. Point the DNS record of
   `docs.nexmap.net` at the Coolify server.
4. Enable automatic deployment. The repository is not on GitHub, so add a webhook in Forgejo
   (**Settings → Webhooks**, type Gitea or Forgejo) with the URL and secret from the application's webhook
   settings in Coolify, triggered by push events on `main`.
5. If your Coolify version offers watch paths, set them to `docs/**` and `docs-site/**`, so changes to
   the server code do not rebuild the site.

Every push to `main` then rebuilds and deploys the site. The deployment log ends with a line such as
`docs-site: built latest v0.2.0 v0.1.0 (stable: v0.2.0)`; check it after the first deployment and after
each release:

- **A new release tag alone may not trigger a deployment**, since Coolify deploys on pushes to the
  branch. The release then appears with the next push to `main`, or redeploy the site in Coolify after
  tagging.
- **If the log warns about a shallow clone or lists no releases**, Coolify cloned without the tags and
  the site only shows `/latest/`.
