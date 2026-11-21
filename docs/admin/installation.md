# Installation

## Which version to deploy

Deploy the branch `stable` or a release tag, never `main`:

| Ref | Holds | Use it for |
|---|---|---|
| `stable` | The newest release; it moves only when a release is made | Deployments that update with each release, such as Coolify watching the branch |
| `v0.9.1`, … | One release, unchanged | Deployments pinned to a version, updated by hand |
| `main` | Development work between releases, reporting `v0.9.1+dev` | Development only |

Pre-releases (`v1.0.0-rc.1`) and fix releases for older versions are tagged, but do not move `stable`;
deploy them by their tag. Updating to a new release runs its database migrations, which cannot be
undone, so [back up](backup.md#upgrades) before a release reaches your instance.

## Container image

The `Dockerfile`'s default stage is the server image: `wiekkod` alone on a distroless base, running as a
non-root user, listening on port 8080 on all interfaces, with a `HEALTHCHECK` (`wiekkod healthcheck`).
It keeps the SQLite database and attachments in `/data`, so mount persistent storage there. Migrations
run automatically when the server starts.

When building with podman, pass `--format docker`: the default OCI image format drops the `HEALTHCHECK`.

## Coolify

Create the application with the **Dockerfile** build pack and:

- set the branch to `stable` (see [which version to deploy](#which-version-to-deploy)). With
  automatic deployment on, every release then updates the instance, and nothing else does;
- add persistent storage (a volume) with destination path `/data`;
- set the environment variables, at least `WIEKKO_ENCRYPTION_KEY` (see
  [generating a key](security.md#the-encryption-key)), and `WIEKKO_ADMIN_USER` / `WIEKKO_ADMIN_PASSWORD`
  for the first start; `WIEKKO_PUBLIC_URL` with the application's domain; for PostgreSQL, `WIEKKO_DB_DRIVER=postgres` and `WIEKKO_DB_DSN`. All variables are
  listed in the [configuration reference](configuration.md);
- nothing for the version: it comes from the source tree (`internal/buildinfo/VERSION`). A release
  reports its version (`v0.9.1`), a build between releases the release it is based on and, when
  Coolify passes `SOURCE_COMMIT` to the build, the commit (`v0.9.1+dev.<commit>`). Do not set a
  `VERSION` variable: Coolify passes it to the build, where it overrides the version from the source.

Ports (8080) and the health check come from the image. Leave Coolify's own health check (in the
application's **Healthcheck** settings) turned off: Coolify runs it through `/bin/sh` and, for the HTTP
type, `curl` or `wget` inside the container, and the distroless image has none of them, so it fails
with `stat /bin/sh: no such file or directory`. The image's `HEALTHCHECK` runs `wiekkod healthcheck`
directly and needs no shell. Coolify's proxy terminates TLS; see
[Security](security.md#tls-and-the-reverse-proxy) for what the server expects from it.

## After the first start

Sign in as the administrator created from `WIEKKO_ADMIN_USER` / `WIEKKO_ADMIN_PASSWORD` (or with the
generated password from the log, see [the first administrator](security.md#the-first-administrator)).
Then change the administrator's email address on the profile page, and review the instance settings under
`/admin/settings`: self-registration, moderation, product creation, enforced 2FA and password rules.

Wiekko makes no backups itself: set them up before relying on the instance, see
[Backup and restore](backup.md).
