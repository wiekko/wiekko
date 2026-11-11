#!/bin/sh
# Runs the tests with the PostgreSQL runs included: starts a throwaway server
# in a container (data on tmpfs, durability off), points the tests at it and
# removes it afterwards.
#
# Usage: scripts/test-pg.sh [go test arguments]   (usually: just test-pg [...])
# ENGINE (podman or docker) and PG_IMAGE come from the justfile.
#
# Plain POSIX sh: `just` runs it with the same sh as its other recipes, which
# on Windows is Git's.
set -eu

engine=${ENGINE:-podman}
image=${PG_IMAGE:-docker.io/library/postgres:18}
[ $# -gt 0 ] || set -- ./...

# Git Bash on Windows would rewrite container paths like /var/lib/...
export MSYS_NO_PATHCONV=1

id=$($engine run -d --rm -e POSTGRES_PASSWORD=nexmap -p 127.0.0.1::5432 \
    --tmpfs /var/lib/postgresql "$image" \
    -c fsync=off -c synchronous_commit=off -c full_page_writes=off -c max_connections=500)
trap '$engine rm -f "$id" >/dev/null' EXIT

# The image's init phase runs a temporary server on the Unix socket only; the
# real one is up once TCP answers.
i=0
until $engine exec "$id" pg_isready -q -h 127.0.0.1 -U postgres; do
    i=$((i + 1))
    if [ "$i" -ge 60 ]; then
        echo "postgres did not come up" >&2
        $engine logs "$id" >&2
        exit 1
    fi
    sleep 1
done

port=$($engine port "$id" 5432/tcp | head -n 1 | sed 's/.*://')
export NEXMAP_TEST_POSTGRES_DSN="postgres://postgres:nexmap@127.0.0.1:${port}/postgres?sslmode=disable"
export NEXMAP_TEST_POSTGRES=required
go test "$@"
