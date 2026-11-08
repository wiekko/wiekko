# Reproducible build of nexmapd and nexmap. Everything the build needs (Go,
# templ, Tailwind, just) is pinned here, so the host only needs podman or
# docker. Use `just container-build [os] [arch]`, which exports the binaries to
# build/<os>-<arch>/.
#
# To update Go, change both the tag and the digest (and keep the toolchain line
# in go.mod in step). To update Tailwind, take the digests from the release's
# assets on GitHub.

ARG GO_IMAGE=docker.io/library/golang:1.26.10-trixie@sha256:78773c59d6d4ecedc0845b1a8e5379e7bf0e0be62f4ea796e689bbc97dab3ceb

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

ARG GOOS=linux
ARG GOARCH=amd64
ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    just generate css; \
    go vet ./...; \
    mkdir -p /out; \
    for cmd in nexmapd nexmap; do \
        CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath \
            -ldflags "-s -w -X nexmap.net/internal/buildinfo.Version=${VERSION}" \
            -o /out/ ./cmd/"$cmd"; \
    done

FROM scratch AS export
COPY --from=build /out/ /out/
