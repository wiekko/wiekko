# Reproducible build of wiekkod and wiekko. Everything the build needs (Go,
# templ, Tailwind, just) is pinned here, so the host only needs podman or
# docker.
#
# Stages:
#   runtime (default, last): the wiekkod server image, as deployed by Coolify's
#            Dockerfile build pack. Data lives in /data (mount a volume there).
#   export:  just the binaries; `just container-build [os] [arch]` copies them to
#            build/<os>-<arch>/.
#
# To update Go or the runtime base, change both tag and digest (and keep the
# toolchain line in go.mod in step with Go). To update Tailwind, take the
# digests from the release's assets on GitHub.

ARG GO_IMAGE=docker.io/library/golang:1.26.11-trixie@sha256:fa340cb59ebeb1d53ac1cccfc334201d938fe5f3458003cccb767096d8e032b8
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

FROM ${GO_IMAGE} AS build

ARG TAILWIND_VERSION=v4.3.5
ARG TAILWIND_SHA256_X64=e47695cee3f0d90623a91f3939b08bdee610eb5ad99340adff9d4145b284d6cc
ARG TAILWIND_SHA256_ARM64=ffcd0d21dbd7c605dc12743ff0c0b47debc963c86b399674084c1c8f2c96ecad

RUN apt-get update \
    && apt-get install -y --no-install-recommends just \
    && rm -rf /var/lib/apt/lists/*

# Tailwind's standalone CLI runs on the build machine, so it follows the build
# architecture, not GOARCH.
RUN set -eu; \
    case "$(uname -m)" in \
        x86_64) asset=tailwindcss-linux-x64; sum="$TAILWIND_SHA256_X64" ;; \
        aarch64) asset=tailwindcss-linux-arm64; sum="$TAILWIND_SHA256_ARM64" ;; \
        *) echo "unsupported build architecture: $(uname -m)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/tailwindcss \
        "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${asset}"; \
    echo "${sum}  /usr/local/bin/tailwindcss" | sha256sum -c -; \
    chmod +x /usr/local/bin/tailwindcss

# The image's Go is the toolchain; never download another one.
ENV GOTOOLCHAIN=local

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# GOOS/GOARCH default to the platform the image is built for, so the runtime
# image matches the server (amd64 or arm64).
ARG TARGETOS
ARG TARGETARCH
ARG GOOS
ARG GOARCH
# The version the binaries report comes from internal/buildinfo/VERSION
# ("v0.9.0" on a release commit, "v0.9.0+dev" after it). Builds between
# releases append the commit: Go reads it from .git when the context has it,
# else it comes from SOURCE_COMMIT (Coolify can pass it). VERSION, if passed
# as a build argument, overrides all of this.
ARG VERSION
ARG SOURCE_COMMIT

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    goos="${GOOS:-${TARGETOS:-linux}}"; \
    goarch="${GOARCH:-${TARGETARCH:-amd64}}"; \
    git config --global --add safe.directory /src; \
    ldflags="-s -w"; \
    if [ -n "${VERSION:-}" ]; then \
        ldflags="$ldflags -X wiekko.net/internal/buildinfo.Version=${VERSION}"; \
    elif [ -n "${SOURCE_COMMIT:-}" ]; then \
        ldflags="$ldflags -X wiekko.net/internal/buildinfo.Revision=${SOURCE_COMMIT}"; \
    fi; \
    just generate css; \
    go vet ./...; \
    mkdir -p /out /data/uploads; \
    for cmd in wiekkod wiekko; do \
        CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
            -ldflags "$ldflags" -o /out/ ./cmd/"$cmd"; \
    done

FROM scratch AS export
COPY --from=build /out/ /out/

FROM ${RUNTIME_IMAGE} AS runtime

COPY --from=build /out/wiekkod /usr/local/bin/wiekkod
# Owned by the image's nonroot user, so a fresh volume mounted here is
# writable (Docker copies ownership into an empty named volume).
COPY --from=build --chown=65532:65532 /data /data

# Listen on all interfaces: the reverse proxy (Coolify's Traefik) reaches the
# container over its network, never via localhost.
ENV WIEKKO_HOST=0.0.0.0 \
    WIEKKO_PORT=8080 \
    WIEKKO_DB_DSN=/data/wiekko.db \
    WIEKKO_UPLOAD_DIR=/data/uploads

USER 65532:65532
WORKDIR /data
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD ["/usr/local/bin/wiekkod", "healthcheck"]

ENTRYPOINT ["/usr/local/bin/wiekkod"]
