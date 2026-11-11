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

# The release tags come from the git repository: the checkout's own .git if
# there is one (local builds), otherwise a fresh clone of the repository.
# Coolify passes the build only the working tree, so the clone is the normal
# case there; the repository is private, so it needs a read-only access token,
# given as the BuildKit secret DOCS_GIT_TOKEN (preferred: a file in
# /run/secrets) or the environment variable DOCS_GIT_TOKEN.
remote=${DOCS_GIT_URL:-https://forge.my-universe.com/daemotron/nexmap.git}
token=${DOCS_GIT_TOKEN:-}
token_file=${DOCS_GIT_TOKEN_FILE:-/run/secrets/DOCS_GIT_TOKEN}
if [ -z "$token" ] && [ -s "$token_file" ]; then
    token=$(cat "$token_file")
fi

gitdir=""
if [ -e "$repo/.git" ]; then
    if gd=$(git -C "$repo" rev-parse --absolute-git-dir 2>&1); then
        gitdir=$gd
        if [ "$(git -C "$repo" rev-parse --is-shallow-repository)" = "true" ]; then
            echo "docs-site: warning: shallow clone; release tags may be missing" >&2
        fi
    else
        echo "docs-site: warning: git cannot read $repo/.git: $gd" >&2
    fi
fi
if [ -z "$gitdir" ] && [ -n "$token" ]; then
    echo "docs-site: no usable .git; fetching release tags from $remote"
    # The token travels in a header set through the environment, so it shows
    # up neither in the command line nor in git's error messages. No
    # credential helper: a rejected token must fail, not prompt.
    auth=$(printf 'docs-build:%s' "$token" | base64 | tr -d '\n')
    if ! GIT_TERMINAL_PROMPT=0 GIT_CONFIG_COUNT=2 \
        GIT_CONFIG_KEY_0=http.extraHeader GIT_CONFIG_VALUE_0="Authorization: Basic $auth" \
        GIT_CONFIG_KEY_1=credential.helper GIT_CONFIG_VALUE_1= \
        git clone --quiet --bare "$remote" "$work/remote.git" 2>"$work/clone.err"; then
        # A token was given, so versions are expected: fail rather than
        # silently publishing a site without them.
        echo "docs-site: error: cannot clone $remote: $(cat "$work/clone.err")" >&2
        echo "docs-site: error: check that DOCS_GIT_TOKEN is valid and can read the repository" >&2
        exit 1
    fi
    gitdir="$work/remote.git"
fi

if [ -n "$gitdir" ]; then
    # Newest first; pre-releases (v1.0.0-rc.1) sort before their release.
    tags=$(git --git-dir="$gitdir" -c versionsort.suffix=- tag --list 'v*' --sort=-v:refname)
    echo "docs-site: release tags: $(echo ${tags:-none})"
else
    echo "docs-site: warning: no .git and no DOCS_GIT_TOKEN; building /latest/ only" >&2
fi

for tag in $tags; do
    if ! git --git-dir="$gitdir" cat-file -e "$tag:docs-site/book.toml" 2>/dev/null; then
        continue # released before the documentation site existed
    fi
    mkdir -p "$work/$tag"
    git --git-dir="$gitdir" archive "$tag" docs docs-site | tar -x -C "$work/$tag"
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
