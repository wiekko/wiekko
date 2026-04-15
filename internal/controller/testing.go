package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// TestingController handles test case, campaign, and campaign result endpoints.
type TestingController struct {
	testing *service.TestingService
}

// NewTesting creates a TestingController.
func NewTesting(testing *service.TestingService) *TestingController {
	return &TestingController{testing: testing}
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

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
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

	httputil.JSON(w, http.StatusOK, testCaseToJSON(item))
}

// UpdateTestCase handles PATCH /products/{productId}/test-cases/{id}.
func (c *TestingController) UpdateTestCase(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
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
		Name string `json:"name"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.testing.CreateCampaign(r.Context(), user, service.CreateCampaignInput{
		ProductID: productID,
		Name:      req.Name,
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
		Name *string `json:"name"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.testing.UpdateCampaign(r.Context(), user, productID, id, service.UpdateCampaignInput{
		Name: req.Name,
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

// UpdateResult handles PATCH /products/{productId}/campaigns/{id}/results/{resultId}.
func (c *TestingController) UpdateResult(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	campaignID := r.PathValue("id")

	var req struct {
		TestCaseID    string `json:"test_case_id"`
		EnvironmentID string `json:"environment_id"`
		Status        string `json:"status"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.testing.UpdateCampaignResult(r.Context(), user, productID, campaignID, req.TestCaseID, req.EnvironmentID, req.Status); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
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
		"id":         c.ID,
		"product_id": c.ProductID,
		"name":       c.Name,
		"created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt,
	}
}

func campaignResultToJSON(cr *model.CampaignResult) map[string]any {
	return map[string]any{
		"id":             cr.ID,
		"campaign_id":    cr.CampaignID,
		"test_case_id":   cr.TestCaseID,
		"environment_id": cr.EnvironmentID,
		"status":         cr.Status,
		"updated_at":     cr.UpdatedAt,
	}
}
