package main

import (
	"os/exec"
	"strings"
	"testing"
)

// allowedModulePackages lists the nexmap.net packages the CLI may link. The CLI
// talks to the server through the REST API only (AC-224); importing server
// packages would couple it to server internals and bloat the binary with the
// database drivers, templates and web assets.
var allowedModulePackages = []string{
	"nexmap.net/cmd/nexmap",
	"nexmap.net/internal/buildinfo",
	"nexmap.net/internal/client",
	"nexmap.net/internal/cli",
}

// allowedThirdParty lists the non-module, non-standard-library dependencies.
var allowedThirdParty = []string{
	"golang.org/x/term",
	"golang.org/x/sys/",
}

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
