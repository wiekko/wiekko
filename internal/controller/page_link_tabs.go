package controller

import (
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	productpage "nexmap.net/web/templates/pages/product"
)

// Detail pages place links by how many items they point to (UC-88): what an
// item belongs to is an Overview field, everything it lists has a tab of its
// own. Tabs of things people add (links, comments, attachments) count them
// in their label; views of history do not. This file serves the tabs that replaced
// lists on the Overview (UR-162) and the counts shown on page load; after a
// tab loads, its content keeps the count current.

// --- Documents tabs ---

func (c *PageProductController) renderDocumentsTab(w http.ResponseWriter, r *http.Request, docs []*model.Document, noun string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentsTabContent(r.PathValue("id"), docs, noun).Render(r.Context(), w)
}

// RequirementDocumentsTab renders GET /products/{id}/requirements/{requirementId}/tabs/documents.
func (c *PageProductController) RequirementDocumentsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	c.renderDocumentsTab(w, r, c.linkedDocuments(r, user, r.PathValue("id"), model.DocumentLinkRequirement, r.PathValue("requirementId")), "requirement")
}

// ConstraintDocumentsTab renders GET /products/{id}/constraints/{constraintId}/tabs/documents.
func (c *PageProductController) ConstraintDocumentsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	c.renderDocumentsTab(w, r, c.linkedDocuments(r, user, r.PathValue("id"), model.DocumentLinkConstraint, r.PathValue("constraintId")), "constraint")
}

// TestCaseDocumentsTab renders GET /products/{id}/test-cases/{testCaseId}/tabs/documents.
func (c *PageProductController) TestCaseDocumentsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	c.renderDocumentsTab(w, r, c.linkedDocuments(r, user, r.PathValue("id"), model.DocumentLinkTestCase, r.PathValue("testCaseId")), "test case")
}

// ComponentDocumentsTab renders GET /products/{id}/components/{componentId}/tabs/documents.
func (c *PageProductController) ComponentDocumentsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	c.renderDocumentsTab(w, r, c.structureDocuments(r, user, r.PathValue("id"), store.DocumentFilter{ComponentID: r.PathValue("componentId")}), "component")
}

// CapabilityDocumentsTab renders GET /products/{id}/capabilities/{capabilityId}/tabs/documents.
func (c *PageProductController) CapabilityDocumentsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	c.renderDocumentsTab(w, r, c.structureDocuments(r, user, r.PathValue("id"), store.DocumentFilter{CapabilityID: r.PathValue("capabilityId")}), "capability")
}

// --- Children and inherited capabilities of backlog items ---

