// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os/exec"
	"strings"
	"testing"
)

// allowedModulePackages lists the nexmap.net packages the CLI may link. The CLI
// talks to the server through the REST API only (AC-16); importing server
// packages would couple it to server internals and bloat the binary with the
// database drivers, templates and web assets.
var allowedModulePackages = []string{
	"nexmap.net/cmd/nexmap",
	"nexmap.net/internal/buildinfo",
	"nexmap.net/internal/client",
	"nexmap.net/internal/cli",
	"nexmap.net/internal/ingest",
}

// allowedThirdParty lists the non-module, non-standard-library dependencies.
// A trailing slash allows a module's packages. Everything below the MCP SDK
// line is the SDK with its own dependencies.
var allowedThirdParty = []string{
	"golang.org/x/term",
	"golang.org/x/sys/",

	"github.com/modelcontextprotocol/go-sdk/",
	"github.com/google/jsonschema-go/",
	"github.com/segmentio/asm/",
	"github.com/segmentio/encoding/",
	"github.com/yosida95/uritemplate/",
	"golang.org/x/oauth2",
	"golang.org/x/sync/errgroup",
	"golang.org/x/time/rate",
}

// mcpSDK may only be imported by mcpPackage, which keeps the protocol library
// out of the rest of the CLI.
const (
	mcpSDK     = "github.com/modelcontextprotocol/go-sdk/"
	mcpPackage = "nexmap.net/internal/cli/mcp"
)

func TestCLIDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("runs `go list`; skipped in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not on PATH")
	}
	out, err := exec.Command(goBin, "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "nexmap.net/cmd/nexmap").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	for _, pkg := range strings.Fields(string(out)) {
		if !allowed(pkg) {
			t.Errorf("the CLI must not depend on %s", pkg)
		}
	}

	// Direct imports of each of the CLI's own packages.
	out, err = exec.Command(goBin, "list", "-deps", "-f",
		`{{if not .Standard}}{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{"\n"}}{{end}}`, "nexmap.net/cmd/nexmap").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "nexmap.net/") || fields[0] == mcpPackage {
			continue
		}
		for _, imp := range fields[1:] {
			if strings.HasPrefix(imp, mcpSDK) {
				t.Errorf("%s imports %s; only %s may use the MCP SDK", fields[0], imp, mcpPackage)
			}
		}
	}
}

func allowed(pkg string) bool {
	if strings.HasPrefix(pkg, "nexmap.net/") {
		for _, p := range allowedModulePackages {
			if pkg == p || strings.HasPrefix(pkg, p+"/") {
				return true
			}
		}
		return false
	}
	for _, p := range allowedThirdParty {
		if pkg == strings.TrimSuffix(p, "/") || strings.HasPrefix(pkg, p) {
			return true
		}
	}
	return false
}
