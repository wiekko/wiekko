#!/bin/sh
# SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
# SPDX-License-Identifier: EUPL-1.2
# Entrypoint hook of the download site (site/Dockerfile): publishes the release
# built into the image on the persistent volume, before nginx starts.
#
# - Copies each release from /opt/nexmap/releases/<version>/ to
#   /data/releases/<version>/ unless it is there already. A release on the
#   volume is never replaced, so its checksums stay what users downloaded.
# - Points /data/releases/latest.json at it if it is a final release. The site
#   deploys branch `stable`, which only ever moves forward to the newest final
#   release, so the image's release is the newest one. (Redeploying an older
#   commit by hand moves latest back with it.)
#
# Copies and the latest.json update are atomic renames, so visitors never see
# a half-written release.
set -eu

src=/opt/nexmap/releases
dst=/data/releases
semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

if ! mkdir -p "$dst" || ! [ -w "$dst" ]; then
    echo "publish-releases: $dst is not writable by uid $(id -u); mount a volume at /data that it can write" >&2
    exit 1
fi

for dir in "$src"/*; do
    [ -d "$dir" ] || continue
    v=${dir##*/}
    if ! printf '%s\n' "$v" | grep -Eq "$semver"; then
        echo "publish-releases: skipping $v, not a release version" >&2
        continue
    fi
    if [ -d "$dst/$v" ]; then
        echo "publish-releases: $v is published already"
    else
        tmp="$dst/.tmp-$v-$$"
        rm -rf "$tmp"
        cp -R "$dir" "$tmp"
        mv "$tmp" "$dst/$v"
        echo "publish-releases: published $v"
    fi
    case $v in
    *-*) ;;
    *)
        cp "$dst/$v/release.json" "$dst/.latest.json.$$"
        mv "$dst/.latest.json.$$" "$dst/latest.json"
        echo "publish-releases: latest is $v"
        ;;
    esac
done
