package controller

import (
	"net/http"
	"sort"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	productpage "nexmap.net/web/templates/pages/product"
)

// Derived links (UR-160) follow the product structure one step along
// component → capability → requirement → task. They are computed here from
// the stored links, shown read-only, and never stored themselves.

// maxAncestorSteps bounds the walk from a task up to its requirement; the
// backlog service refuses deeper nesting anyway.
const maxAncestorSteps = 100

// derivedComponents groups a requirement's capabilities by component.
// Product-level capabilities have no component and add nothing.
func (c *PageProductController) derivedComponents(r *http.Request, user *model.User, productID string, caps []*model.Capability) []productpage.DerivedComponent {
	if len(caps) == 0 {
		return nil
	}
	comps, _ := c.component.ListComponents(r.Context(), user, productID)
	byID := make(map[string]*model.Component, len(comps))
	for _, comp := range comps {
		byID[comp.ID] = comp
	}
	index := map[string]int{}
	var out []productpage.DerivedComponent
	for _, cap := range caps {
		comp, ok := byID[cap.ComponentID]
		if !ok {
			continue
		}
		i, seen := index[comp.ID]
		if !seen {
			i = len(out)
			index[comp.ID] = i
			out = append(out, productpage.DerivedComponent{Component: comp})
		}
		out[i].Via = append(out[i].Via, cap)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Component.Name < out[b].Component.Name })
	return out
}

// requirementComponents returns the components a requirement reaches through
// its capabilities.
func (c *PageProductController) requirementComponents(r *http.Request, user *model.User, productID, requirementID string) []productpage.DerivedComponent {
	caps, _ := c.component.ListLinkedCapabilities(r.Context(), user, productID, requirementID)
	return c.derivedComponents(r, user, productID, caps)
}

// inheritedCapabilities returns the capabilities of the requirement a task
// refines: its nearest requirement ancestor, as tasks may hang below tasks.
// Tasks themselves cannot be linked to capabilities.
func (c *PageProductController) inheritedCapabilities(r *http.Request, user *model.User, productID string, task *model.Requirement) productpage.InheritedCapabilities {
	parentID := task.ParentID
	for step := 0; parentID != "" && step < maxAncestorSteps; step++ {
		parent, err := c.backlog.GetRequirement(r.Context(), user, productID, parentID)
		if err != nil {
			return productpage.InheritedCapabilities{}
		}
		if parent.Kind != model.RequirementKindRequirement {
			parentID = parent.ParentID
			continue
		}
		caps, _ := c.component.ListLinkedCapabilities(r.Context(), user, productID, parent.ID)
		names := map[string]string{}
		for _, d := range c.derivedComponents(r, user, productID, caps) {
			for _, cap := range d.Via {
				names[cap.ID] = d.Component.Name
			}
		}
		prefixes := c.buildBacklogPrefixMap(r, user, productID)
		return productpage.InheritedCapabilities{
			SourceIdentifier: model.EntityIdentifier(categoryPrefix(prefixes, parent.CategoryID), parent.SequenceNumber),
			SourcePath:       backlogItemPath(productID, parent),
			Capabilities:     caps,
			ComponentNames:   names,
		}
	}
	return productpage.InheritedCapabilities{}
}

// componentRequirements returns the requirements linked to a component's
// capabilities, each with the capabilities it comes through.
func (c *PageProductController) componentRequirements(r *http.Request, user *model.User, productID, componentID string) []productpage.DerivedRequirement {
	caps, _ := c.component.ListCapabilitiesByComponent(r.Context(), user, productID, componentID)
	prefixes := c.buildBacklogPrefixMap(r, user, productID)
	index := map[string]int{}
	var out []productpage.DerivedRequirement
	for _, cap := range caps {
		links, _ := c.component.ListCapabilityRequirements(r.Context(), user, productID, cap.ID)
		for _, link := range links {
			i, seen := index[link.RequirementID]
			if !seen {
				req, err := c.backlog.GetRequirement(r.Context(), user, productID, link.RequirementID)
				if err != nil {
					continue
				}
				i = len(out)
				index[req.ID] = i
				out = append(out, productpage.DerivedRequirement{
					Requirement: req,
					Identifier:  model.EntityIdentifier(categoryPrefix(prefixes, req.CategoryID), req.SequenceNumber),
				})
			}
			out[i].Via = append(out[i].Via, cap)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		return out[a].Requirement.SequenceNumber < out[b].Requirement.SequenceNumber
	})
	return out
}

// componentDefectsViaCapabilities returns the defects linked to a
// component's capabilities. A defect also linked to the component itself is
// left out: it is listed once, as a direct link.
func (c *PageProductController) componentDefectsViaCapabilities(r *http.Request, user *model.User, productID, componentID string, direct []*model.Defect) []productpage.DerivedDefect {
	skip := make(map[string]bool, len(direct))
	for _, d := range direct {
		skip[d.ID] = true
	}
	caps, _ := c.component.ListCapabilitiesByComponent(r.Context(), user, productID, componentID)
	index := map[string]int{}
	var out []productpage.DerivedDefect
	for _, cap := range caps {
		defects, _, _ := c.backlog.ListLinkedDefects(r.Context(), user, productID, model.DefectLinkTargetCapability, cap.ID)
		for _, d := range defects {
			if skip[d.ID] {
				continue
			}
			i, seen := index[d.ID]
			if !seen {
				i = len(out)
				index[d.ID] = i
				out = append(out, productpage.DerivedDefect{Defect: d})
			}
			out[i].Via = append(out[i].Via, cap)
		}
	}
	return out
}

// ComponentRequirementsTab renders GET /products/{id}/components/{componentId}/tabs/requirements:
// the requirements of the component's capabilities, read-only.
func (c *PageProductController) ComponentRequirementsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	if _, err := c.component.GetComponent(r.Context(), user, productID, componentID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentRequirementsTabContent(productpage.ComponentRequirementsTabProps{
		ProductID:    productID,
		Requirements: c.componentRequirements(r, user, productID, componentID),
	}).Render(r.Context(), w)
}
