// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package access

import (
	"context"
	"testing"

	"wiekko.net/internal/model"
)

func TestParseGrantsRoundTrip(t *testing.T) {
	g, err := ParseGrants("defects:read  tests:write admin:write tests:read")
	if err != nil {
		t.Fatal(err)
	}
	if g[Tests] != Write {
		t.Errorf("a lower duplicate must not downgrade tests: got %v", g[Tests])
	}
	if got, want := g.String(), "defects:read tests:write admin:write"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	for _, bad := range []string{"defects", "bogus:read", "defects:all"} {
		if _, err := ParseGrants(bad); err == nil {
			t.Errorf("ParseGrants(%q) should fail", bad)
		}
	}
	if g, err := ParseGrants(""); err != nil || len(g) != 0 {
		t.Errorf("empty grants: %v, %v", g, err)
	}
}

func TestFullAccessExcludesAccountAndAdmin(t *testing.T) {
	r := &Restriction{Grants: FullAccess()}
	for _, sc := range ProductScopes {
		if !r.Allows(sc, Write) {
			t.Errorf("full access should write %s", sc)
		}
	}
	if r.Allows(Account, Read) || r.Allows(Admin, Read) {
		t.Error("full access must not include account or admin")
	}
}

func TestNilRestrictionAllowsEverything(t *testing.T) {
	var r *Restriction
	if !r.Allows(Admin, Write) || !r.AllowsProduct("p") || r.CapRole(model.RoleOwner) != model.RoleOwner {
		t.Error("a nil restriction (web session) must not restrict")
	}
	if _, missing := r.Missing([]Need{W(Admin)}); missing {
		t.Error("nil restriction reported a missing need")
	}
}

func TestAllowsLevels(t *testing.T) {
	r := &Restriction{Grants: Grants{Defects: Read, Tests: Write}}
	cases := []struct {
		s    Scope
		l    Level
		want bool
	}{
		{Defects, Read, true}, {Defects, Write, false}, {Tests, Read, true}, {Tests, Write, true},
		{Backlog, Read, false},
	}
	for _, c := range cases {
		if got := r.Allows(c.s, c.l); got != c.want {
			t.Errorf("Allows(%s, %s) = %v, want %v", c.s, c.l, got, c.want)
		}
	}
	if n, missing := r.Missing([]Need{R(Tests), W(Defects)}); !missing || n != W(Defects) {
		t.Errorf("Missing = %v, %v", n, missing)
	}
}

func TestAllowsProduct(t *testing.T) {
	open := &Restriction{}
	if !open.AllowsProduct("a") {
		t.Error("unrestricted products should be reachable")
	}
	r := &Restriction{ProductsRestricted: true, Products: []string{"a"}}
	if !r.AllowsProduct("a") || r.AllowsProduct("b") {
		t.Error("product restriction not applied")
	}
	empty := &Restriction{ProductsRestricted: true}
	if empty.AllowsProduct("a") {
		t.Error("a restricted token with no products must reach nothing")
	}
}

func TestCapRole(t *testing.T) {
	r := &Restriction{RoleCap: model.RoleTester}
	if got := r.CapRole(model.RoleOwner); got != model.RoleTester {
		t.Errorf("owner capped to tester: got %s", got)
	}
	if got := r.CapRole(model.RoleViewer); got != model.RoleViewer {
		t.Errorf("a cap must never raise a role: got %s", got)
	}
}

func TestSubsetOf(t *testing.T) {
	parent := &Restriction{Grants: Grants{Defects: Write, Tests: Read}, RoleCap: model.RoleDeveloper,
		ProductsRestricted: true, Products: []string{"a", "b"}}
	ok := &Restriction{Grants: Grants{Defects: Read}, RoleCap: model.RoleTester,
		ProductsRestricted: true, Products: []string{"a"}}
	if !ok.SubsetOf(parent) {
		t.Error("narrower restriction should be a subset")
	}
	if !ok.SubsetOf(nil) {
		t.Error("everything is a subset of no restriction")
	}
	var none *Restriction
	if none.SubsetOf(parent) {
		t.Error("no restriction is never a subset of a restriction")
	}
	bad := []*Restriction{
		{Grants: Grants{Tests: Write}, RoleCap: model.RoleTester, ProductsRestricted: true, Products: []string{"a"}},
		{Grants: Grants{Account: Write}, RoleCap: model.RoleTester, ProductsRestricted: true, Products: []string{"a"}},
		{Grants: Grants{}, RoleCap: "", ProductsRestricted: true, Products: []string{"a"}},
		{Grants: Grants{}, RoleCap: model.RoleOwner, ProductsRestricted: true, Products: []string{"a"}},
		{Grants: Grants{}, RoleCap: model.RoleTester},
		{Grants: Grants{}, RoleCap: model.RoleTester, ProductsRestricted: true, Products: []string{"c"}},
	}
	for i, r := range bad {
		if r.SubsetOf(parent) {
			t.Errorf("case %d should not be a subset", i)
		}
	}
}

func TestContext(t *testing.T) {
	if FromContext(context.Background()) != nil {
		t.Error("empty context should have no restriction")
	}
	r := &Restriction{RoleCap: model.RoleViewer}
	if FromContext(WithRestriction(context.Background(), r)) != r {
		t.Error("restriction not round-tripped through context")
	}
}

func TestScopeForOwnerType(t *testing.T) {
	cases := map[string]Scope{
		model.OwnerTypeRequirement: Backlog, model.OwnerTypeTask: Backlog, model.OwnerTypeConstraint: Backlog,
		model.OwnerTypeDefect: Defects, model.OwnerTypeTestCase: Tests, model.OwnerTypeDocument: Documents,
		model.OwnerTypeComponent: Structure, model.OwnerTypeCapability: Structure, model.OwnerTypeMilestone: Structure,
	}
	for ot, want := range cases {
		if got := ScopeForOwnerType(ot); got != want {
			t.Errorf("ScopeForOwnerType(%s) = %s, want %s", ot, got, want)
		}
	}
}

// TestPresets checks what each preset grants (SC-58): every product scope,
// documents included, in the full and read-only presets, and a spec author
// that writes the product's definition but no settings.
func TestPresets(t *testing.T) {
	presets := map[string]Grants{}
	for _, p := range Presets() {
		presets[p.Name] = p.Grants
	}
	if presets["full"][Documents] != Write || presets["agent-read"][Documents] != Read {
		t.Errorf("documents missing from the full or read-only preset: %v", presets)
	}
	if presets["ci"][Documents] != None {
		t.Error("CI ingestion reaches documents")
	}
	spec, ok := presets["agent-spec"]
	if !ok {
		t.Fatal("no agent-spec preset")
	}
	for sc, want := range map[Scope]Level{
		Structure: Write, Backlog: Write, Documents: Write, Discussion: Write,
		Defects: Read, Tests: Read, Settings: None, Account: None, Admin: None,
	} {
		if spec[sc] != want {
			t.Errorf("agent-spec grants %s %s, want %s", sc, spec[sc], want)
		}
	}
	full := FullAccess()
	for _, p := range Presets() {
		for sc, l := range p.Grants {
			if l > full[sc] {
				t.Errorf("preset %s exceeds full access on %s", p.Name, sc)
			}
		}
	}
}
