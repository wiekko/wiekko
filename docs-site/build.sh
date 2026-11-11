#!/bin/sh
# Builds the documentation site with every version:
#
#   /latest/     the checked-out tree (main, when Coolify deploys)
#   /<tag>/      every release tag v* that contains this site's configuration
#   /stable/     a copy of the newest release that is not a pre-release
#   /versions.json, /index.html (redirects to /stable/, else /latest/)
#
# Usage: docs-site/build.sh [OUT_DIR]   (default: build/docs-site)
# Needs git and mdbook (or MDBOOK=/path/to/mdbook) on the PATH.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/.." && pwd)
out=${1:-$repo/build/docs-site}
mdbook=${MDBOOK:-mdbook}

rm -rf "$out"
mkdir -p "$out"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# build_version TREE ID: build TREE/docs-site into $out/ID. Every version gets
# today's theme files, so old versions list newer releases in their switcher.
build_version() {
    if [ "$1" != "$repo" ]; then
        mkdir -p "$1/docs-site/theme"
        cp "$here"/theme/* "$1/docs-site/theme/"
    fi
    # site-url lets the 404 page find its assets below the version's path.
    # (MSYS2_ENV_CONV_EXCL keeps Git Bash on Windows from turning the value
    # into a Windows path; elsewhere it has no effect.)
    MSYS2_ENV_CONV_EXCL=MDBOOK_OUTPUT__HTML__SITE_URL \
    MDBOOK_OUTPUT__HTML__SITE_URL="/$2/" "$mdbook" build --dest-dir "$out/$2" "$1/docs-site" >/dev/null
    # mdBook mirrors the source tree; drop the folders of unpublished pages.
    find "$out/$2" -depth -type d -empty -exec rmdir {} \;
}

build_version "$repo" latest
built="latest"

stable=""
versions=""
tags=""
# Release tags come from .git in the build context. Say exactly why they are
# unavailable, since the site then silently shrinks to /latest/.
if [ ! -e "$repo/.git" ]; then
    echo "docs-site: warning: no .git in $repo (not in the build context?); building /latest/ only" >&2
elif ! git_err=$(git -C "$repo" rev-parse --git-dir 2>&1); then
    echo "docs-site: warning: git cannot read $repo/.git: $git_err" >&2
    echo "docs-site: warning: building /latest/ only" >&2
else
    if [ "$(git -C "$repo" rev-parse --is-shallow-repository)" = "true" ]; then
        echo "docs-site: warning: shallow clone; release tags may be missing" >&2
    fi
    # Newest first; pre-releases (v1.0.0-rc.1) sort before their release.
    tags=$(git -C "$repo" -c versionsort.suffix=- tag --list 'v*' --sort=-v:refname)
    echo "docs-site: commit $(git -C "$repo" rev-parse --short HEAD), release tags: ${tags:-none}" | tr '\n' ' '
    echo
fi
for tag in $tags; do
    if ! git -C "$repo" cat-file -e "$tag:docs-site/book.toml" 2>/dev/null; then
        continue # released before the documentation site existed
    fi
    mkdir -p "$work/$tag"
    git -C "$repo" archive "$tag" docs docs-site | tar -x -C "$work/$tag"
    build_version "$work/$tag" "$tag"
    built="$built $tag"
    versions="$versions${versions:+,}\"$tag\""
    case "$tag" in
        *-*) ;; # pre-release
        *) [ -n "$stable" ] || stable=$tag ;;
    esac
done

if [ -n "$stable" ]; then
    cp -R "$out/$stable" "$out/stable"
    start="stable"
    stable_json="\"$stable\""
else
    start="latest"
    stable_json="null"
fi

printf '{"latest": "latest", "stable": %s, "versions": [%s]}\n' "$stable_json" "$versions" > "$out/versions.json"

cat > "$out/index.html" <<EOF
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Nexmap documentation</title>
<meta http-equiv="refresh" content="0; url=$start/">
<link rel="canonical" href="$start/">
</head>
<body><p><a href="$start/">Nexmap documentation</a></p></body>
</html>
EOF

echo "docs-site: built $built${stable:+ (stable: $stable)}"
[ "$built" != "latest" ] || echo "docs-site: no release tags with documentation found"
