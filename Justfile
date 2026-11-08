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
    find . -name '*_templ.go' -delete
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

# Run the tests with the PostgreSQL runs included: start a throwaway server in
# a container (data on tmpfs, durability off), point the tests at it, remove
# it afterwards. Extra arguments go to `go test` (default ./...).
test-pg *args="./...":
    #!/usr/bin/env bash
    set -euo pipefail
    # Git Bash on Windows would rewrite container paths like /var/lib/...
    export MSYS_NO_PATHCONV=1
    engine={{ CONTAINER }}
    id=$($engine run -d --rm -e POSTGRES_PASSWORD=nexmap -p 127.0.0.1::5432 \
        --tmpfs /var/lib/postgresql {{ PG_IMAGE }} \
        -c fsync=off -c synchronous_commit=off -c full_page_writes=off -c max_connections=500)
    trap '$engine rm -f "$id" >/dev/null' EXIT
    # The image's init phase runs a temporary server on the Unix socket only;
    # the real one is up once TCP answers.
    for i in $(seq 1 60); do
        $engine exec "$id" pg_isready -q -h 127.0.0.1 -U postgres && break
        [ "$i" = 60 ] && { echo "postgres did not come up" >&2; $engine logs "$id" >&2; exit 1; }
        sleep 1
    done
    port=$($engine port "$id" 5432/tcp | head -n1 | sed 's/.*://')
    export NEXMAP_TEST_POSTGRES_DSN="postgres://postgres:nexmap@127.0.0.1:${port}/postgres?sslmode=disable"
    export NEXMAP_TEST_POSTGRES=required
    go test {{ args }}

# Build release binaries in a container with the pinned Go toolchain, templ and
# Tailwind; nothing but a container engine is needed on the host. Binaries land
# in build/<os>-<arch>/.
# (The binaries are copied out of a stopped container rather than with
# `build --output`, which podman's remote client on Windows and macOS lacks.)
container-build os="linux" arch="amd64":
    {{ CONTAINER }} build -f Dockerfile --target export -t nexmap-build:{{ os }}-{{ arch }} \
        --build-arg GOOS={{ os }} --build-arg GOARCH={{ arch }} \
        --build-arg VERSION=`git describe --tags --always --dirty 2>/dev/null || echo dev` .
    mkdir -p build/{{ os }}-{{ arch }}
    id=`{{ CONTAINER }} create nexmap-build:{{ os }}-{{ arch }} /none` && \
        {{ CONTAINER }} cp "$id:/out/." build/{{ os }}-{{ arch }}/; rc=$?; \
        {{ CONTAINER }} rm "$id" >/dev/null; exit $rc
