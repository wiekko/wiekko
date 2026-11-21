// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"wiekko.net/internal/client"
)

// Writes to the product structure, constraints and traceability links
// (FR-134, FR-135). Like the other writes, names and identifiers are resolved
// before the first request, and every operation returns the view the
// matching show operation returns.

// forget drops cached lists after a write changed them.
func (s *Session) forget(keys ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.cache, k)
	}
}

// findComponent matches a component by name (case-insensitive), ID or ID
// prefix.
func (s *Session) findComponent(ctx context.Context, ref string) (client.Component, error) {
	comps, err := s.components(ctx)
	if err != nil {
		return client.Component{}, err
	}
	return match("component", ref, comps,
		func(c client.Component) string { return c.ID },
		func(c client.Component) string { return c.Name },
		func(c client.Component) bool { return strings.EqualFold(c.Name, strings.TrimSpace(ref)) })
}

// componentView returns one component with its capabilities.
func (s *Session) componentView(ctx context.Context, id string) (*ComponentView, error) {
	st, err := s.Structure(ctx)
	if err != nil {
		return nil, err
	}
	for i := range st.Components {
		if st.Components[i].ID == id {
			return &st.Components[i], nil
		}
	}
	return nil, &NotFoundError{Kind: "component", Ref: id}
}

// NewComponent creates a component.
type NewComponent struct {
	Name        string `json:"name" jsonschema:"component name, unique in the product"`
	Description string `json:"description,omitempty" jsonschema:"Markdown description"`
}

// CreateComponent creates a component (owner role, FR-79).
func (s *Session) CreateComponent(ctx context.Context, in NewComponent) (*ComponentView, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("a name is required")
	}
	c, err := s.C.CreateComponent(ctx, s.ProductID, client.ComponentWrite{Name: &name, Description: &in.Description})
	if err != nil {
		return nil, err
	}
	s.forget("components")
	return s.componentView(ctx, c.ID)
}

// ComponentChange updates a component; omitted fields stay as they are.
type ComponentChange struct {
	Ref         string  `json:"ref" jsonschema:"the component: name or ID"`
	Name        *string `json:"name,omitempty" jsonschema:"new name"`
	Description *string `json:"description,omitempty" jsonschema:"new Markdown description; empty clears it"`
}

// UpdateComponent changes a component (owner role, FR-79).
func (s *Session) UpdateComponent(ctx context.Context, in ComponentChange) (*ComponentView, error) {
	if in.Name == nil && in.Description == nil {
		return nil, errors.New("nothing to change: give a new name or description")
	}
	if in.Name != nil {
		if *in.Name = strings.TrimSpace(*in.Name); *in.Name == "" {
			return nil, errors.New("the name cannot be empty")
		}
	}
	c, err := s.findComponent(ctx, in.Ref)
	if err != nil {
		return nil, err
	}
	if _, err := s.C.UpdateComponent(ctx, s.ProductID, c.ID, client.ComponentWrite{Name: in.Name, Description: in.Description}); err != nil {
		return nil, err
	}
	s.forget("components")
	return s.componentView(ctx, c.ID)
}

// DeleteComponent deletes a component (owner role). Its capabilities stay,
// without a component. Not offered via MCP (FR-136).
func (s *Session) DeleteComponent(ctx context.Context, ref string) (*Ref, error) {
	c, err := s.findComponent(ctx, ref)
	if err != nil {
		return nil, err
	}
	if err := s.C.DeleteComponent(ctx, s.ProductID, c.ID); err != nil {
		return nil, err
	}
	s.forget("components", "capabilities")
	return &Ref{ID: c.ID, Title: c.Name}, nil
}

// NewCapability creates a capability.
type NewCapability struct {
	Name        string `json:"name" jsonschema:"capability name, unique in the product"`
	Component   string `json:"component,omitempty" jsonschema:"component (name or ID) the capability belongs to; omit for a product-level capability"`
	Description string `json:"description,omitempty" jsonschema:"Markdown description"`
}

