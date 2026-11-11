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
- set the environment variables, at least `NEXMAP_ENCRYPTION_KEY`, and `NEXMAP_ADMIN_USER` /
  `NEXMAP_ADMIN_PASSWORD` for the first start; for PostgreSQL, `NEXMAP_DB_DRIVER=postgres` and
  `NEXMAP_DB_DSN`;
- nothing for the version: the build reads it from the repository's git tags, as `go build` does
  locally (`v0.1.0` on a tagged commit, `v0.1.1-0.<date>-<commit>` after it). If the checkout has no
  tags (a shallow clone), it reports `v0.0.0-<date>-<commit>`; set the build variable `VERSION` to
  override it.

Ports (8080) and the health check come from the image.
