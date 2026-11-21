// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// Restricted personal access tokens (SC-51 – SC-56) through the CLI.

import (
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/cli"
	cliconfig "nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// withToken runs the CLI with the given raw token.
func (e *e2e) withToken(token, stdin string, args ...string) result {
	e.t.Helper()
	saved := e.env[cliconfig.EnvToken]
	defer func() { e.env[cliconfig.EnvToken] = saved }()
	e.env[cliconfig.EnvToken] = token
	return e.run(stdin, "", args...)
}

func (e *e2e) aviaryID() string { return e.env[cliconfig.EnvProduct] }

// wantScopeRefusal asserts the server refused the request for a missing scope.
func wantScopeRefusal(t *testing.T, what string, r result) {
	t.Helper()
	if r.code != cli.ExitAPI || !strings.Contains(r.stderr, "scope") {
		t.Errorf("%s: exit %d, stderr %q; want a scope refusal (exit %d)", what, r.code, r.stderr, cli.ExitAPI)
	}
}

func TestE2ETokenCIIngestion(t *testing.T) {
	e := newAviary(t)
	// bo is a tester in Aviary; a CI token needs nothing but tests:write.
	ci := e.mint("bo", service.CreatePATInput{Name: "ci", Grants: access.Grants{access.Tests: access.Write},
		ProductsRestricted: true, ProductIDs: []string{e.aviaryID()}})

	args := []string{"utest", "--identifier-style", "name", "--env", "android 14", "-o", "json"}
	r := e.withToken(ci, aviaryRun, args...)
	if r.code != cli.ExitOK {
		t.Fatalf("utest with a CI token: exit %d\nstderr: %s", r.code, r.stderr)
	}
	run := decode[utestJSON](t, r.stdout)
	if run.Report == nil || !run.Report.Recorded {
		t.Fatalf("results not recorded: %+v", run.Report)
	}
	filed := false
	for _, res := range run.Report.Results {
		filed = filed || res.DefectID != ""
	}
	if !filed {
		t.Error("a failing test filed no defect, although tests:write covers it (SC-55)")
	}

	// The same token cannot read the backlog or defects.
	wantScopeRefusal(t, "req list", e.withToken(ci, "", "req", "list"))
	wantScopeRefusal(t, "defect list", e.withToken(ci, "", "defect", "list"))
}

func TestE2ETokenReadOnly(t *testing.T) {
	e := newAviary(t)
	readOnly := access.Grants{}
	for _, sc := range access.ProductScopes {
		readOnly[sc] = access.Read
	}
	// cy is a developer in Aviary; the token caps her at viewer as well.
	ro := e.mint("cy", service.CreatePATInput{Name: "agent", Grants: readOnly, RoleCap: model.RoleViewer})

	reqs := decode[[]ops.RequirementSummary](t, e.withToken(ro, "", "req", "list", "-o", "json").stdout)
	if len(reqs) == 0 {
		t.Fatal("a read-only token should list requirements")
	}
	wantScopeRefusal(t, "req create", e.withToken(ro, "", "req", "create", "--title", "Read-only agents cannot write"))

	// With write scope but a viewer cap, the role check refuses instead.
	capped := e.mint("cy", service.CreatePATInput{Name: "capped", RoleCap: model.RoleViewer})
	wantForbidden(t, "capped developer", e.withToken(capped, "", "req", "status", reqs[0].Identifier, "In", "Review"))

	// whoami shows the restriction.
	who := e.withToken(ro, "", "whoami")
	if who.code != cli.ExitOK {
		t.Fatalf("whoami: exit %d, stderr %s", who.code, who.stderr)
	}
	for _, want := range []string{"backlog:read", "viewer"} {
		if !strings.Contains(who.stdout, want) {
			t.Errorf("whoami does not show %q:\n%s", want, who.stdout)
		}
	}
}

func TestE2ETokenProductRestriction(t *testing.T) {
	e := newAviary(t)
	var other string
	for _, p := range e.products {
		if p.ID != e.aviaryID() {
			other = p.ID
			break
		}
	}
	if other == "" {
		t.Skip("seed has a single product")
	}
	tok := e.mint("root", service.CreatePATInput{Name: "one product", ProductsRestricted: true, ProductIDs: []string{other}})
	r := e.withToken(tok, "", "req", "list")
	if r.code == cli.ExitOK {
		t.Fatalf("a token restricted to another product read Aviary:\n%s", r.stdout)
	}
}
