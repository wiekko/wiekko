package router

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/service"
)

// resourceScopes maps the resource segments of API paths to their scope.
// Segments not listed (assignees, links, results, …) name no resource of
// their own.
var resourceScopes = map[string]access.Scope{
	"components": access.Structure, "capabilities": access.Structure,
	"milestones": access.Structure, "sprints": access.Structure,
	"requirements": access.Backlog, "constraints": access.Backlog,
	"defects":    access.Defects,
	"test-cases": access.Tests, "campaigns": access.Tests, "test-runs": access.Tests,
	"comments": access.Discussion, "attachments": access.Discussion,
	"members": access.Settings, "transfer-ownership": access.Settings, "categories": access.Settings,
	"statuses": access.Settings, "pyramid-levels": access.Settings, "runtime-environments": access.Settings,
}

// expectedNeeds is the scope rule of apiRoutes, written out independently.
func expectedNeeds(t *testing.T, method, path string) []access.Need {
	t.Helper()
	level := access.Read
	if method != http.MethodGet {
		level = access.Write
	}
	switch {
	case strings.HasPrefix(path, "/auth/"):
		return nil
	case path == "/user/me" && level == access.Read:
		return nil // login and whoami identify the token's user
	case strings.HasPrefix(path, "/user/me"):
		return []access.Need{{Scope: access.Account, Level: level}}
	case strings.HasPrefix(path, "/admin/"):
		return []access.Need{{Scope: access.Admin, Level: level}}
	case path == "/products" || path == "/products/{productId}":
		if level == access.Write {
			return []access.Need{access.W(access.Settings)}
		}
		return nil
	}
	rest, ok := strings.CutPrefix(path, "/products/{productId}/")
	if !ok {
		t.Fatalf("%s %s: no scope rule for this path", method, path)
	}
	segs := strings.Split(rest, "/")
	switch segs[0] {
	case "audit":
		return readAll
	case "specification": // the export covers the structure and the backlog
		return []access.Need{access.R(access.Structure), access.R(access.Backlog)}
	}
	if _, ok := resourceScopes[segs[0]]; !ok {
		t.Fatalf("%s %s: unknown resource %q; add it to resourceScopes", method, path, segs[0])
	}
	var scopes []access.Scope
	for _, seg := range segs {
		if sc, ok := resourceScopes[seg]; ok && !slices.Contains(scopes, sc) {
			scopes = append(scopes, sc)
		}
	}
	needs := []access.Need{{Scope: scopes[0], Level: level}}
	for _, sc := range scopes[1:] {
		needs = append(needs, access.R(sc))
	}
	return needs
}

// TestAPIRouteScopes checks every authenticated API route's declared scopes
// against the rule, so a new route cannot slip through with the wrong ones.
func TestAPIRouteScopes(t *testing.T) {
	routes := mountAPIv1(http.NewServeMux(), &service.Services{}, true)
	if len(routes) < 100 {
		t.Fatalf("only %d routes registered", len(routes))
	}
	for pattern, got := range routes {
		method, path, _ := strings.Cut(pattern, " ")
		want := expectedNeeds(t, method, path)
		if !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("%s: declared %v, want %v", pattern, got, want)
		}
	}
}
