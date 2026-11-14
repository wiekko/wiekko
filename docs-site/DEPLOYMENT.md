# Deploying docs.nexmap.net on Coolify

Internal runbook: how the documentation site is deployed, and how to set it up again on another Coolify
server. This file lives in `docs-site/`, outside the mdBook source, so it is never published. For how the
site is built and organised, see `CONTRIBUTING.md`, "Documentation site".

## Overview

The site is its own Coolify application, next to the Nexmap server, built from the same Forgejo
repository (`ssh://git@forge.my-universe.com:22222/daemotron/nexmap.git`, private) on branch `main`.

```
push to main ──webhook──▶ Coolify ──clone (SSH)──▶ working tree, no .git
                                   │
                                   └─ docker build -f docs-site/Dockerfile
                                        ├─ mdBook builds /latest/ from the working tree
                                        ├─ build.sh clones the repo over HTTPS with DOCS_GIT_TOKEN
                                        │   and builds every release tag v* as /<tag>/, plus /stable/
                                        └─ nginx (unprivileged) serves the result on port 8080
```

The token clone is needed because Coolify builds from a copy of the working tree **without `.git`**, so
the release tags are not in the build context. Whether Coolify clones shallow does not matter for that.

## What you need

- A Coolify v4 server with Docker BuildKit (Coolify logs "Docker … with BuildKit and Buildx detected").
- A DNS record for `docs.nexmap.net` pointing at that server.
- Access to the Forgejo repository as an administrator of it (deploy key, webhook, collaborator).

## Setting up

### 1. Read-only token for the release tags

1. In Forgejo, create a user for the build, e.g. `nexmap-docs`. A dedicated user keeps the token from
   reaching any other repository.
2. Add it to the repository as a collaborator with **Read** access.
3. Signed in as that user, create an access token under **Settings → Applications** with only the
   `read:repository` scope. Keep it at hand for step 3 below; Forgejo shows it only once.

### 2. Coolify application

1. Create an application from the private repository, in the same way as the Nexmap server's: Coolify
   clones over SSH with a deploy key, which has to be registered in Forgejo (repository **Settings →
   Deploy keys**, read-only). On a new server, that is a new key.
2. Branch `main`.
3. Build pack **Dockerfile**, base directory `/`, Dockerfile location `/docs-site/Dockerfile`. The base
   directory has to be the repository root: the build needs `docs/`, not only `docs-site/`.
4. Exposed port `8080`, domain `https://docs.nexmap.net`.
5. Health check: path `/versions.json`, port `8080`. Every build writes that file, and nginx serves it
   uncached.
6. If your Coolify version offers watch paths, set them to `docs/**` and `docs-site/**`, so changes to the
   server code do not rebuild the site.

### 3. Token in Coolify

1. Add the environment variable `DOCS_GIT_TOKEN` with the token from step 1, **available during build**,
   and **lock** it so its value stays hidden in the UI and the logs.
2. Set the application's build secrets to **Docker BuildKit secrets**.

The Dockerfile only accepts the token as a BuildKit secret (mounted at `/run/secrets/DOCS_GIT_TOKEN`),
not as a build argument, which would end up in the image's build history. Without the BuildKit secrets
setting, Coolify passes it as a build argument, the build does not see it, and it publishes `/latest/`
only (see the log messages below).

`DOCS_GIT_URL` needs no variable: `build.sh` defaults to
`https://forge.my-universe.com/daemotron/nexmap.git`. Only change that default if the repository moves.
Do not rely on setting `DOCS_GIT_URL` in Coolify: with BuildKit secrets on, Coolify may pass it as a
secret, which the Dockerfile does not mount.

### 4. Automatic deployment

The repository is not on GitHub, so Coolify needs a webhook: in Forgejo, **Settings → Webhooks → Add
webhook**, type Gitea (or Forgejo), with the URL and secret from the application's webhook settings in
Coolify, triggered by push events, branch filter `main`.

### 5. First deployment and check

Deploy, open **Show Debug Logs** and look for the build step that runs `build.sh`:

```
docs-site: no usable .git; fetching release tags from https://forge.my-universe.com/daemotron/nexmap.git
docs-site: release tags: v0.2.0 v0.1.0          (or "none" before the first release)
docs-site: built latest v0.2.0 v0.1.0 (stable: v0.2.0)
```

Then check the site:

```sh
curl -fsS https://docs.nexmap.net/versions.json   # {"latest": "latest", "stable": ..., "versions": [...]}
curl -fsSI https://docs.nexmap.net/latest/        # 200
```

The root redirects to `/stable/`, or to `/latest/` while there is no release.

## Day to day

- **Push to `main`** rebuilds and deploys the site through the webhook.
- **Releases need a forced rebuild.** A release tag usually points at a commit that is already deployed,
  and Coolify skips the build for a commit it has built before ("Image not found … Building new image"
  only appears for new commits). After tagging, redeploy with the option to force a rebuild without cache.
- **Token rotation:** create a new token for the build user, replace `DOCS_GIT_TOKEN` in Coolify, delete
  the old token in Forgejo, then redeploy and check the log.

## Moving to another server

1. Point the DNS record of `docs.nexmap.net` at the new server.
2. Set up the application there as in "Setting up", step 2, with a new deploy key in Forgejo.
3. Add `DOCS_GIT_TOKEN` again (the existing build user and token can stay; or rotate it now) and turn on
   BuildKit secrets.
4. Replace the Forgejo webhook's URL and secret with the new application's, or add a new webhook and
   delete the old one once the old server is gone.
5. Deploy and check as in step 5. Then remove the old application, its deploy key and its webhook.

Nothing on the site is state: every deployment rebuilds it completely from the repository, so there is
nothing to migrate.

## Troubleshooting

Messages from `build.sh` in the debug log:

| Message | Meaning |
|---|---|
| `release tags: none` | The clone worked; the repository has no `v*` tags. |
| `no release tags with documentation found` | No tag contains `docs-site/book.toml`; tags from before the site existed are skipped. |
| `warning: no .git and no DOCS_GIT_TOKEN; building /latest/ only` | The token did not reach the build: check that the variable exists, is available during build, and that build secrets are set to Docker BuildKit secrets. |
| `error: cannot clone …` | The token was refused or the repository is unreachable: check that the token is valid, has `read:repository`, and that its user can read the repository. The build fails on purpose rather than publishing without versions. |
| `warning: git cannot read …/.git` | A `.git` is in the context but unusable (local builds only). |
| `warning: shallow clone; release tags may be missing` | A shallow `.git` is in the context (local builds only). |

Other things seen during the first setup:

- **No build after a push:** check the webhook's recent deliveries in Forgejo, and the watch paths.
- **A new commit's site looks unchanged:** the build may have been skipped because Coolify already had an
  image for that commit; force a rebuild.
- **`SecretsUsedInArgOrEnv` warning:** an `ARG` named like a secret is back in the Dockerfile. Keep the
  token a secret only.
- Coolify adds every build-time variable as a secret mount to *every* `RUN` step it rewrites. That is
  Coolify's doing; nothing in those steps reads the token, and secrets do not end up in the image.
