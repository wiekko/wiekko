package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{
			name: "module version wins",
			info: debug.BuildInfo{Main: debug.Module{Version: "v1.4.0"}},
			want: "v1.4.0",
		},
		{
			name: "devel without vcs",
			info: debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want: "dev",
		},
		{
			name: "vcs revision shortened",
			info: debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "4161949abcdef0123456789"}},
			},
			want: "dev-4161949abcde",
		},
		{
			name: "dirty tree",
			info: debug.BuildInfo{
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "4161949"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			want: "dev-4161949-dirty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromBuildInfo(&tt.info); got != tt.want {
				t.Fatalf("fromBuildInfo() = %q, want %q", got, tt.want)
			}
		})
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
