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

# From main or a release/* branch: checks and tests, commits VERSION as the
# release and tags it, then commits "<version>+dev" for the work after it.
# Pass `push` as second argument to also push both commits and the tag.
# See CONTRIBUTING.md, "Releasing".
# Release a version, e.g. `just release v0.9.1` or `just release v0.9.1 push`
release version push="":
    #!/usr/bin/env bash
    set -euo pipefail
    file=internal/buildinfo/VERSION
    version='{{ version }}'
    semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
    fail() { echo "release: $*" >&2; exit 1; }

    case '{{ push }}' in ""|push) ;; *) fail "second argument must be 'push' or nothing" ;; esac
    [[ $version =~ $semver ]] || fail "$version is not a version like v1.2.3 or v1.2.3-rc.1"
    new=("${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}")
    new_pre=${BASH_REMATCH[4]}

    branch=$(git symbolic-ref --quiet --short HEAD) || fail "not on a branch"
    [[ $branch == main || $branch == release/* ]] || fail "releases are made from main or release/*, not $branch"
    [ -z "$(git status --porcelain)" ] || fail "the working tree has uncommitted changes"
    ! git rev-parse -q --verify "refs/tags/$version" >/dev/null || fail "tag $version exists already"

    # The new version must be higher than the release the tree is based on.
    base=$(tr -d '[:space:]' < "$file")
    base=${base%+dev}
    [[ $base =~ $semver ]] || fail "$file holds '$base', not a version"
    old=("${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}")
    old_pre=${BASH_REMATCH[4]}
    higher=
    # Same core version: only a pre-release may move on (rc.1 -> rc.2 compares
    # as text, so rc.10 needs care) or become the release.
    for i in 0 1 2; do
        if (( new[i] > old[i] )); then higher=1; break; fi
        if (( new[i] < old[i] )); then break; fi
        if (( i == 2 )) && [[ -n $old_pre && $version != "$base" ]] && [[ -z $new_pre || $new_pre > $old_pre ]]; then higher=1; fi
    done
    [ -n "$higher" ] || fail "$version is not higher than $base, the release this tree is based on"

    echo "release: testing"
    just generate css >/dev/null 2>&1
    go vet ./...
    go test ./... >/dev/null

    printf '%s\n' "$version" > "$file"
    git add "$file"
    git commit -q -m "bump version to $version"
    git tag -a "$version" -m "$version"
    printf '%s+dev\n' "$version" > "$file"
    git add "$file"
    git commit -q -m "start development after $version"
    echo "release: tagged $version on $branch"

    if [ '{{ push }}' = push ]; then
        git push --atomic origin "HEAD:refs/heads/$branch" "refs/tags/$version"
    else
        echo "release: nothing pushed; to publish: git push --atomic origin HEAD:refs/heads/$branch refs/tags/$version"
    fi

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
