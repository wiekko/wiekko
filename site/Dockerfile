# Download site (nexmap.net): the landing page, the CLI install scripts and the
# CLI release archives under /releases/, served by nginx on port 8080. Build
# from the repository root:
#
#   docker build -f site/Dockerfile -t nexmap-site .   (or: just site-container)
#
# Stages:
#   dist:    builds and packages the CLI for every platform (site/dist.sh)
#   export:  just the archives; `just dist` copies them to build/dist/
#   runtime: (default, last) nginx with the site and this build's release
#
# Each build packages only the release in the source tree. Releases collect on
# the volume mounted at /data: at start, the entrypoint hook copies this
# image's release to /data/releases/ and points latest.json at it, so pinned
# older versions stay downloadable. See site/DEPLOYMENT.md.
#
# GO_IMAGE is the root Dockerfile's, RUNTIME_IMAGE docs-site/Dockerfile's: keep
# tag and digest in step with them.

ARG GO_IMAGE=docker.io/library/golang:1.26.11-trixie@sha256:fa340cb59ebeb1d53ac1cccfc334201d938fe5f3458003cccb767096d8e032b8
ARG RUNTIME_IMAGE=docker.io/nginxinc/nginx-unprivileged:1.31.7-alpine3.24@sha256:b5047b9b76845eac124358373c6852377c52e4b05834a47647e955dd91cb8f1e

FROM ${GO_IMAGE} AS dist

RUN apt-get update \
    && apt-get install -y --no-install-recommends zip \
    && rm -rf /var/lib/apt/lists/*

# The image's Go is the toolchain; never download another one.
ENV GOTOOLCHAIN=local

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Overrides the version from internal/buildinfo/VERSION, for test builds
# (`just dist v9.9.9`). Deployments leave it empty.
ARG VERSION

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    sh site/dist.sh /dist \
    && mkdir -p /data/releases

FROM scratch AS export
COPY --from=dist /dist/ /dist/

FROM ${RUNTIME_IMAGE} AS runtime

COPY site/nginx.conf /etc/nginx/conf.d/default.conf
COPY site/www/ /usr/share/nginx/html/
COPY --from=dist /dist/ /opt/nexmap/releases/
# Owned by the image's user (101), so a fresh named volume mounted at /data is
# writable (Docker copies ownership into an empty named volume).
COPY --from=dist --chown=101:101 /data /data
# The stock entrypoint runs executable scripts from /docker-entrypoint.d/
# before nginx starts.
COPY --chmod=0755 site/publish-releases.sh /docker-entrypoint.d/40-publish-releases.sh

EXPOSE 8080
