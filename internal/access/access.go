// Package access models the restrictions a personal access token can carry
// (SC-401 – SC-409): the products it reaches, a cap on the role it acts with,
// and the scopes it may read or write. It is shared by the middleware, which
// enforces scopes per route, and the services, which enforce products, the
// role cap and the scopes of comment and attachment owners.
package access

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"nexmap.net/internal/model"
)

// Scope names an area of the API a token can be granted.
type Scope string

// Product scopes cover the data of a product; Account and Admin cover the
// user's own account and the instance administration.
const (
	Structure  Scope = "structure"  // components, capabilities, milestones, sprints
	Backlog    Scope = "backlog"    // requirements, tasks, constraints
	Defects    Scope = "defects"    // defects
	Tests      Scope = "tests"      // test cases, campaigns, results, test runs
	Discussion Scope = "discussion" // comments, attachments
	Settings   Scope = "settings"   // product settings and members
	Account    Scope = "account"    // the user's account and tokens
	Admin      Scope = "admin"      // instance administration
)

// ProductScopes lists the scopes of product data, in display order.
var ProductScopes = []Scope{Structure, Backlog, Defects, Tests, Discussion, Settings}

// AllScopes lists every scope, in display order.
var AllScopes = []Scope{Structure, Backlog, Defects, Tests, Discussion, Settings, Account, Admin}

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool { return slices.Contains(AllScopes, s) }

// Level is how far a scope is granted. Write includes read.
type Level int

// Levels, in increasing order.
const (
	None Level = iota
	Read
	Write
)

func (l Level) String() string {
	switch l {
	case Read:
		return "read"
	case Write:
		return "write"
	default:
		return "none"
	}
}

// ParseLevel parses "none", "read" or "write".
func ParseLevel(s string) (Level, error) {
	switch s {
	case "", "none":
		return None, nil
	case "read":
		return Read, nil
	case "write":
		return Write, nil
	}
	return None, fmt.Errorf("unknown scope level %q", s)
}

// Grants maps scopes to the level they are granted at. Scopes not present
// are not granted.
type Grants map[Scope]Level

// ParseGrants parses the stored form "backlog:write defects:read".
func ParseGrants(s string) (Grants, error) {
	g := Grants{}
	for _, f := range strings.Fields(s) {
		name, lvl, ok := strings.Cut(f, ":")
		if !ok {
			return nil, fmt.Errorf("invalid scope %q: want scope:level", f)
		}
		sc := Scope(name)
		if !sc.Valid() {
			return nil, fmt.Errorf("unknown scope %q", name)
		}
		l, err := ParseLevel(lvl)
		if err != nil {
			return nil, err
		}
		if l > g[sc] {
			g[sc] = l
		}
	}
	return g, nil
}

// String returns the stored form, in AllScopes order, leaving out scopes
// that are not granted.
func (g Grants) String() string {
	parts := make([]string, 0, len(g))
	for _, sc := range AllScopes {
		if l := g[sc]; l > None {
			parts = append(parts, string(sc)+":"+l.String())
		}
	}
	return strings.Join(parts, " ")
}

// FullAccess grants write on every product scope, but not Account or Admin,
// which a token only receives when asked for explicitly (SC-406).
func FullAccess() Grants {
	g := Grants{}
	for _, sc := range ProductScopes {
		g[sc] = Write
	}
	return g
}

// Preset is a named set of grants offered when creating a token (SC-408).
type Preset struct {
	Name   string
	Label  string
	Grants Grants
}

// Presets lists the presets offered when creating a token.
func Presets() []Preset {
	readOnly := Grants{}
	for _, sc := range ProductScopes {
		readOnly[sc] = Read
	}
	return []Preset{
		{Name: "full", Label: "Full access", Grants: FullAccess()},
		{Name: "ci", Label: "CI ingestion", Grants: Grants{Tests: Write}},
		{Name: "agent-read", Label: "Agent, read-only", Grants: readOnly},
	}
}

// Restriction is what a token is limited to. A nil *Restriction means the
// request is not token-authenticated (a web session) and is not restricted.
type Restriction struct {
	Grants             Grants
	RoleCap            string   // "" if the role is not capped
	ProductsRestricted bool     // true if only Products are reachable
	Products           []string // reachable products when ProductsRestricted
}

// FromPAT reads the restriction stored on a token.
func FromPAT(p *model.PAT) (*Restriction, error) {
	g, err := ParseGrants(p.Scopes)
	if err != nil {
		return nil, err
	}
	return &Restriction{Grants: g, RoleCap: p.RoleCap, ProductsRestricted: p.ProductsRestricted,
		Products: p.ProductIDs}, nil
}

// Allows reports whether scope s is granted at level l or higher.
func (r *Restriction) Allows(s Scope, l Level) bool {
	if r == nil {
		return true
	}
	return r.Grants[s] >= l
}

// AllowsProduct reports whether the product is reachable.
func (r *Restriction) AllowsProduct(productID string) bool {
	if r == nil || !r.ProductsRestricted {
		return true
	}
	return slices.Contains(r.Products, productID)
}

// CapRole returns the lower of role and the role cap.
func (r *Restriction) CapRole(role string) string {
	if r == nil || r.RoleCap == "" {
		return role
	}
	if model.RoleRank(role) > model.RoleRank(r.RoleCap) {
		return r.RoleCap
	}
	return role
}

// SubsetOf reports whether r grants nothing that o does not (SC-406). Any
// restriction is a subset of no restriction.
func (r *Restriction) SubsetOf(o *Restriction) bool {
	if o == nil {
		return true
	}
	if r == nil {
		return false
	}
	for sc, l := range r.Grants {
		if l > o.Grants[sc] {
			return false
		}
	}
	if o.RoleCap != "" && (r.RoleCap == "" || model.RoleRank(r.RoleCap) > model.RoleRank(o.RoleCap)) {
		return false
	}
	if o.ProductsRestricted {
		if !r.ProductsRestricted {
			return false
		}
		for _, p := range r.Products {
			if !slices.Contains(o.Products, p) {
				return false
			}
		}
	}
	return true
}

// Need is a scope a route requires at a given level.
type Need struct {
	Scope Scope
	Level Level
}

// R requires read on s.
func R(s Scope) Need { return Need{s, Read} }

// W requires write on s.
func W(s Scope) Need { return Need{s, Write} }

// Missing returns the first need r does not satisfy, if any.
func (r *Restriction) Missing(needs []Need) (Need, bool) {
	for _, n := range needs {
		if !r.Allows(n.Scope, n.Level) {
			return n, true
		}
	}
	return Need{}, false
}

// ScopeForOwnerType returns the scope of an item that carries comments and
// attachments (SC-405).
func ScopeForOwnerType(ownerType string) Scope {
	switch ownerType {
	case model.OwnerTypeRequirement, model.OwnerTypeTask, model.OwnerTypeConstraint:
		return Backlog
	case model.OwnerTypeDefect:
		return Defects
	case model.OwnerTypeTestCase:
		return Tests
	default: // component, capability, milestone
		return Structure
	}
}

type ctxKey struct{}

// WithRestriction stores a token's restriction in the context.
func WithRestriction(ctx context.Context, r *Restriction) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// ViaToken reports whether the request is token-authenticated.
func ViaToken(ctx context.Context) bool {
	return FromContext(ctx) != nil
}

// FromContext returns the restriction of the request's token, or nil when
// the request is not token-authenticated.
func FromContext(ctx context.Context) *Restriction {
	r, _ := ctx.Value(ctxKey{}).(*Restriction)
	return r
}
