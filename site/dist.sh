#!/bin/sh
# Builds the CLI release archives for every platform: run by site/Dockerfile
# (stage dist), which pins the Go toolchain. Not meant to run on a host.
#
# Usage: site/dist.sh OUT_DIR
#
# Writes OUT_DIR/<version>/:
#   nexmap_<version>_<os>_<arch>.tar.gz   (.zip for windows), binary + LICENSE
#   SHA256SUMS                            sha256sum format
#   release.json                          {"version": …, "api_level": …}
#
# The version comes from internal/buildinfo/VERSION, or from the VERSION
# environment variable (test builds). Only release versions are packaged:
# a "+dev" tree fails, so a deployment that tracks the wrong branch is noticed.
set -eu

out=${1:?usage: dist.sh OUT_DIR}
targets=${TARGETS:-"linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"}

cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# -buildvcs=false: release binaries carry neither VCS data nor a module
# version, which is how `nexmap update` tells them from `go install` builds.
ldflags="-s -w"
if [ -n "${VERSION:-}" ]; then
    ldflags="$ldflags -X nexmap.net/internal/buildinfo.Version=$VERSION"
fi
build() { # GOOS GOARCH OUTFILE
    CGO_ENABLED=0 GOOS=$1 GOARCH=$2 go build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$3" ./cmd/nexmap
}

# Ask a native build for its version and API level, so the metadata matches
# exactly what the binaries report. It runs outside the repository and with
# an empty configuration, so it never contacts a server.
build "$(go env GOOS)" "$(go env GOARCH)" "$work/native/nexmap"
info=$(cd "$work" && env -u NEXMAP_SERVER NEXMAP_CONFIG_DIR="$work/config" ./native/nexmap version -o json)
field() {
    printf '%s\n' "$info" | sed -n "s/^ *\"$1\": *\"\{0,1\}\([^\",]*\)\"\{0,1\},\{0,1\}\$/\1/p" | head -n 1
}
version=$(field version)
api_level=$(field api_level)

semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if ! printf '%s\n' "$version" | grep -Eq "$semver"; then
    echo "dist: '$version' is not a release version; build a tagged release, or pass VERSION=vX.Y.Z for a test build" >&2
    exit 1
fi
case $api_level in '' | *[!0-9]*)
    echo "dist: could not read the API level from: $info" >&2
    exit 1
    ;;
esac

dest="$out/$version"
rm -rf "$dest"
mkdir -p "$dest"
echo "dist: $version (API level $api_level)"

for target in $targets; do
    os=${target%/*}
    arch=${target#*/}
    dir="$work/$os-$arch"
    bin=nexmap
    [ "$os" = windows ] && bin=nexmap.exe
    build "$os" "$arch" "$dir/$bin"
    cp LICENSE "$dir/LICENSE"
    # Fixed timestamps and owners, so the same binaries give the same archive.
    touch -d @0 "$dir/$bin" "$dir/LICENSE"
    name="nexmap_${version}_${os}_${arch}"
    if [ "$os" = windows ]; then
        (cd "$dir" && zip -qX "$dest/$name.zip" "$bin" LICENSE)
    else
        tar -C "$dir" --owner=0 --group=0 --numeric-owner -cf - "$bin" LICENSE | gzip -9n >"$dest/$name.tar.gz"
    fi
    echo "dist:   $os/$arch"
done

(cd "$dest" && sha256sum nexmap_* >SHA256SUMS)
printf '{"version":"%s","api_level":%s}\n' "$version" "$api_level" >"$dest/release.json"
