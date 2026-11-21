// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// TestingController handles test case, campaign, and campaign result endpoints.
type TestingController struct {
	testing   *service.TestingService
	defectRef refResolver
	documents *service.DocumentService
	// constraints names and resolves linked constraints (FR-150); nil
	// accepts constraint IDs only.
	constraints *service.ConstraintService
}

// UseDocuments makes single-item responses name the documents linked to the
// item (FR-141).
func (c *TestingController) UseDocuments(docs *service.DocumentService) { c.documents = docs }

// NewTesting creates a TestingController.
//
// defectRef resolves defect identifiers given for a campaign result's defect
// (FR-49); nil accepts IDs only.
func NewTesting(testing *service.TestingService, defectRef refResolver) *TestingController {
	return &TestingController{testing: testing, defectRef: defectRef}
}

// --- Test Cases ---

// ListTestCases handles GET /products/{productId}/test-cases.
func (c *TestingController) ListTestCases(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.testing.ListTestCases(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// Optional exact-match filters, e.g. ?test_identifier=TestFoo to find the
	// test case behind a test function.
	identifier := r.URL.Query().Get("test_identifier")
	mode := r.URL.Query().Get("execution_mode")
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if identifier != "" && item.TestIdentifier != identifier {
			continue
		}
		if mode != "" && item.ExecutionMode != mode {
			continue
		}
		result = append(result, testCaseToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// ListRequirementTestCases handles GET /products/{productId}/requirements/{id}/test-cases.
func (c *TestingController) ListRequirementTestCases(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.testing.ListLinkedTestCasesForRequirement(r.Context(), user, productID, r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeTestCases(w, items, productID)
}

// ListCapabilityTestCases handles GET /products/{productId}/capabilities/{id}/test-cases.
func (c *TestingController) ListCapabilityTestCases(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.testing.ListTestCasesByCapability(r.Context(), user, productID, r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeTestCases(w, items, productID)
}

// ListTestCaseResults handles GET /products/{productId}/test-cases/{id}/results:
// the test case's results across all campaigns.
func (c *TestingController) ListTestCaseResults(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	// The result store is keyed by test case only; make sure the test case
	// belongs to the product in the URL before listing its results.
	tc, err := c.testing.GetTestCase(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if tc.ProductID != productID {
		writeServiceError(w, service.ErrNotFound)
		return
	}
	items, err := c.testing.ListResultsForTestCase(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, campaignResultToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// writeTestCases renders test cases, dropping any that belong to another
// product.
func writeTestCases(w http.ResponseWriter, items []*model.TestCase, productID string) {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.ProductID != productID {
			continue
		}
		result = append(result, testCaseToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateTestCase handles POST /products/{productId}/test-cases.
func (c *TestingController) CreateTestCase(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		ComponentID    string `json:"component_id"`
		CapabilityID   string `json:"capability_id"`
		Title          string `json:"title"`
		Description    string `json:"description"`
		ExecutionMode  string `json:"execution_mode"`
		TestIdentifier string `json:"test_identifier"`
		PyramidLevelID string `json:"pyramid_level_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.testing.CreateTestCase(r.Context(), user, service.CreateTestCaseInput{
		ProductID:      productID,
		ComponentID:    req.ComponentID,
		CapabilityID:   req.CapabilityID,
		Title:          req.Title,
		Description:    req.Description,
		ExecutionMode:  req.ExecutionMode,
		TestIdentifier: req.TestIdentifier,
		PyramidLevelID: req.PyramidLevelID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, testCaseToJSON(item))
}

// GetTestCase handles GET /products/{productId}/test-cases/{id}.
func (c *TestingController) GetTestCase(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.testing.GetTestCase(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, withLinkedDocuments(r, c.documents, model.DocumentLinkTestCase, item.ID, testCaseToJSON(item)))
}

// UpdateTestCase handles PATCH /products/{productId}/test-cases/{id}.
func (c *TestingController) UpdateTestCase(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		// IfUpdatedAt is the updated_at the client edited: a change made
		// meanwhile turns the update into 409 CONFLICT.
		IfUpdatedAt    string  `json:"if_updated_at"`
		ComponentID    *string `json:"component_id"`
		CapabilityID   *string `json:"capability_id"`
		Title          *string `json:"title"`
		Description    *string `json:"description"`
		ExecutionMode  *string `json:"execution_mode"`
		TestIdentifier *string `json:"test_identifier"`
		PyramidLevelID *string `json:"pyramid_level_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.testing.UpdateTestCase(r.Context(), user, productID, id, service.UpdateTestCaseInput{
		IfUpdatedAt:    req.IfUpdatedAt,
		ComponentID:    req.ComponentID,
		CapabilityID:   req.CapabilityID,
		Title:          req.Title,
		Description:    req.Description,
		ExecutionMode:  req.ExecutionMode,
		TestIdentifier: req.TestIdentifier,
		PyramidLevelID: req.PyramidLevelID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, testCaseToJSON(item))
}

// DeleteTestCase handles DELETE /products/{productId}/test-cases/{id}.
func (c *TestingController) DeleteTestCase(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.testing.DeleteTestCase(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListTestCaseRequirements handles GET /products/{productId}/test-cases/{id}/requirements.
func (c *TestingController) ListTestCaseRequirements(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.testing.ListTestCaseRequirements(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, testCaseRequirementToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// LinkRequirement handles POST /products/{productId}/test-cases/{id}/requirements.
func (c *TestingController) LinkRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		RequirementID string `json:"requirement_id"`
		LinkType      string `json:"link_type"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.testing.LinkRequirement(r.Context(), user, productID, id, req.RequirementID, req.LinkType); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "linked"})
}

// UnlinkRequirement handles DELETE /products/{productId}/test-cases/{id}/requirements/{reqId}.
func (c *TestingController) UnlinkRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")
	reqID := r.PathValue("reqId")

	if err := c.testing.UnlinkRequirement(r.Context(), user, productID, id, reqID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

// ListTestCaseEnvironments handles GET /products/{productId}/test-cases/{id}/environments.
func (c *TestingController) ListTestCaseEnvironments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	envIDs, err := c.testing.ListTestCaseEnvironments(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, envIDs)
}

// AddEnvironment handles POST /products/{productId}/test-cases/{id}/environments.
func (c *TestingController) AddEnvironment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		EnvironmentID string `json:"environment_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.testing.AddTestCaseEnvironment(r.Context(), user, productID, id, req.EnvironmentID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

// RemoveEnvironment handles DELETE /products/{productId}/test-cases/{id}/environments/{envId}.
func (c *TestingController) RemoveEnvironment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")
	envID := r.PathValue("envId")

	if err := c.testing.RemoveTestCaseEnvironment(r.Context(), user, productID, id, envID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// --- Campaigns ---

// ListCampaigns handles GET /products/{productId}/campaigns.
func (c *TestingController) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.testing.ListCampaigns(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, campaignToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateCampaign handles POST /products/{productId}/campaigns.
func (c *TestingController) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		StartsAt    string   `json:"starts_at"`
		EndsAt      string   `json:"ends_at"`
		TestCaseIDs []string `json:"test_case_ids"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.testing.CreateCampaign(r.Context(), user, service.CreateCampaignInput{
		ProductID:   productID,
		Name:        req.Name,
		Description: req.Description,
		StartsAt:    req.StartsAt,
		EndsAt:      req.EndsAt,
		TestCaseIDs: req.TestCaseIDs,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, campaignToJSON(item))
}

// GetCampaign handles GET /products/{productId}/campaigns/{id}.
func (c *TestingController) GetCampaign(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.testing.GetCampaign(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, campaignToJSON(item))
}

// UpdateCampaign handles PATCH /products/{productId}/campaigns/{id}.
func (c *TestingController) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		// IfUpdatedAt is the updated_at the client edited: a change made
		// meanwhile turns the update into 409 CONFLICT.
		IfUpdatedAt string  `json:"if_updated_at"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		StartsAt    *string `json:"starts_at"`
		EndsAt      *string `json:"ends_at"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.testing.UpdateCampaign(r.Context(), user, productID, id, service.UpdateCampaignInput{
		IfUpdatedAt: req.IfUpdatedAt,
		Name:        req.Name,
		Description: req.Description,
		StartsAt:    req.StartsAt,
		EndsAt:      req.EndsAt,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, campaignToJSON(item))
}

// DeleteCampaign handles DELETE /products/{productId}/campaigns/{id}.
func (c *TestingController) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.testing.DeleteCampaign(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListCampaignResults handles GET /products/{productId}/campaigns/{id}/results.
func (c *TestingController) ListCampaignResults(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.testing.ListCampaignResults(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, campaignResultToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// ListResultEvents handles GET /products/{productId}/campaigns/{id}/events.
func (c *TestingController) ListResultEvents(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.testing.ListResultEvents(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, campaignResultEventToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// Progress handles GET /products/{productId}/campaigns/{id}/progress.
func (c *TestingController) Progress(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	counts, err := c.testing.CampaignProgress(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"counts":  counts,
		"verdict": model.CampaignVerdict(counts),
	})
}

// ActivateCampaign handles POST /products/{productId}/campaigns/{id}/activate.
func (c *TestingController) ActivateCampaign(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.testing.ActivateCampaign(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, campaignToJSON(item))
}

// AddCampaignTestCases handles POST /products/{productId}/campaigns/{id}/test-cases:
// it adds test cases to a draft campaign's scope (FR-94).
func (c *TestingController) AddCampaignTestCases(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		TestCaseIDs []string `json:"test_case_ids"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if len(req.TestCaseIDs) == 0 {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "test_case_ids is required")
		return
	}
	if err := c.testing.AddCampaignTestCases(r.Context(), user, productID, id, req.TestCaseIDs); err != nil {
		writeServiceError(w, err)
		return
	}
	item, err := c.testing.GetCampaign(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, campaignToJSON(item))
}

// RemoveCampaignTestCase handles DELETE /products/{productId}/campaigns/{id}/test-cases/{testCaseId}:
// it takes a test case out of a draft campaign's scope (FR-94).
func (c *TestingController) RemoveCampaignTestCase(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := c.testing.RemoveCampaignTestCase(r.Context(), user, r.PathValue("productId"), r.PathValue("id"), r.PathValue("testCaseId")); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// CloseCampaign handles POST /products/{productId}/campaigns/{id}/close.
// Closing a campaign with results still awaiting testing requires "force".
func (c *TestingController) CloseCampaign(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Force bool `json:"force"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.testing.CloseCampaign(r.Context(), user, productID, id, req.Force)
	if err != nil {
		var incomplete *service.IncompleteResultsError
		if errors.As(err, &incomplete) {
			httputil.Error(w, http.StatusConflict, "CAMPAIGN_INCOMPLETE", incomplete.Error())
			return
		}
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, campaignToJSON(item))
}

// UpdateResult handles PATCH /products/{productId}/campaigns/{id}/results/{resultId}.
func (c *TestingController) UpdateResult(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	campaignID := r.PathValue("id")
	resultID := r.PathValue("resultId")

	var req struct {
		Status   string `json:"status"`
		DefectID string `json:"defect_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	defectID, err := resolveDefect(r, c.defectRef, req.DefectID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	item, err := c.testing.SetResultStatus(r.Context(), user, productID, campaignID, resultID, req.Status, defectID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, campaignResultToJSON(item))
}

func testCaseToJSON(tc *model.TestCase) map[string]any {
	return map[string]any{
		"id":               tc.ID,
		"product_id":       tc.ProductID,
		"component_id":     tc.ComponentID,
		"capability_id":    tc.CapabilityID,
		"title":            tc.Title,
		"description":      tc.Description,
		"execution_mode":   tc.ExecutionMode,
		"test_identifier":  tc.TestIdentifier,
		"pyramid_level_id": tc.PyramidLevelID,
		"created_at":       tc.CreatedAt,
		"updated_at":       tc.UpdatedAt,
	}
}

func testCaseRequirementToJSON(tcr *model.TestCaseRequirement) map[string]any {
	return map[string]any{
		"id":             tcr.ID,
		"test_case_id":   tcr.TestCaseID,
		"requirement_id": tcr.RequirementID,
		"link_type":      tcr.LinkType,
		"created_at":     tcr.CreatedAt,
	}
}

func campaignToJSON(c *model.TestingCampaign) map[string]any {
	return map[string]any{
		"id":          c.ID,
		"product_id":  c.ProductID,
		"name":        c.Name,
		"description": c.Description,
		"status":      c.Status,
		"starts_at":   c.StartsAt,
		"ends_at":     c.EndsAt,
		"closed_at":   c.ClosedAt,
		"closed_by":   c.ClosedBy,
		"created_at":  c.CreatedAt,
		"updated_at":  c.UpdatedAt,
	}
}

func campaignResultToJSON(cr *model.CampaignResult) map[string]any {
	return map[string]any{
		"id":          cr.ID,
		"campaign_id": cr.CampaignID,
		// The campaign keeps its copy of the test: test_title is the title it
		// copied; test_case_id is null once the test case has been deleted.
		"test_case_id":     nullable(cr.TestCaseID),
		"test_title":       cr.TestTitle,
		"environment_id":   cr.EnvironmentID,
		"environment_name": cr.EnvironmentName,
		"status":           cr.Status,
		"defect_id":        cr.DefectID,
		"executed_by":      cr.ExecutedBy,
		"executed_at":      cr.ExecutedAt,
		"updated_at":       cr.UpdatedAt,
	}
}

func campaignResultEventToJSON(ev *model.CampaignResultEvent) map[string]any {
	return map[string]any{
		"id":          ev.ID,
		"result_id":   ev.ResultID,
		"campaign_id": ev.CampaignID,
		"user_id":     ev.UserID,
		"from_status": ev.FromStatus,
		"to_status":   ev.ToStatus,
		"defect_id":   ev.DefectID,
		"created_at":  ev.CreatedAt,
	}
}
