// Package buildinfo reports the version of the running binary. It is shared by
// the server (nexmapd) and the CLI (nexmap), so it must stay free of any
// dependency beyond the standard library.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// APILevel is the revision of the /api/v1 contract this build implements (the
// server) or requires (the CLI). It increases whenever the server adds API
// surface the CLI depends on; removing or changing surface needs /api/v2.
const APILevel = 1

// Version may be set at link time:
//
//	go build -ldflags "-X nexmap.net/internal/buildinfo.Version=v1.2.3" ./cmd/nexmap
//
// When it is empty, String derives a version from the module and VCS metadata
// the Go toolchain embeds.
var Version = ""

// String returns the version of the running binary: the link-time Version if
// set, else the module version, else the short VCS revision (with a "-dirty"
// suffix for modified trees), else "dev".
func String() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	return fromBuildInfo(info)
}

func fromBuildInfo(info *debug.BuildInfo) string {
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	var b strings.Builder
	b.WriteString("dev-")
	b.WriteString(revision)
	if dirty {
		b.WriteString("-dirty")
	}
	return b.String()
}