// CreateCapability creates a capability (owner role, FR-79).
func (s *Session) CreateCapability(ctx context.Context, in NewCapability) (*CapabilityDetail, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("a name is required")
	}
	body := client.CapabilityWrite{Name: &name, Description: &in.Description}
	if strings.TrimSpace(in.Component) != "" {
		comp, err := s.findComponent(ctx, in.Component)
		if err != nil {
			return nil, err
		}
		body.ComponentID = &comp.ID
	}
	c, err := s.C.CreateCapability(ctx, s.ProductID, body)
	if err != nil {
		return nil, err
	}
	s.forget("capabilities")
	return s.ShowCapability(ctx, c.ID)
}

// CapabilityChange updates a capability; omitted fields stay as they are.
type CapabilityChange struct {
	Ref         string  `json:"ref" jsonschema:"the capability: name or ID"`
	Name        *string `json:"name,omitempty" jsonschema:"new name"`
	Component   *string `json:"component,omitempty" jsonschema:"component (name or ID) to move it to; empty makes it a product-level capability"`
	Description *string `json:"description,omitempty" jsonschema:"new Markdown description; empty clears it"`
}

// UpdateCapability changes a capability (owner role, FR-79).
func (s *Session) UpdateCapability(ctx context.Context, in CapabilityChange) (*CapabilityDetail, error) {
	if in.Name == nil && in.Component == nil && in.Description == nil {
		return nil, errors.New("nothing to change: give a new name, component or description")
	}
	if in.Name != nil {
		if *in.Name = strings.TrimSpace(*in.Name); *in.Name == "" {
			return nil, errors.New("the name cannot be empty")
		}
	}
	c, err := s.findCapability(ctx, in.Ref)
	if err != nil {
		return nil, err
	}
	body := client.CapabilityWrite{Name: in.Name, Description: in.Description}
	if in.Component != nil {
		id := ""
		if strings.TrimSpace(*in.Component) != "" {
			comp, err := s.findComponent(ctx, *in.Component)
			if err != nil {
				return nil, err
			}
			id = comp.ID
		}
		body.ComponentID = &id
	}
	if _, err := s.C.UpdateCapability(ctx, s.ProductID, c.ID, body); err != nil {
		return nil, err
	}
	s.forget("capabilities")
	return s.ShowCapability(ctx, c.ID)
}

// DeleteCapability deletes a capability (owner role). Not offered via MCP
// (FR-136).
func (s *Session) DeleteCapability(ctx context.Context, ref string) (*Ref, error) {
	c, err := s.findCapability(ctx, ref)
	if err != nil {
		return nil, err
	}
	if err := s.C.DeleteCapability(ctx, s.ProductID, c.ID); err != nil {
		return nil, err
	}
	s.forget("capabilities")
	return &Ref{ID: c.ID, Title: c.Name}, nil
}

// CapabilityLinks links requirements and constraints to a capability, or
// unlinks them.
type CapabilityLinks struct {
	Capability string   `json:"capability" jsonschema:"the capability: name or ID"`
	Items      []string `json:"items" jsonschema:"identifiers of requirements (FR-12) and constraints (AC-3)"`
	// Unlink removes the links instead; set by the unlink tool and command.
	Unlink bool `json:"-"`
}

