// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ops

import (
	"context"
	"fmt"
	"strings"

	"wiekko.net/internal/client"
)

// RequirementQuery filters ListRequirements. Empty fields do not filter.
//
// The json and jsonschema tags define the input schema of the MCP tool built
// on this operation, so CLI flags and MCP arguments cannot drift apart.
type RequirementQuery struct {
	Kind     string `json:"kind,omitempty" jsonschema:"requirement or task; omit for both"`
	Status   string `json:"status,omitempty" jsonschema:"status name (case-insensitive)"`
	Category string `json:"category,omitempty" jsonschema:"category prefix (e.g. FR) or name"`
	Sprint   string `json:"sprint,omitempty" jsonschema:"sprint name or ID, or 'current' for the active sprint"`
	Search   string `json:"search,omitempty" jsonschema:"case-insensitive text in title or identifier"`
	Open     bool   `json:"open,omitempty" jsonschema:"only items whose status is not a closed one"`
	Active   bool   `json:"active,omitempty" jsonschema:"only items activated for work"`
	Coarse   bool   `json:"coarse,omitempty" jsonschema:"only items marked as needing refinement"`
	Archived string `json:"archived,omitempty" jsonschema:"include or only to list archived items too or only them; by default they are left out"`
	Limit    int    `json:"limit,omitempty" jsonschema:"return at most this many items; 0 for all"`
}

