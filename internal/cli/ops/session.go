// Package ops holds the agent-facing read (and later write) operations. Each
// operation takes an input, talks to the server through internal/client and
// returns a view with IDs resolved to names and identifiers. CLI commands and
// the MCP server are thin front ends over this package, so both always show
// the same thing.
package ops

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"nexmap.net/internal/client"
)

// Session is the context of one invocation: a client, a product, and caches
// of the small lookup tables (statuses, categories, members, …) that views
// need to resolve IDs. It is not meant to outlive one command.
type Session struct {
	C         *client.Client
	ProductID string

	mu      sync.Mutex
	product *client.Product
	cache   map[string]any
}

// NewSession creates a Session for one product.
func NewSession(c *client.Client, productID string) *Session {
	return &Session{C: c, ProductID: productID, cache: map[string]any{}}
}

// cached loads a lookup table once per session.
func cached[T any](s *Session, key string, load func() (T, error)) (T, error) {
	s.mu.Lock()
	if v, ok := s.cache[key]; ok {
		s.mu.Unlock()
		return v.(T), nil
	}
	s.mu.Unlock()
	v, err := load()
	if err != nil {
		var zero T
		return zero, err
	}
	s.mu.Lock()
	s.cache[key] = v
	s.mu.Unlock()
	return v, nil
}

// Product returns the session's product.
func (s *Session) Product(ctx context.Context) (*client.Product, error) {
	s.mu.Lock()
	p := s.product
	s.mu.Unlock()
	if p != nil {
		return p, nil
	}
	p, err := s.C.Product(ctx, s.ProductID)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", s.ProductID, err)
	}
	s.mu.Lock()
	s.product = p
	s.mu.Unlock()
	return p, nil
}

// statuses maps status ID to status, for requirements/tasks and defects.
func (s *Session) statuses(ctx context.Context) (map[string]client.Status, error) {
	return cached(s, "statuses", func() (map[string]client.Status, error) {
		m := map[string]client.Status{}
		for _, et := range []string{"requirement", "defect"} {
			list, err := s.C.Statuses(ctx, s.ProductID, et)
			if err != nil {
				return nil, err
			}
			for _, st := range list {
				m[st.ID] = st
			}
		}
		return m, nil
	})
}

// workflow returns the status names of one entity type in column order.
func (s *Session) workflow(ctx context.Context, entityType string) ([]client.Status, error) {
	return cached(s, "workflow:"+entityType, func() ([]client.Status, error) {
		list, err := s.C.Statuses(ctx, s.ProductID, entityType)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].Position < list[j].Position })
		return list, nil
	})
}

// categories maps category ID to category for requirements, tasks and constraints.
func (s *Session) categories(ctx context.Context) (map[string]client.Category, error) {
	return cached(s, "categories", func() (map[string]client.Category, error) {
		m := map[string]client.Category{}
		for _, et := range []string{"requirement", "task", "constraint"} {
			list, err := s.C.Categories(ctx, s.ProductID, et)
			if err != nil {
				return nil, err
			}
			for _, c := range list {
				m[c.ID] = c
			}
		}
		return m, nil
	})
}

// usernames maps user ID to username for product members.
func (s *Session) usernames(ctx context.Context) (map[string]string, error) {
	return cached(s, "members", func() (map[string]string, error) {
		list, err := s.C.Members(ctx, s.ProductID)
		if err != nil {
			return nil, err
		}
		m := map[string]string{}
		for _, mem := range list {
			if mem.Username != "" {
				m[mem.UserID] = mem.Username
			}
		}
		return m, nil
	})
}

func (s *Session) components(ctx context.Context) ([]client.Component, error) {
	return cached(s, "components", func() ([]client.Component, error) { return s.C.Components(ctx, s.ProductID) })
}

func (s *Session) capabilities(ctx context.Context) ([]client.Capability, error) {
	return cached(s, "capabilities", func() ([]client.Capability, error) { return s.C.Capabilities(ctx, s.ProductID) })
}

func (s *Session) environments(ctx context.Context) ([]client.RuntimeEnvironment, error) {
	return cached(s, "environments", func() ([]client.RuntimeEnvironment, error) {
		return s.C.RuntimeEnvironments(ctx, s.ProductID)
	})
}

func (s *Session) pyramidLevels(ctx context.Context) ([]client.PyramidLevel, error) {
	return cached(s, "pyramid", func() ([]client.PyramidLevel, error) { return s.C.PyramidLevels(ctx, s.ProductID) })
}

