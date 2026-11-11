# Installation

## Container image

The `Dockerfile`'s default stage is the server image: `nexmapd` alone on a distroless base, running as a
non-root user, listening on port 8080 on all interfaces, with a `HEALTHCHECK` (`nexmapd healthcheck`).
It keeps the SQLite database and attachments in `/data`, so mount persistent storage there. Migrations
run automatically when the server starts.

When building with podman, pass `--format docker`: the default OCI image format drops the `HEALTHCHECK`.

## Coolify

Create the application with the **Dockerfile** build pack and:

- add persistent storage (a volume) with destination path `/data`;
- set the environment variables, at least `NEXMAP_ENCRYPTION_KEY` (see
  [generating a key](security.md#the-encryption-key)), and `NEXMAP_ADMIN_USER` / `NEXMAP_ADMIN_PASSWORD`
  for the first start; for PostgreSQL, `NEXMAP_DB_DRIVER=postgres` and `NEXMAP_DB_DSN`. All variables are
  listed in the [configuration reference](configuration.md);
- nothing for the version: the build reads it from the repository's git tags, as `go build` does
  locally (`v0.1.0` on a tagged commit, `v0.1.1-0.<date>-<commit>` after it). If the checkout has no
  tags (a shallow clone), it reports `v0.0.0-<date>-<commit>`; set the build variable `VERSION` to
  override it.

Ports (8080) and the health check come from the image. Coolify's proxy terminates TLS; see
[Security](security.md#tls-and-the-reverse-proxy) for what the server expects from it.

## After the first start

Sign in as the administrator created from `NEXMAP_ADMIN_USER` / `NEXMAP_ADMIN_PASSWORD` (or with the
generated password from the log, see [the first administrator](security.md#the-first-administrator)).
Then change the administrator's email address on the profile page, and review the instance settings under
`/admin/settings`: self-registration, moderation, product creation, enforced 2FA and password rules.