// ListRequirements lists requirements and tasks.
func (s *Session) ListRequirements(ctx context.Context, q RequirementQuery) ([]RequirementSummary, error) {
	f := client.RequirementFilter{Kind: q.Kind}
	if err := checkArchived(q.Archived); err != nil {
		return nil, err
	}
	f.Archived = q.Archived
	if q.Kind != "" && q.Kind != "requirement" && q.Kind != "task" {
		return nil, fmt.Errorf("unknown kind %q (want requirement or task)", q.Kind)
	}
	if q.Category != "" {
		id, err := s.categoryID(ctx, q.Category, "requirement", "task")
		if err != nil {
			return nil, err
		}
		f.CategoryID = id
	}
	if q.Status != "" {
		id, err := s.statusID(ctx, "requirement", q.Status)
		if err != nil {
			return nil, err
		}
		f.StatusID = id
	}
	if q.Sprint != "" {
		id, err := s.sprintID(ctx, q.Sprint)
		if err != nil {
			return nil, err
		}
		f.SprintID = id
	}

	items, err := s.C.Requirements(ctx, s.ProductID, f)
	if err != nil {
		return nil, err
	}
	out := make([]RequirementSummary, 0, len(items))
	for _, it := range items {
		sum, err := s.requirementSummary(ctx, it)
		if err != nil {
			return nil, err
		}
		if q.Open && sum.StatusClosed || q.Active && !sum.Active || q.Coarse && !sum.Coarse {
			continue
		}
		if q.Search != "" && !containsFold(sum.Title, q.Search) && !containsFold(sum.Identifier, q.Search) {
			continue
		}
		out = append(out, sum)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

// ShowRequirement returns one requirement or task by identifier ("FR-12") or ID.
func (s *Session) ShowRequirement(ctx context.Context, ref string) (*RequirementDetail, error) {
	r, err := s.C.Requirement(ctx, s.ProductID, ref)
	if err != nil {
		if client.IsNotFound(err) {
			return nil, &NotFoundError{Kind: "requirement or task", Ref: ref}
		}
		return nil, err
	}
	sum, err := s.requirementSummary(ctx, *r)
	if err != nil {
		return nil, err
	}
	d := &RequirementDetail{
		RequirementSummary: sum,
		Description:        r.Description,
		DefinitionOfDone:   r.DefinitionOfDone,
		Children:           []RequirementSummary{},
		Assignees:          []string{},
		Capabilities:       []Ref{},
		TestCases:          []TestCaseSummary{},
		Defects:            []DefectSummary{},
		Documents:          documentRefs(r.Documents),
	}

	if r.MilestoneID != "" {
		ms, err := s.milestoneNames(ctx)
		if err != nil {
			return nil, err
		}
		d.Milestone = ms[r.MilestoneID]
	}
	if r.ParentID != "" {
		if p, err := s.C.Requirement(ctx, s.ProductID, r.ParentID); err == nil {
			d.Parent = &Ref{ID: p.ID, Identifier: p.Identifier, Title: p.Title}
		} else if !client.IsNotFound(err) {
			return nil, err
		}
	}

	children, err := s.C.RequirementChildren(ctx, s.ProductID, r.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range children {
		cs, err := s.requirementSummary(ctx, c)
		if err != nil {
			return nil, err
		}
		d.Children = append(d.Children, cs)
	}

	if d.Assignees, err = s.assignees(ctx, func() ([]client.Assignment, error) {
		return s.C.RequirementAssignees(ctx, s.ProductID, r.ID)
	}); err != nil {
		return nil, err
	}

	// Tasks take no part in traceability (FR-13): no capabilities, tests.
	if r.Kind != "task" {
		links, err := s.C.RequirementCapabilityLinks(ctx, s.ProductID, r.ID)
		if err != nil {
			return nil, err
		}
		capNames, err := s.capabilityNames(ctx)
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			d.Capabilities = append(d.Capabilities, Ref{ID: l.CapabilityID, Title: capNames[l.CapabilityID]})
		}
		tcs, err := s.C.RequirementTestCases(ctx, s.ProductID, r.ID)
		if err != nil {
			return nil, err
		}
		if d.TestCases, err = s.testCaseSummaries(ctx, tcs); err != nil {
			return nil, err
		}
	}

	defects, err := s.C.RequirementDefects(ctx, s.ProductID, r.ID)
	if err != nil {
		return nil, err
	}
	if d.Defects, err = s.defectSummaries(ctx, defects); err != nil {
		return nil, err
	}

	ownerType := "requirement"
	if r.Kind == "task" {
		ownerType = "task"
	}
	if d.Comments, err = s.comments(ctx, ownerType, r.ID); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Session) requirementSummary(ctx context.Context, r client.Requirement) (RequirementSummary, error) {
	statuses, err := s.statuses(ctx)
	if err != nil {
		return RequirementSummary{}, err
	}
	cats, err := s.categories(ctx)
	if err != nil {
		return RequirementSummary{}, err
	}
	sum := RequirementSummary{
		ID:         r.ID,
		Identifier: r.Identifier,
		Kind:       r.Kind,
		Title:      r.Title,
		Category:   cats[r.CategoryID].Name,
		Priority:   r.Priority,
		Active:     r.IsActive,
		Archived:   r.Archived,
		Coarse:     r.IsCoarse,
		Estimation: r.Estimation,
	}
	if st, ok := statuses[r.StatusID]; ok {
		sum.Status, sum.StatusClosed = st.Name, st.IsClosed
	}
	if r.SprintID != "" {
		sprints, err := s.sprintNames(ctx)
		if err != nil {
			return RequirementSummary{}, err
		}
		sum.Sprint = sprints[r.SprintID]
	}
	return sum, nil
}

// ConstraintQuery filters ListConstraints.
type ConstraintQuery struct {
	Category   string `json:"category,omitempty" jsonschema:"category prefix (e.g. AC) or name"`
	Compliance string `json:"compliance,omitempty" jsonschema:"not_assessed, met or violated"`
	Search     string `json:"search,omitempty" jsonschema:"case-insensitive text in title or identifier"`
	Limit      int    `json:"limit,omitempty" jsonschema:"return at most this many items; 0 for all"`
}

// ListConstraints lists constraints.
func (s *Session) ListConstraints(ctx context.Context, q ConstraintQuery) ([]ConstraintSummary, error) {
	items, err := s.C.Constraints(ctx, s.ProductID)
	if err != nil {
		return nil, err
	}
	var catID string
	if q.Category != "" {
		if catID, err = s.categoryID(ctx, q.Category, "constraint"); err != nil {
			return nil, err
		}
	}
	if q.Compliance != "" {
		switch q.Compliance {
		case "not_assessed", "met", "violated":
		default:
			return nil, fmt.Errorf("unknown compliance state %q (want not_assessed, met or violated)", q.Compliance)
		}
	}
	out := make([]ConstraintSummary, 0, len(items))
	for _, it := range items {
		if catID != "" && it.CategoryID != catID || q.Compliance != "" && it.ComplianceState != q.Compliance {
			continue
		}
		if q.Search != "" && !containsFold(it.Title, q.Search) && !containsFold(it.Identifier, q.Search) {
			continue
		}
		sum, err := s.constraintSummary(ctx, it)
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

// ShowConstraint returns one constraint by identifier ("AC-42") or ID.
func (s *Session) ShowConstraint(ctx context.Context, ref string) (*ConstraintDetail, error) {
	c, err := s.constraint(ctx, ref)
	if err != nil {
		return nil, err
	}
	sum, err := s.constraintSummary(ctx, *c)
	if err != nil {
		return nil, err
	}
	d := &ConstraintDetail{ConstraintSummary: sum, Description: c.Description, Capabilities: []Ref{}, TestCases: []EvidenceView{}, Documents: documentRefs(c.Documents)}
	if c.MilestoneID != "" {
		ms, err := s.milestoneNames(ctx)
		if err != nil {
			return nil, err
		}
		d.Milestone = ms[c.MilestoneID]
	}
	links, err := s.C.ConstraintCapabilityLinks(ctx, s.ProductID, c.ID)
	if err != nil {
		return nil, err
	}
	capNames, err := s.capabilityNames(ctx)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		d.Capabilities = append(d.Capabilities, Ref{ID: l.CapabilityID, Title: capNames[l.CapabilityID]})
	}
	if d.TestCases, err = s.constraintTestCases(ctx, c.ID); err != nil {
		return nil, err
	}
	if d.Comments, err = s.comments(ctx, "constraint", c.ID); err != nil {
		return nil, err
	}
	return d, nil
}

// constraint finds a constraint by identifier ("SC-30") or ID.
func (s *Session) constraint(ctx context.Context, ref string) (*client.Constraint, error) {
	c, err := s.C.Constraint(ctx, s.ProductID, strings.TrimSpace(ref))
	if err != nil {
		if client.IsNotFound(err) {
			return nil, &NotFoundError{Kind: "constraint", Ref: ref}
		}
		return nil, err
	}
	return c, nil
}

// constraintTestCases lists the test cases of a constraint with their latest
// results (FR-150).
func (s *Session) constraintTestCases(ctx context.Context, constraintID string) ([]EvidenceView, error) {
	items, err := s.C.ConstraintTestCases(ctx, s.ProductID, constraintID)
	if err != nil {
		return nil, err
	}
	cases := make([]client.TestCase, len(items))
	for i, item := range items {
		cases[i] = item.TestCase
	}
	sums, err := s.testCaseSummaries(ctx, cases)
	if err != nil {
		return nil, err
	}
	out := make([]EvidenceView, len(items))
	for i, item := range items {
		out[i] = EvidenceView{TestCaseSummary: sums[i]}
		if item.LatestResult != nil {
			out[i].LatestResult, out[i].LatestAt = item.LatestResult.Status, item.LatestResult.ExecutedAt
		}
		for _, via := range item.Via {
			out[i].Via = append(out[i].Via, via.Name)
		}
	}
	return out, nil
}

func (s *Session) constraintSummary(ctx context.Context, c client.Constraint) (ConstraintSummary, error) {
	cats, err := s.categories(ctx)
	if err != nil {
		return ConstraintSummary{}, err
	}
	return ConstraintSummary{
		ID:         c.ID,
		Identifier: c.Identifier,
		Title:      c.Title,
		Category:   cats[c.CategoryID].Name,
		Compliance: c.ComplianceState,
		Priority:   c.Priority,
	}, nil
}

// comments loads an entity's comments with author names, oldest first.
func (s *Session) comments(ctx context.Context, ownerType, ownerID string) ([]CommentView, error) {
	list, err := s.C.Comments(ctx, s.ProductID, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	users, err := s.usernames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CommentView, 0, len(list))
	for _, c := range list {
		out = append(out, CommentView{Author: author(users, c.UserID), CreatedAt: c.CreatedAt, Body: c.Body})
	}
	return out, nil
}

func (s *Session) assignees(ctx context.Context, load func() ([]client.Assignment, error)) ([]string, error) {
	list, err := load()
	if err != nil {
		return nil, err
	}
	users, err := s.usernames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, user(users, a.UserID))
	}
	return out, nil
}

// categoryID resolves a category prefix or name among the given entity types.
func (s *Session) categoryID(ctx context.Context, ref string, entityTypes ...string) (string, error) {
	cats, err := s.categories(ctx)
	if err != nil {
		return "", err
	}
	var known []string
	for _, c := range cats {
		if !contains(entityTypes, c.EntityType) {
			continue
		}
		if strings.EqualFold(c.Prefix, ref) || strings.EqualFold(c.Name, ref) {
			return c.ID, nil
		}
		known = append(known, c.Prefix+" ("+c.Name+")")
	}
	return "", fmt.Errorf("unknown category %q; known: %s", ref, strings.Join(known, ", "))
}

// statusID resolves a status name for an entity type.
func (s *Session) statusID(ctx context.Context, entityType, name string) (string, error) {
	list, err := s.workflow(ctx, entityType)
	if err != nil {
		return "", err
	}
	var known []string
	for _, st := range list {
		if strings.EqualFold(st.Name, name) {
			return st.ID, nil
		}
		known = append(known, st.Name)
	}
	return "", fmt.Errorf("unknown status %q; known: %s", name, strings.Join(known, ", "))
}

// sprintID resolves "current", a sprint name, or an ID.
func (s *Session) sprintID(ctx context.Context, ref string) (string, error) {
	list, err := s.sprints(ctx)
	if err != nil {
		return "", err
	}
	for _, sp := range list {
		if strings.EqualFold(ref, "current") && sp.Status == "active" ||
			sp.ID == ref || strings.EqualFold(sp.Name, ref) {
			return sp.ID, nil
		}
	}
	if strings.EqualFold(ref, "current") {
		return "", fmt.Errorf("no sprint is active")
	}
	return "", &NotFoundError{Kind: "sprint", Ref: ref}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