// LinkCapability links (or unlinks) requirements and constraints to a
// capability (owner role; FR-6, FR-44). Every item is resolved before
// the first change, so a bad identifier changes nothing.
func (s *Session) LinkCapability(ctx context.Context, in CapabilityLinks) (*CapabilityDetail, error) {
	if len(in.Items) == 0 {
		return nil, errors.New("name at least one requirement or constraint, e.g. FR-12")
	}
	c, err := s.findCapability(ctx, in.Capability)
	if err != nil {
		return nil, err
	}
	type target struct {
		constraint bool
		id         string
	}
	targets := make([]target, 0, len(in.Items))
	for _, ref := range in.Items {
		ref = strings.TrimSpace(ref)
		if !identifierPattern.MatchString(ref) {
			return nil, fmt.Errorf("%q is not an identifier like FR-12 or AC-3", ref)
		}
		r, err := s.C.Requirement(ctx, s.ProductID, ref)
		if err == nil {
			if r.Kind == "task" {
				return nil, fmt.Errorf("%s is a task; only requirements and constraints link to capabilities", r.Identifier)
			}
			targets = append(targets, target{false, r.ID})
			continue
		}
		if !client.IsNotFound(err) {
			return nil, err
		}
		con, err := s.C.Constraint(ctx, s.ProductID, ref)
		if err != nil {
			if client.IsNotFound(err) {
				return nil, &NotFoundError{Kind: "requirement or constraint", Ref: ref}
			}
			return nil, err
		}
		targets = append(targets, target{true, con.ID})
	}
	for _, t := range targets {
		switch {
		case t.constraint && in.Unlink:
			err = s.C.UnlinkConstraintCapability(ctx, s.ProductID, t.id, c.ID)
		case t.constraint:
			err = s.C.LinkConstraintCapability(ctx, s.ProductID, t.id, c.ID)
		case in.Unlink:
			err = s.C.UnlinkCapabilityRequirement(ctx, s.ProductID, c.ID, t.id)
		default:
			err = s.C.LinkCapabilityRequirement(ctx, s.ProductID, c.ID, t.id)
		}
		if err != nil {
			return nil, err
		}
	}
	return s.ShowCapability(ctx, c.ID)
}

// TestCaseLinks links a test case to a requirement, constraints, a
// capability or a component, or removes those links.
type TestCaseLinks struct {
	TestCase    string   `json:"test_case" jsonschema:"the test case: test identifier, short ID or ID"`
	Requirement string   `json:"requirement,omitempty" jsonschema:"requirement the test case verifies, e.g. FR-12"`
	Constraints []string `json:"constraints,omitempty" jsonschema:"constraints the test case verifies, e.g. SC-30"`
	Capability  string   `json:"capability,omitempty" jsonschema:"capability (name or ID) the test case belongs to; replaces its current one"`
	Component   string   `json:"component,omitempty" jsonschema:"component (name or ID) the test case belongs to; replaces its current one"`
	// Unlink removes the named links instead; set by the unlink tool and
	// command. A capability or component is only cleared if it is the
	// test case's current one.
	Unlink bool `json:"-"`
}

// LinkTestCase links (or unlinks) a test case (developer role; FR-21,
// FR-22, FR-150).
func (s *Session) LinkTestCase(ctx context.Context, in TestCaseLinks) (*TestCaseDetail, error) {
	if in.Requirement == "" && len(in.Constraints) == 0 && in.Capability == "" && in.Component == "" {
		return nil, errors.New("name a requirement, constraint, capability or component")
	}
	tc, err := s.findTestCase(ctx, in.TestCase)
	if err != nil {
		return nil, err
	}
	reqID := ""
	if in.Requirement != "" {
		r, err := s.requirement(ctx, in.Requirement, "requirement")
		if err != nil {
			return nil, err
		}
		reqID = r.ID
	}
	constraintIDs := make([]string, 0, len(in.Constraints))
	for _, ref := range in.Constraints {
		con, err := s.constraint(ctx, ref)
		if err != nil {
			return nil, err
		}
		constraintIDs = append(constraintIDs, con.ID)
	}
	var patch client.TestCasePatch
	if in.Capability != "" {
		c, err := s.findCapability(ctx, in.Capability)
		if err != nil {
			return nil, err
		}
		if in.Unlink {
			if tc.CapabilityID != c.ID {
				return nil, fmt.Errorf("test case is not linked to capability %q", c.Name)
			}
			c.ID = ""
		}
		patch.CapabilityID = &c.ID
	}
	if in.Component != "" {
		c, err := s.findComponent(ctx, in.Component)
		if err != nil {
			return nil, err
		}
		if in.Unlink {
			if tc.ComponentID != c.ID {
				return nil, fmt.Errorf("test case is not linked to component %q", c.Name)
			}
			c.ID = ""
		}
		patch.ComponentID = &c.ID
	}

	if reqID != "" {
		if in.Unlink {
			err = s.C.UnlinkTestCaseRequirement(ctx, s.ProductID, tc.ID, reqID)
		} else {
			err = s.C.LinkTestCaseRequirement(ctx, s.ProductID, tc.ID, reqID)
		}
		if err != nil {
			return nil, err
		}
	}
	for _, id := range constraintIDs {
		if in.Unlink {
			err = s.C.UnlinkTestCaseConstraint(ctx, s.ProductID, tc.ID, id)
		} else {
			err = s.C.LinkTestCaseConstraint(ctx, s.ProductID, tc.ID, id)
		}
		if err != nil {
			return nil, err
		}
	}
	if patch.CapabilityID != nil || patch.ComponentID != nil {
		if _, err := s.C.UpdateTestCase(ctx, s.ProductID, tc.ID, patch); err != nil {
			return nil, err
		}
	}
	return s.ShowTestCase(ctx, tc.ID)
}

