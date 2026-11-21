// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package patterns

import (
	"strings"
	"testing"

	"wiekko.net/internal/buildinfo"
)

// A release pins the CLI install to itself; a build between releases installs
// the latest release (UR-172).
func TestInstallCommands(t *testing.T) {
	pinned := InstallCommands("v1.2.3")
	if len(pinned) != 2 ||
		pinned[0].Command != "curl -fsSL https://wiekko.net/install.sh | WIEKKO_VERSION=v1.2.3 sh" ||
		pinned[1].Command != "$env:WIEKKO_VERSION='v1.2.3'; irm https://wiekko.net/install.ps1 | iex" {
		t.Errorf("pinned: %+v", pinned)
	}
	for _, c := range InstallCommands("") {
		if strings.Contains(c.Command, "WIEKKO_VERSION") {
			t.Errorf("unpinned command names a version: %s", c.Command)
		}
	}
}

func TestDocsURL(t *testing.T) {
	want := "https://docs.wiekko.net/latest/cli/setup.html"
	if r := buildinfo.Release(); r != "" {
		want = "https://docs.wiekko.net/" + r + "/cli/setup.html"
	}
	if got := DocsURL("cli/setup.html"); got != want {
		t.Errorf("DocsURL = %q, want %q", got, want)
	}
}