func (s *Session) sprints(ctx context.Context) ([]client.Sprint, error) {
	return cached(s, "sprints", func() ([]client.Sprint, error) { return s.C.Sprints(ctx, s.ProductID) })
}

func (s *Session) milestones(ctx context.Context) ([]client.Milestone, error) {
	return cached(s, "milestones", func() ([]client.Milestone, error) { return s.C.Milestones(ctx, s.ProductID) })
}

func (s *Session) campaigns(ctx context.Context) ([]client.Campaign, error) {
	return cached(s, "campaigns", func() ([]client.Campaign, error) { return s.C.Campaigns(ctx, s.ProductID) })
}

// names builds an ID → name map from a slice.
func names[T any](items []T, id func(T) string, name func(T) string) map[string]string {
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[id(it)] = name(it)
	}
	return m
}

func (s *Session) componentNames(ctx context.Context) (map[string]string, error) {
	list, err := s.components(ctx)
	if err != nil {
		return nil, err
	}
	return names(list, func(c client.Component) string { return c.ID }, func(c client.Component) string { return c.Name }), nil
}

func (s *Session) capabilityNames(ctx context.Context) (map[string]string, error) {
	list, err := s.capabilities(ctx)
	if err != nil {
		return nil, err
	}
	return names(list, func(c client.Capability) string { return c.ID }, func(c client.Capability) string { return c.Name }), nil
}

func (s *Session) environmentNames(ctx context.Context) (map[string]string, error) {
	list, err := s.environments(ctx)
	if err != nil {
		return nil, err
	}
	return names(list, func(e client.RuntimeEnvironment) string { return e.ID }, func(e client.RuntimeEnvironment) string { return e.Name }), nil
}

func (s *Session) pyramidNames(ctx context.Context) (map[string]string, error) {
	list, err := s.pyramidLevels(ctx)
	if err != nil {
		return nil, err
	}
	return names(list, func(p client.PyramidLevel) string { return p.ID }, func(p client.PyramidLevel) string { return p.Name }), nil
}

func (s *Session) sprintNames(ctx context.Context) (map[string]string, error) {
	list, err := s.sprints(ctx)
	if err != nil {
		return nil, err
	}
	return names(list, func(p client.Sprint) string { return p.ID }, func(p client.Sprint) string { return p.Name }), nil
}

func (s *Session) milestoneNames(ctx context.Context) (map[string]string, error) {
	list, err := s.milestones(ctx)
	if err != nil {
		return nil, err
	}
	return names(list, func(m client.Milestone) string { return m.ID }, func(m client.Milestone) string { return m.Name }), nil
}

// user renders a user ID as username, falling back to a short ID for users
// who are not (or no longer) members.
func user(usernames map[string]string, id string) string {
	if id == "" {
		return ""
	}
	if n, ok := usernames[id]; ok {
		return n
	}
	return "user:" + ShortID(id)
}

// author names the author of a comment: the server sends no author once the
// account is deleted.
func author(usernames map[string]string, id string) string {
	if id == "" {
		return "deleted user"
	}
	return user(usernames, id)
}

// ShortID returns the first 8 characters of an ID, enough to reference a
// defect or test case unambiguously in practice (see matchID).
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// NotFoundError reports that a reference matched nothing.
type NotFoundError struct {
	Kind string
	Ref  string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("no %s matches %q", e.Kind, e.Ref) }

// AmbiguousError reports that a reference matched several entities.
type AmbiguousError struct {
	Kind    string
	Ref     string
	Matches []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches several %ss: %s", e.Ref, e.Kind, strings.Join(e.Matches, ", "))
}

// match finds the single item whose ID equals ref, whose ID starts with ref
// (at least 4 characters), or for which extra(ref) is true.
func match[T any](kind, ref string, items []T, id func(T) string, label func(T) string, extra func(T) bool) (T, error) {
	var zero T
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return zero, &NotFoundError{Kind: kind, Ref: ref}
	}
	for _, it := range items {
		if id(it) == ref {
			return it, nil
		}
	}
	var found []T
	lower := strings.ToLower(ref)
	for _, it := range items {
		if (len(ref) >= 4 && strings.HasPrefix(strings.ToLower(id(it)), lower)) || (extra != nil && extra(it)) {
			found = append(found, it)
		}
	}
	switch len(found) {
	case 0:
		return zero, &NotFoundError{Kind: kind, Ref: ref}
	case 1:
		return found[0], nil
	}
	labels := make([]string, 0, len(found))
	for _, it := range found {
		labels = append(labels, label(it))
	}
	return zero, &AmbiguousError{Kind: kind, Ref: ref, Matches: labels}
}