var complianceStates = []string{"not_assessed", "met", "violated"}

// normalizeCompliance accepts "met", "Violated", "not assessed" and the like.
func normalizeCompliance(v string) (string, error) {
	v = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(v)), " ", "_")
	if !contains(complianceStates, v) {
		return "", fmt.Errorf("compliance must be one of %s, not %q", strings.Join(complianceStates, ", "), v)
	}
	return v, nil
}

// NewConstraint creates a constraint.
type NewConstraint struct {
	Title       string `json:"title" jsonschema:"the rule, as a short statement"`
	Category    string `json:"category,omitempty" jsonschema:"constraint category prefix (e.g. AC) or name; omit for the product's default"`
	Description string `json:"description,omitempty" jsonschema:"Markdown description"`
	Compliance  string `json:"compliance,omitempty" jsonschema:"not_assessed (default), met or violated"`
}

// CreateConstraint creates a constraint (owner role, FR-104, FR-135).
func (s *Session) CreateConstraint(ctx context.Context, in NewConstraint) (*ConstraintDetail, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("a title is required")
	}
	body := client.ConstraintCreate{Title: strings.TrimSpace(in.Title), Description: in.Description}
	var err error
	if in.Category != "" {
		if body.CategoryID, err = s.categoryID(ctx, strings.TrimSpace(in.Category), "constraint"); err != nil {
			return nil, err
		}
	}
	if in.Compliance != "" {
		if body.ComplianceState, err = normalizeCompliance(in.Compliance); err != nil {
			return nil, err
		}
	}
	c, err := s.C.CreateConstraint(ctx, s.ProductID, body)
	if err != nil {
		return nil, err
	}
	return s.ShowConstraint(ctx, c.ID)
}

// ConstraintChange updates a constraint; omitted fields stay as they are.
type ConstraintChange struct {
	Ref         string  `json:"ref" jsonschema:"the constraint: identifier (AC-3) or ID"`
	Title       *string `json:"title,omitempty" jsonschema:"new title"`
	Category    *string `json:"category,omitempty" jsonschema:"new category prefix or name; the identifier changes with it"`
	Description *string `json:"description,omitempty" jsonschema:"new Markdown description; empty clears it"`
	Compliance  *string `json:"compliance,omitempty" jsonschema:"not_assessed, met or violated (FR-43)"`
}

// UpdateConstraint changes a constraint (owner role, FR-104, FR-135).
func (s *Session) UpdateConstraint(ctx context.Context, in ConstraintChange) (*ConstraintDetail, error) {
	if in.Title == nil && in.Category == nil && in.Description == nil && in.Compliance == nil {
		return nil, errors.New("nothing to change: give a title, category, description or compliance state")
	}
	var patch client.ConstraintPatch
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return nil, errors.New("the title cannot be empty")
		}
		patch.Title = &t
	}
	patch.Description = in.Description
	if in.Category != nil {
		id, err := s.categoryID(ctx, strings.TrimSpace(*in.Category), "constraint")
		if err != nil {
			return nil, err
		}
		patch.CategoryID = &id
	}
	if in.Compliance != nil {
		v, err := normalizeCompliance(*in.Compliance)
		if err != nil {
			return nil, err
		}
		patch.ComplianceState = &v
	}
	c, err := s.C.UpdateConstraint(ctx, s.ProductID, strings.TrimSpace(in.Ref), patch)
	if err != nil {
		if client.IsNotFound(err) {
			return nil, &NotFoundError{Kind: "constraint", Ref: in.Ref}
		}
		return nil, err
	}
	return s.ShowConstraint(ctx, c.ID)
}
