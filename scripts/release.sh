#!/bin/sh
# SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
# SPDX-License-Identifier: EUPL-1.2
# Makes a release: see CONTRIBUTING.md, "Releasing".
#
# Usage: scripts/release.sh VERSION [push]      (usually: just release VERSION [push])
#
# From main or a release/* branch with a clean tree: runs go vet and the tests,
# commits internal/buildinfo/VERSION as VERSION ("bump version to VERSION") and
# tags that commit, then commits VERSION+dev ("start development after
# VERSION"). With "push", pushes both commits and the tag.
#
# A final release (no pre-release suffix) made from main also moves the branch
# "stable" to the release commit, so deployments that track stable get exactly
# the tagged release, never the +dev commit after it. Pre-releases and
# releases from release/* branches leave stable alone; the push is a
# fast-forward, so it fails rather than move stable back or sideways.
#
# Plain POSIX sh: `just` runs it with the same sh as its other recipes, which
# on Windows is Git's.
set -eu

fail() {
    echo "release: $*" >&2
    exit 1
}

[ $# -ge 1 ] && [ $# -le 2 ] || fail "usage: just release vX.Y.Z [push]"
version=$1
push=${2:-}
case $push in "" | push) ;; *) fail "second argument must be 'push' or nothing" ;; esac

cd "$(dirname "$0")/.."
file=internal/buildinfo/VERSION

# parse VERSION: prints "MAJOR MINOR PATCH PRE" (PRE empty for a release), or
# fails for anything but vMAJOR.MINOR.PATCH[-PRE].
semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
parse() {
    printf '%s\n' "$1" | grep -Eq "$semver" || return 1
    printf '%s\n' "$1" | sed 's/^v\([0-9]*\)\.\([0-9]*\)\.\([0-9]*\)-\{0,1\}\(.*\)$/\1 \2 \3 \4/'
}

new=$(parse "$version") || fail "$version is not a version like v1.2.3 or v1.2.3-rc.1"

branch=$(git symbolic-ref --quiet --short HEAD) || fail "not on a branch"
case $branch in main | release/*) ;; *) fail "releases are made from main or release/*, not $branch" ;; esac
[ -z "$(git status --porcelain)" ] || fail "the working tree has uncommitted changes"
if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
    fail "tag $version exists already"
fi

# The new version must be higher than the release the tree is based on.
base=$(tr -d ' \t\r\n' <"$file")
base=${base%+dev}
old=$(parse "$base") || fail "$file holds '$base', not a version"
set -- $new
n1=$1 n2=$2 n3=$3 npre=${4:-}
set -- $old
o1=$1 o2=$2 o3=$3 opre=${4:-}
higher=
if [ "$n1" -ne "$o1" ]; then
    [ "$n1" -gt "$o1" ] && higher=1
elif [ "$n2" -ne "$o2" ]; then
    [ "$n2" -gt "$o2" ] && higher=1
elif [ "$n3" -ne "$o3" ]; then
    [ "$n3" -gt "$o3" ] && higher=1
elif [ -n "$opre" ] && [ "$npre" != "$opre" ]; then
    # Same core version: only a pre-release may move on or become the release.
    # Pre-releases compare as text, so rc.10 sorts before rc.9.
    if [ -z "$npre" ] || [ "$(printf '%s\n%s\n' "$opre" "$npre" | LC_ALL=C sort | tail -n 1)" = "$npre" ]; then
        higher=1
    fi
fi
[ -n "$higher" ] || fail "$version is not higher than $base, the release this tree is based on"

echo "release: testing"
just generate css >/dev/null 2>&1 || fail "just generate css failed"
go vet ./...
go test ./... >/dev/null || fail "tests failed; run go test ./... to see why"

printf '%s\n' "$version" >"$file"
git add "$file"
git commit -q -m "bump version to $version"
git tag -a "$version" -m "$version"
release_commit=$(git rev-parse HEAD)
printf '%s+dev\n' "$version" >"$file"
git add "$file"
git commit -q -m "start development after $version"
echo "release: tagged $version on $branch"

refs="HEAD:refs/heads/$branch refs/tags/$version"
if [ "$branch" = main ] && [ -z "$npre" ]; then
    refs="$refs $release_commit:refs/heads/stable"
    echo "release: stable moves to $version"
fi

if [ "$push" = push ]; then
    # shellcheck disable=SC2086 # refs is a list of refspecs without spaces
    git push --atomic origin $refs ||
        fail "push refused, nothing was pushed; the commits and tag $version are made locally. Fix the cause, then: git push --atomic origin $refs"
else
    echo "release: nothing pushed; to publish: git push --atomic origin $refs"
fi
