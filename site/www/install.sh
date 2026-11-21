#!/bin/sh
# SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
# SPDX-License-Identifier: EUPL-1.2
# Installs the Wiekko CLI (wiekko) on Linux or macOS:
#
#   curl -fsSL https://wiekko.net/install.sh | sh
#
# Environment:
#   WIEKKO_VERSION       version to install, e.g. v1.0.1 (default: the latest release)
#   WIEKKO_INSTALL_DIR   target directory (default: /usr/local/bin as root, else ~/.local/bin)
#   WIEKKO_RELEASES_URL  where releases are published (default: https://wiekko.net/releases)
#
# Downloads the archive for this platform, checks it against the release's
# SHA256SUMS and installs the binary. Nothing else is changed.
set -eu

base=${WIEKKO_RELEASES_URL:-https://wiekko.net/releases}
base=${base%/}

fail() {
    echo "wiekko install: $*" >&2
    exit 1
}

case $(uname -s) in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported system $(uname -s); on Windows use install.ps1, otherwise download from $base/" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "unsupported architecture $(uname -m); see $base/ for the available builds" ;;
esac

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO- "$1"; }
else
    fail "needs curl or wget"
fi
if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
    fail "needs sha256sum or shasum to verify the download"
fi

version=${WIEKKO_VERSION:-}
if [ -z "$version" ]; then
    version=$(fetch "$base/latest.json" | sed -n 's/.*"version" *: *"\([^"]*\)".*/\1/p')
    [ -n "$version" ] || fail "could not read the latest version from $base/latest.json"
fi
printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' ||
    fail "'$version' is not a version like v1.2.3"

if [ -n "${WIEKKO_INSTALL_DIR:-}" ]; then
    dir=$WIEKKO_INSTALL_DIR
elif [ "$(id -u)" = 0 ]; then
    dir=/usr/local/bin
else
    dir=$HOME/.local/bin
fi

name="wiekko_${version}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading wiekko $version for $os/$arch"
fetch "$base/$version/SHA256SUMS" >"$tmp/SHA256SUMS" ||
    fail "version $version not found at $base/"
want=$(awk -v n="$name" '$2 == n || $2 == "*" n { print $1 }' "$tmp/SHA256SUMS")
[ -n "$want" ] || fail "$version has no build for $os/$arch"
fetch "$base/$version/$name" >"$tmp/$name" || fail "download of $name failed"
[ "$(sha256 "$tmp/$name")" = "$want" ] ||
    fail "checksum mismatch for $name; the download is damaged or was tampered with"

tar -xzf "$tmp/$name" -C "$tmp" wiekko
mkdir -p "$dir"
# Write next to the target, then rename: replaces a running wiekko safely.
cp "$tmp/wiekko" "$dir/.wiekko.new"
chmod 0755 "$dir/.wiekko.new"
mv -f "$dir/.wiekko.new" "$dir/wiekko"

echo "Installed $("$dir/wiekko" version -o json 2>/dev/null | sed -n 's/.*"version": *"\([^"]*\)".*/wiekko \1/p' | head -n 1) to $dir/wiekko"
case ":$PATH:" in
*":$dir:"*) ;;
*) echo "Note: $dir is not on your PATH; add it, e.g. export PATH=\"$dir:\$PATH\"" ;;
esac
