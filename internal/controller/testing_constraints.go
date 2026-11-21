// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// Test case to constraint links (FR-150).

// UseConstraints lets the controller name and resolve the constraints a test
// case is linked to (FR-150).
func (c *TestingController) UseConstraints(constraints *service.ConstraintService) {
	c.constraints = constraints
}

// ListTestCaseConstraints handles GET /products/{productId}/test-cases/{id}/constraints.
func (c *TestingController) ListTestCaseConstraints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.testing.ListTestCaseConstraints(r.Context(), user, productID, r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	var prefixes map[string]string
	if c.constraints != nil {
		prefixes, _ = c.constraints.CategoryPrefixes(r.Context(), user, productID)
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, constraintToJSON(item, prefixes))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// LinkConstraint handles POST /products/{productId}/test-cases/{id}/constraints.
// constraint_id takes an ID or an identifier such as SC-30.
func (c *TestingController) LinkConstraint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		ConstraintID string `json:"constraint_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.ConstraintID == "" {
		httputil.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", "constraint_id is required")
		return
	}
	constraintID := req.ConstraintID
	if c.constraints != nil {
		var err error
		if constraintID, err = c.constraints.ResolveConstraintRef(r.Context(), user, productID, constraintID); err != nil {
			writeServiceError(w, err)
			return
		}
	}

	if err := c.testing.LinkConstraint(r.Context(), user, productID, r.PathValue("id"), constraintID); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "linked"})
}

// UnlinkConstraint handles DELETE /products/{productId}/test-cases/{id}/constraints/{conId}.
func (c *TestingController) UnlinkConstraint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	if err := c.testing.UnlinkConstraint(r.Context(), user, productID, r.PathValue("id"), r.PathValue("conId")); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

// ListConstraintTestCases handles GET /products/{productId}/constraints/{id}/test-cases:
// the test cases linked to the constraint, then those reached through its
// capabilities, which name them in "via" (UC-87). Each carries its latest
// result.
func (c *TestingController) ListConstraintTestCases(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	tcs, err := c.testing.ListConstraintTestCases(r.Context(), user, productID, r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	result := make([]map[string]any, 0, len(tcs.Linked)+len(tcs.Derived))
	for _, ev := range tcs.Linked {
		result = append(result, evidenceToJSON(ev, nil))
	}
	for _, d := range tcs.Derived {
		result = append(result, evidenceToJSON(d.TestCaseEvidence, d.Via))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// evidenceToJSON renders a test case with its latest result and, for one
// reached through capabilities, those capabilities.
func evidenceToJSON(ev service.TestCaseEvidence, via []*model.Capability) map[string]any {
	out := testCaseToJSON(ev.TestCase)
	out["latest_result"] = latestResultToJSON(ev.Latest)
	viaJSON := make([]map[string]string, 0, len(via))
	for _, capability := range via {
		viaJSON = append(viaJSON, map[string]string{"id": capability.ID, "name": capability.Name})
	}
	out["via"] = viaJSON
	return out
}

// latestResultToJSON renders a test case's latest result, nil if it was never
// executed.
func latestResultToJSON(r *model.CampaignResult) map[string]any {
	if r == nil {
		return nil
	}
	return map[string]any{
		"campaign_id":      r.CampaignID,
		"environment_name": r.EnvironmentName,
		"status":           r.Status,
		"executed_at":      r.ExecutedAt,
	}
}
