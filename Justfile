#!/usr/bin/env just --justfile

set dotenv-load

TAILWIND := if os_family() == "windows" { "tailwindcss.exe" } else { "tailwindcss" }

# The templ generator must match the templ runtime library pinned in go.mod;
# generated code from a different version does not compile against it.
TEMPL_VERSION := `go list -m -f '{{.Version}}' github.com/a-h/templ`
TEMPL := "go run github.com/a-h/templ/cmd/templ@" + TEMPL_VERSION

# Container engine for the container-* recipes: podman if installed, else
# docker. Override with CONTAINER_ENGINE.
CONTAINER := env("CONTAINER_ENGINE", `command -v podman >/dev/null 2>&1 && echo podman || echo docker`)

DIST := "web/dist"
STATIC := "web/static"

default:
    @just --list

all: clean generate copy-assets css build

build: generate css
    go vet ./...
    go build ./cmd/nexmapd
    go build ./cmd/nexmap

clean:
    go clean
    rm -f nexmap nexmap.exe nexmapd nexmapd.exe
    /usr/bin/find . -name '*_templ.go' -delete
    rm -rf {{ DIST }}

css: copy-assets
    {{ TAILWIND }} -i {{ STATIC }}/css/input.css -o {{ DIST }}/css/styles.css --minify

copy-assets:
    @mkdir -p {{ DIST }}/css {{ DIST }}/fonts {{ DIST }}/js {{ DIST }}/img
    cp {{ STATIC }}/fonts/* {{ DIST }}/fonts/
    cp {{ STATIC }}/js/* {{ DIST }}/js/
    cp {{ STATIC }}/img/* {{ DIST }}/img/

dev:
    {{ TEMPL }} generate --watch &
    {{ TAILWIND }} -i {{ STATIC }}/css/input.css -o {{ DIST }}/css/styles.css --watch &
    go run ./cmd/nexmapd

generate:
    {{ TEMPL }} generate

test:
    go test -v ./...

# PostgreSQL image for test-pg; override with NEXMAP_TEST_POSTGRES_IMAGE.
PG_IMAGE := env("NEXMAP_TEST_POSTGRES_IMAGE", "docker.io/library/postgres:18")

# Run the tests with the PostgreSQL runs included, on a throwaway server in a
# container (scripts/test-pg.sh). Extra arguments go to `go test` (default ./...).
[positional-arguments]
test-pg *args:
    ENGINE={{ CONTAINER }} PG_IMAGE={{ PG_IMAGE }} sh scripts/test-pg.sh "$@"

# From main or a release/* branch: checks and tests, commits VERSION as the
# release and tags it, then commits "<version>+dev" for the work after it.
# Pass `push` as second argument to also push both commits and the tag.
# See scripts/release.sh and CONTRIBUTING.md, "Releasing".
# Release a version, e.g. `just release v0.9.1` or `just release v0.9.1 push`
[positional-arguments]
release version push="":
    sh scripts/release.sh "$@"

# Build release binaries in a container with the pinned Go toolchain, templ and
# Tailwind; nothing but a container engine is needed on the host. Binaries land
# in build/<os>-<arch>/.
# (The binaries are copied out of a stopped container rather than with
# `build --output`, which podman's remote client on Windows and macOS lacks.)
container-build os="linux" arch="amd64":
    {{ CONTAINER }} build -f Dockerfile --target export -t nexmap-build:{{ os }}-{{ arch }} \
        --build-arg GOOS={{ os }} --build-arg GOARCH={{ arch }} .
    mkdir -p build/{{ os }}-{{ arch }}
    id=`{{ CONTAINER }} create nexmap-build:{{ os }}-{{ arch }} /none` && \
        {{ CONTAINER }} cp "$id:/out/." build/{{ os }}-{{ arch }}/; rc=$?; \
        {{ CONTAINER }} rm "$id" >/dev/null; exit $rc

# Needs mdbook on the PATH. Shows the working tree only, without other versions.
# Serve the documentation with live reload on http://localhost:3000
docs:
    mdbook serve docs-site

# Needs git and mdbook on the PATH; the result equals the docs container's.
# Build the documentation site with all versions into build/docs-site
docs-build:
    sh docs-site/build.sh

# Stop it with Ctrl+C.
# Build the documentation site image and serve it on http://localhost:8080
docs-container:
    {{ CONTAINER }} build -f docs-site/Dockerfile -t nexmap-docs .
    {{ CONTAINER }} run --rm -p 8080:8080 nexmap-docs
