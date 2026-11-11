// Package buildinfo reports the version of the running binary. It is shared by
// the server (nexmapd) and the CLI (nexmap), so it must stay free of any
// dependency beyond the standard library.
package buildinfo

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

// APILevel is the revision of the /api/v1 contract this build implements (the
// server) or requires (the CLI). It increases whenever the server adds API
// surface the CLI depends on; removing or changing surface needs /api/v2.
const APILevel = 1

// source is the version recorded in the source tree. `just release vX.Y.Z`
// writes "vX.Y.Z" for the tagged release commit and "vX.Y.Z+dev" for the
// commit right after it, so a build between releases names the release it is
// based on rather than guessing the next one.
//
//go:embed VERSION
var source string

// Version overrides everything when set at link time:
//
//	go build -ldflags "-X nexmap.net/internal/buildinfo.Version=v1.2.3" ./cmd/nexmap
var Version = ""

// Revision is the commit a build without VCS metadata was made from, set at
// link time (the Dockerfile passes Coolify's SOURCE_COMMIT). VCS metadata the
// Go toolchain embeds takes precedence.
var Revision = ""

// devSuffix marks a version as "based on this release, plus later changes".
// It is SemVer build metadata: version ordering ignores it.
const devSuffix = "+dev"

// String returns the version of the running binary:
//
//   - the link-time Version, if set;
//   - for a release ("v1.2.3"), that version, with "+dirty" when built from a
//     modified tree;
//   - between releases ("v1.2.3+dev"), that version followed by the short
//     commit ("v1.2.3+dev.4161949abcde") and ".dirty" when known.
func String() string {
	if Version != "" {
		return Version
	}
	var settings []debug.BuildSetting
	if info, ok := debug.ReadBuildInfo(); ok {
		settings = info.Settings
	}
	return compose(source, settings, Revision)
}

func compose(src string, settings []debug.BuildSetting, linkRevision string) string {
	v := strings.TrimSpace(src)
	revision, dirty := strings.TrimSpace(linkRevision), false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if !strings.HasSuffix(v, devSuffix) {
		if dirty {
			v += "+dirty"
		}
		return v
	}
	if revision != "" {
		v += "." + revision
	}
	if dirty {
		v += ".dirty"
	}
	return v
}
