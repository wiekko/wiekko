package ops

import (
	"context"
	"strings"

	"nexmap.net/internal/client"
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
// prefix, with the requirements, test cases and defects traced to it.
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
