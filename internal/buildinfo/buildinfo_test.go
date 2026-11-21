// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package buildinfo

import (
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
)

func TestCompose(t *testing.T) {
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "4161949abcdef0123456789"}
	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	tests := []struct {
		name     string
		src      string
		settings []debug.BuildSetting
		linkRev  string
		want     string
	}{
		{name: "release", src: "v1.2.3\n", settings: []debug.BuildSetting{rev}, want: "v1.2.3"},
		{name: "release from a modified tree", src: "v1.2.3", settings: []debug.BuildSetting{rev, dirty}, want: "v1.2.3+dirty"},
		{name: "pre-release", src: "v1.0.0-rc.1", want: "v1.0.0-rc.1"},
		{name: "dev without vcs", src: "v1.2.3+dev\n", want: "v1.2.3+dev"},
		{name: "dev with vcs", src: "v1.2.3+dev", settings: []debug.BuildSetting{rev}, want: "v1.2.3+dev.4161949abcde"},
		{name: "dev modified", src: "v1.2.3+dev", settings: []debug.BuildSetting{rev, dirty}, want: "v1.2.3+dev.4161949abcde.dirty"},
		{name: "dev with link-time revision", src: "v1.2.3+dev", linkRev: "abc123def4567890", want: "v1.2.3+dev.abc123def456"},
		{name: "vcs wins over link-time revision", src: "v1.2.3+dev", settings: []debug.BuildSetting{rev}, linkRev: "abc123", want: "v1.2.3+dev.4161949abcde"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compose(tt.src, tt.settings, tt.linkRev); got != tt.want {
				t.Fatalf("compose() = %q, want %q", got, tt.want)
			}
		})
	}
}

// versionPattern is a release version as written by `just release`: SemVer
// with a "v" prefix and an optional pre-release, without build metadata.
var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

// The release recipe writes VERSION; a hand edit must keep it parseable.
func TestSourceVersionIsValid(t *testing.T) {
	v := strings.TrimSuffix(strings.TrimSpace(source), devSuffix)
	if !versionPattern.MatchString(v) {
		t.Fatalf("VERSION = %q: want vMAJOR.MINOR.PATCH[-pre-release], optionally followed by %q", strings.TrimSpace(source), devSuffix)
	}
}

func TestStringPrefersLinkTimeVersion(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v9.9.9"
	if got := String(); got != "v9.9.9" {
		t.Fatalf("String() = %q, want v9.9.9", got)
	}
}

func TestRelease(t *testing.T) {
	for v, want := range map[string]string{
		"v1.2.3":                        "v1.2.3",
		"v1.2.3-rc.1":                   "v1.2.3-rc.1",
		"v1.2.3+dirty":                  "v1.2.3",
		"v1.2.3+dev":                    "",
		"v1.2.3+dev.4161949abcde":       "",
		"v1.2.3+dev.4161949abcde.dirty": "",
		"(devel)":                       "",
		"":                              "",
	} {
		if got := release(v); got != want {
			t.Errorf("release(%q) = %q, want %q", v, got, want)
		}
	}
}