func (c *PageProductController) backlogChildrenTab(w http.ResponseWriter, r *http.Request, itemID string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	if _, err := c.backlog.GetRequirement(r.Context(), user, productID, itemID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	children, _ := c.backlog.ListChildRequirements(r.Context(), user, productID, itemID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ChildrenTabContent(productID, children, c.buildBacklogPrefixMap(r, user, productID), c.buildStatusMap(r, user, productID, "requirement")).Render(r.Context(), w)
}

// RequirementChildrenTab renders GET /products/{id}/requirements/{requirementId}/tabs/children.
func (c *PageProductController) RequirementChildrenTab(w http.ResponseWriter, r *http.Request) {
	c.backlogChildrenTab(w, r, r.PathValue("requirementId"))
}

// TaskChildrenTab renders GET /products/{id}/tasks/{taskId}/tabs/children.
func (c *PageProductController) TaskChildrenTab(w http.ResponseWriter, r *http.Request) {
	c.backlogChildrenTab(w, r, r.PathValue("taskId"))
}

// TaskCapabilitiesTab renders GET /products/{id}/tasks/{taskId}/tabs/capabilities:
// the capabilities the task inherits from the requirement it refines (UR-160).
func (c *PageProductController) TaskCapabilitiesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	task, err := c.backlog.GetRequirement(r.Context(), user, productID, r.PathValue("taskId"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TaskCapabilitiesTabContent(productID, c.inheritedCapabilities(r, user, productID, task)).Render(r.Context(), w)
}

// --- Counts shown in the tab labels on page load ---

// countOf returns the number of items a tab loader finds, or 0 when it fails.
func countOf[P any](props P, err error, n func(P) int) int {
	if err != nil {
		return 0
	}
	return n(props)
}

func (c *PageProductController) requirementTabCounts(r *http.Request, user *model.User, productID, id string) map[string]int {
	children, _ := c.backlog.ListChildRequirements(r.Context(), user, productID, id)
	caps, capsErr := c.loadRequirementCapabilitiesTabProps(r, user, productID, id)
	tcs, tcsErr := c.loadRequirementTestCasesTabProps(r, user, productID, id)
	defs, defsErr := c.loadRequirementDefectsTabProps(r, user, productID, id)
	return c.discussionCounts(r, user, productID, model.OwnerTypeRequirement, id, map[string]int{
		"children":     len(children),
		"capabilities": countOf(caps, capsErr, func(p productpage.RequirementCapabilitiesTabProps) int { return len(p.Capabilities) }),
		"test-cases":   countOf(tcs, tcsErr, func(p productpage.RequirementTestCasesTabProps) int { return len(p.TestCases) }),
		"defects":      countOf(defs, defsErr, func(p productpage.RequirementDefectsTabProps) int { return len(p.Defects) }),
		"documents":    len(c.linkedDocuments(r, user, productID, model.DocumentLinkRequirement, id)),
	})
}

func (c *PageProductController) taskTabCounts(r *http.Request, user *model.User, productID string, task *model.Requirement) map[string]int {
	children, _ := c.backlog.ListChildRequirements(r.Context(), user, productID, task.ID)
	return c.discussionCounts(r, user, productID, model.OwnerTypeTask, task.ID, map[string]int{
		"children":     len(children),
		"capabilities": len(c.inheritedCapabilities(r, user, productID, task).Capabilities),
	})
}

func (c *PageProductController) constraintTabCounts(r *http.Request, user *model.User, productID, id string) map[string]int {
	caps, capsErr := c.loadConstraintCapabilitiesTabProps(r, user, productID, id)
	tcs, tcsErr := c.loadConstraintTestCasesTabProps(r, user, productID, id)
	return c.discussionCounts(r, user, productID, model.OwnerTypeConstraint, id, map[string]int{
		"capabilities": countOf(caps, capsErr, func(p productpage.ConstraintCapabilitiesTabProps) int { return len(p.Capabilities) }),
		"test-cases":   countOf(tcs, tcsErr, func(p productpage.ConstraintTestCasesTabProps) int { return len(p.Linked) + len(p.Derived) }),
		"documents":    len(c.linkedDocuments(r, user, productID, model.DocumentLinkConstraint, id)),
	})
}

func (c *PageProductController) testCaseTabCounts(r *http.Request, user *model.User, productID, id string) map[string]int {
	reqs, reqsErr := c.loadTestCaseRequirementsTabProps(r, user, productID, id)
	cons, consErr := c.loadTestCaseConstraintsTabProps(r, user, productID, id)
	return c.discussionCounts(r, user, productID, model.OwnerTypeTestCase, id, map[string]int{
		"requirements": countOf(reqs, reqsErr, func(p productpage.TestCaseRequirementsTabProps) int { return len(p.Requirements) }),
		"constraints":  countOf(cons, consErr, func(p productpage.TestCaseConstraintsTabProps) int { return len(p.Constraints) }),
		"documents":    len(c.linkedDocuments(r, user, productID, model.DocumentLinkTestCase, id)),
	})
}

func hostDefectCount(p productpage.HostDefectsTabProps) int { return len(p.Defects) + len(p.Derived) }

func (c *PageProductController) componentTabCounts(r *http.Request, user *model.User, productID, id string) map[string]int {
	caps, capsErr := c.loadComponentCapabilitiesTabProps(r, user, productID, id)
	tcs, tcsErr := c.loadComponentTestCasesTabProps(r, user, productID, id)
	defs, defsErr := c.loadHostDefectsTabProps(r, user, productID, id, componentDefectHost)
	return c.discussionCounts(r, user, productID, model.OwnerTypeComponent, id, map[string]int{
		"capabilities": countOf(caps, capsErr, func(p productpage.ComponentCapabilitiesTabProps) int { return len(p.Capabilities) }),
		"requirements": len(c.componentRequirements(r, user, productID, id)),
		"test-cases":   countOf(tcs, tcsErr, func(p productpage.ComponentTestCasesTabProps) int { return len(p.TestCases) }),
		"defects":      countOf(defs, defsErr, hostDefectCount),
		"documents":    len(c.structureDocuments(r, user, productID, store.DocumentFilter{ComponentID: id})),
	})
}

func (c *PageProductController) capabilityTabCounts(r *http.Request, user *model.User, productID, id string) map[string]int {
	reqs, reqsErr := c.loadCapabilityRequirementsTabProps(r, user, productID, id)
	tcs, tcsErr := c.loadCapabilityTestCasesTabProps(r, user, productID, id)
	defs, defsErr := c.loadHostDefectsTabProps(r, user, productID, id, capabilityDefectHost)
	cons, consErr := c.loadCapabilityConstraintsTabProps(r, user, productID, id)
	return c.discussionCounts(r, user, productID, model.OwnerTypeCapability, id, map[string]int{
		"requirements": countOf(reqs, reqsErr, func(p productpage.CapabilityRequirementsTabProps) int { return len(p.Requirements) }),
		"test-cases":   countOf(tcs, tcsErr, func(p productpage.CapabilityTestCasesTabProps) int { return len(p.TestCases) }),
		"defects":      countOf(defs, defsErr, hostDefectCount),
		"constraints":  countOf(cons, consErr, func(p productpage.CapabilityConstraintsTabProps) int { return len(p.Constraints) }),
		"documents":    len(c.structureDocuments(r, user, productID, store.DocumentFilter{CapabilityID: id})),
	})
}

func (c *PageProductController) defectTabCounts(r *http.Request, user *model.User, productID, id string) map[string]int {
	comps, compsErr := c.loadDefectComponentsTabProps(r, user, productID, id)
	caps, capsErr := c.loadDefectCapabilitiesTabProps(r, user, productID, id)
	tcs, tcsErr := c.loadDefectTestCasesTabProps(r, user, productID, id)
	reqs, reqsErr := c.loadDefectRequirementsTabProps(r, user, productID, id)
	return c.discussionCounts(r, user, productID, model.OwnerTypeDefect, id, map[string]int{
		"components":   countOf(comps, compsErr, func(p productpage.DefectComponentsTabProps) int { return len(p.Components) }),
		"capabilities": countOf(caps, capsErr, func(p productpage.DefectCapabilitiesTabProps) int { return len(p.Capabilities) }),
		"test-cases":   countOf(tcs, tcsErr, func(p productpage.DefectTestCasesTabProps) int { return len(p.TestCases) }),
		"requirements": countOf(reqs, reqsErr, func(p productpage.DefectRequirementsTabProps) int { return len(p.Requirements) }),
	})
}

func (c *PageProductController) milestoneTabCounts(r *http.Request, user *model.User, productID, id, workflowMode string) map[string]int {
	comps, compsErr := c.loadMilestoneComponentsTabProps(r, user, productID, id)
	caps, capsErr := c.loadMilestoneCapabilitiesTabProps(r, user, productID, id)
	reqs, reqsErr := c.loadMilestoneBacklogTabProps(r, user, productID, id, model.RequirementKindRequirement)
	tasks, tasksErr := c.loadMilestoneBacklogTabProps(r, user, productID, id, model.RequirementKindTask)
	cons, consErr := c.loadMilestoneConstraintsTabProps(r, user, productID, id)
	defs, defsErr := c.loadMilestoneDefectsTabProps(r, user, productID, id)
	counts := map[string]int{
		"components":   countOf(comps, compsErr, func(p productpage.MilestoneComponentsTabProps) int { return len(p.Components) + len(p.Affected) }),
		"capabilities": countOf(caps, capsErr, func(p productpage.MilestoneCapabilitiesTabProps) int { return len(p.Capabilities) + len(p.Affected) }),
		"requirements": countOf(reqs, reqsErr, func(p productpage.MilestoneRequirementsTabProps) int { return len(p.Requirements) }),
		"tasks":        countOf(tasks, tasksErr, func(p productpage.MilestoneRequirementsTabProps) int { return len(p.Requirements) }),
		"constraints":  countOf(cons, consErr, func(p productpage.MilestoneConstraintsTabProps) int { return len(p.Constraints) }),
		"defects":      countOf(defs, defsErr, func(p productpage.MilestoneDefectsTabProps) int { return len(p.Defects) }),
	}
	if workflowMode == model.WorkflowModeScrum { // no Sprints tab in Kanban (UR-157)
		sprints, err := c.loadMilestoneSprintsTabProps(r, user, productID, id)
		counts["sprints"] = countOf(sprints, err, func(p productpage.MilestoneSprintsTabProps) int { return len(p.Sprints) })
	}
	return c.discussionCounts(r, user, productID, model.OwnerTypeMilestone, id, counts)
}

func (c *PageProductController) documentTabCounts(r *http.Request, user *model.User, productID, id string) map[string]int {
	links, err := c.loadDocumentLinksProps(r, user, productID, id)
	return c.discussionCounts(r, user, productID, model.OwnerTypeDocument, id, map[string]int{
		"links": countOf(links, err, func(p productpage.DocumentLinksTabProps) int {
			return len(p.Requirements) + len(p.Constraints) + len(p.TestCases)
		}),
	})
}
