// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"wiekko.net/internal/client"
)

// Structure returns the component → capability tree.
func (s *Session) Structure(ctx context.Context) (*Structure, error) {
	comps, err := s.components(ctx)
	if err != nil {
		return nil, err
	}
	caps, err := s.capabilities(ctx)
	if err != nil {
		return nil, err
	}
	out := &Structure{Components: []ComponentView{}, ProductLevel: []CapabilityView{}}
	index := map[string]int{}
	for _, c := range comps {
		index[c.ID] = len(out.Components)
		out.Components = append(out.Components, ComponentView{ID: c.ID, Name: c.Name, Description: c.Description, Capabilities: []CapabilityView{}})
	}
	for _, c := range caps {
		v := CapabilityView{ID: c.ID, Name: c.Name, Description: c.Description}
		if i, ok := index[c.ComponentID]; ok {
			out.Components[i].Capabilities = append(out.Components[i].Capabilities, v)
		} else {
			out.ProductLevel = append(out.ProductLevel, v)
		}
	}
	return out, nil
}

// ShowCapability returns a capability by name (case-insensitive), ID or ID
// prefix, with the requirements, constraints (DF-1), test cases and defects
// traced to it.
func (s *Session) ShowCapability(ctx context.Context, ref string) (*CapabilityDetail, error) {
	c, err := s.findCapability(ctx, ref)
	if err != nil {
		return nil, err
	}
	d := &CapabilityDetail{
		CapabilityView: CapabilityView{ID: c.ID, Name: c.Name, Description: c.Description},
		Requirements:   []RequirementSummary{},
	}
	if c.ComponentID != "" {
		comps, err := s.componentNames(ctx)
		if err != nil {
			return nil, err
		}
		d.Component = comps[c.ComponentID]
	}

	links, err := s.C.CapabilityRequirementLinks(ctx, s.ProductID, c.ID)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		r, err := s.C.Requirement(ctx, s.ProductID, l.RequirementID)
		if err != nil {
			if client.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		sum, err := s.requirementSummary(ctx, *r)
		if err != nil {
			return nil, err
		}
		d.Requirements = append(d.Requirements, sum)
	}
	cons, err := s.C.CapabilityConstraints(ctx, s.ProductID, c.ID)
	if err != nil {
		return nil, err
	}
	d.Constraints = make([]ConstraintSummary, 0, len(cons))
	for _, con := range cons {
		sum, err := s.constraintSummary(ctx, con)
		if err != nil {
			return nil, err
		}
		d.Constraints = append(d.Constraints, sum)
	}
	tcs, err := s.C.CapabilityTestCases(ctx, s.ProductID, c.ID)
	if err != nil {
		return nil, err
	}
	if d.TestCases, err = s.testCaseSummaries(ctx, tcs); err != nil {
		return nil, err
	}
	defects, err := s.C.CapabilityDefects(ctx, s.ProductID, c.ID)
	if err != nil {
		return nil, err
	}
	if d.Defects, err = s.defectSummaries(ctx, defects); err != nil {
		return nil, err
	}
	if d.Comments, err = s.comments(ctx, "capability", c.ID); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Session) findCapability(ctx context.Context, ref string) (client.Capability, error) {
	caps, err := s.capabilities(ctx)
	if err != nil {
		return client.Capability{}, err
	}
	return match("capability", ref, caps,
		func(c client.Capability) string { return c.ID },
		func(c client.Capability) string { return c.Name },
		func(c client.Capability) bool { return strings.EqualFold(c.Name, strings.TrimSpace(ref)) })
}

// SpecificationQuery selects what the specification export contains.
type SpecificationQuery struct {
	Tasks bool `json:"tasks,omitempty" jsonschema:"include tasks; by default only the structure, documents, constraints and requirements are exported"`
	// Drafts lists draft documents besides the current ones.
	Drafts bool `json:"drafts,omitempty" jsonschema:"include draft documents; by default only current ones are listed"`
}

// SpecificationView describes an exported specification. The document
// itself travels separately (stdout, a file, the MCP text content), so a
// large specification is not duplicated in structured output.
type SpecificationView struct {
	ProductID string `json:"product_id"`
	Tasks     bool   `json:"tasks"`
	Drafts    bool   `json:"drafts,omitempty"`
	Bytes     int    `json:"bytes"`
	// SHA256 is the document's hash (hex), so a caller can tell whether a
	// committed export is still current without comparing the text.
	SHA256 string `json:"sha256"`
}

// Specification exports the product's specification as Markdown (FR-137):
// the structure, constraints and requirements, and tasks if asked for. The
// server renders it, in a stable order and without timestamps.
func (s *Session) Specification(ctx context.Context, q SpecificationQuery) (string, *SpecificationView, error) {
	doc, err := s.C.Specification(ctx, s.ProductID, q.Tasks, q.Drafts)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256([]byte(doc))
	return doc, &SpecificationView{ProductID: s.ProductID, Tasks: q.Tasks, Drafts: q.Drafts, Bytes: len(doc), SHA256: hex.EncodeToString(sum[:])}, nil
}
