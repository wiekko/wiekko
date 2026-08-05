package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
)

// BacklogController handles requirement and defect endpoints.
type BacklogController struct {
	backlog *service.BacklogService
}

// NewBacklog creates a BacklogController.
func NewBacklog(backlog *service.BacklogService) *BacklogController {
	return &BacklogController{backlog: backlog}
}

// --- Requirements ---

// ListRequirements handles GET /products/{productId}/requirements.
func (c *BacklogController) ListRequirements(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	filter := store.RequirementFilter{
		Kind:       r.URL.Query().Get("kind"),
		CategoryID: r.URL.Query().Get("category_id"),
		StatusID:   r.URL.Query().Get("status_id"),
		SprintID:   r.URL.Query().Get("sprint_id"),
	}

	items, err := c.backlog.ListRequirements(r.Context(), user, productID, filter)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, requirementToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateRequirement handles POST /products/{productId}/requirements.
func (c *BacklogController) CreateRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Kind             string `json:"kind"`
		ParentID         string `json:"parent_id"`
		CategoryID       string `json:"category_id"`
		Title            string `json:"title"`
		Description      string `json:"description"`
		DefinitionOfDone string `json:"definition_of_done"`
		StatusID         string `json:"status_id"`
		MilestoneID      string `json:"milestone_id"`
		SprintID         string `json:"sprint_id"`
		Estimation       string `json:"estimation"`
		Priority         int    `json:"priority"`
		IsCoarse         bool   `json:"is_coarse"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.backlog.CreateRequirement(r.Context(), user, service.CreateRequirementInput{
		ProductID:        productID,
		Kind:             req.Kind,
		ParentID:         req.ParentID,
		CategoryID:       req.CategoryID,
		Title:            req.Title,
		Description:      req.Description,
		DefinitionOfDone: req.DefinitionOfDone,
		StatusID:         req.StatusID,
		MilestoneID:      req.MilestoneID,
		SprintID:         req.SprintID,
		Estimation:       req.Estimation,
		Priority:         req.Priority,
		IsCoarse:         req.IsCoarse,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, requirementToJSON(item))
}

// GetRequirement handles GET /products/{productId}/requirements/{id}.
func (c *BacklogController) GetRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.backlog.GetRequirement(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, requirementToJSON(item))
}

// UpdateRequirement handles PATCH /products/{productId}/requirements/{id}.
func (c *BacklogController) UpdateRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		CategoryID       *string `json:"category_id"`
		Title            *string `json:"title"`
		Description      *string `json:"description"`
		DefinitionOfDone *string `json:"definition_of_done"`
		StatusID         *string `json:"status_id"`
		MilestoneID      *string `json:"milestone_id"`
		SprintID         *string `json:"sprint_id"`
		Estimation       *string `json:"estimation"`
		Priority         *int    `json:"priority"`
		IsCoarse         *bool   `json:"is_coarse"`
		IsActive         *bool   `json:"is_active"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.backlog.UpdateRequirement(r.Context(), user, productID, id, service.UpdateRequirementInput{
		CategoryID:       req.CategoryID,
		Title:            req.Title,
		Description:      req.Description,
		DefinitionOfDone: req.DefinitionOfDone,
		StatusID:         req.StatusID,
		MilestoneID:      req.MilestoneID,
		SprintID:         req.SprintID,
		Estimation:       req.Estimation,
		Priority:         req.Priority,
		IsCoarse:         req.IsCoarse,
		IsActive:         req.IsActive,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, requirementToJSON(item))
}

// DeleteRequirement handles DELETE /products/{productId}/requirements/{id}.
func (c *BacklogController) DeleteRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.backlog.DeleteRequirement(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListChildRequirements handles GET /products/{productId}/requirements/{id}/children.
func (c *BacklogController) ListChildRequirements(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.backlog.ListChildRequirements(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, requirementToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// ListRequirementAssignees handles GET /products/{productId}/requirements/{id}/assignees.
func (c *BacklogController) ListRequirementAssignees(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.backlog.ListRequirementAssignees(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, assignmentToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// AssignRequirement handles POST /products/{productId}/requirements/{id}/assignees.
func (c *BacklogController) AssignRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		UserID string `json:"user_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.backlog.AssignRequirement(r.Context(), user, productID, id, req.UserID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "assigned"})
}

// UnassignRequirement handles DELETE /products/{productId}/requirements/{id}/assignees/{userId}.
func (c *BacklogController) UnassignRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")
	targetUserID := r.PathValue("userId")

	if err := c.backlog.UnassignRequirement(r.Context(), user, productID, id, targetUserID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
}

// --- Defects ---

// ListDefects handles GET /products/{productId}/defects.
func (c *BacklogController) ListDefects(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	filter := store.DefectFilter{
		Criticality: r.URL.Query().Get("criticality"),
		StatusID:    r.URL.Query().Get("status_id"),
		SprintID:    r.URL.Query().Get("sprint_id"),
	}

	items, err := c.backlog.ListDefects(r.Context(), user, productID, filter)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, defectToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateDefect handles POST /products/{productId}/defects.
func (c *BacklogController) CreateDefect(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Title         string `json:"title"`
		Description   string `json:"description"`
		Criticality   string `json:"criticality"`
		StatusID      string `json:"status_id"`
		MilestoneID   string `json:"milestone_id"`
		SprintID      string `json:"sprint_id"`
		EnvironmentID string `json:"environment_id"`
		Estimation    string `json:"estimation"`
		Priority      int    `json:"priority"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.backlog.CreateDefect(r.Context(), user, service.CreateDefectInput{
		ProductID:     productID,
		Title:         req.Title,
		Description:   req.Description,
		Criticality:   req.Criticality,
		StatusID:      req.StatusID,
		MilestoneID:   req.MilestoneID,
		SprintID:      req.SprintID,
		EnvironmentID: req.EnvironmentID,
		Estimation:    req.Estimation,
		Priority:      req.Priority,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, defectToJSON(item))
}

// GetDefect handles GET /products/{productId}/defects/{id}.
func (c *BacklogController) GetDefect(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.backlog.GetDefect(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, defectToJSON(item))
}

// UpdateDefect handles PATCH /products/{productId}/defects/{id}.
func (c *BacklogController) UpdateDefect(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Title         *string `json:"title"`
		Description   *string `json:"description"`
		Criticality   *string `json:"criticality"`
		StatusID      *string `json:"status_id"`
		MilestoneID   *string `json:"milestone_id"`
		SprintID      *string `json:"sprint_id"`
		EnvironmentID *string `json:"environment_id"`
		Estimation    *string `json:"estimation"`
		Priority      *int    `json:"priority"`
		IsActive      *bool   `json:"is_active"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.backlog.UpdateDefect(r.Context(), user, productID, id, service.UpdateDefectInput{
		Title:         req.Title,
		Description:   req.Description,
		Criticality:   req.Criticality,
		StatusID:      req.StatusID,
		MilestoneID:   req.MilestoneID,
		SprintID:      req.SprintID,
		EnvironmentID: req.EnvironmentID,
		Estimation:    req.Estimation,
		Priority:      req.Priority,
		IsActive:      req.IsActive,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, defectToJSON(item))
}

// DeleteDefect handles DELETE /products/{productId}/defects/{id}.
func (c *BacklogController) DeleteDefect(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.backlog.DeleteDefect(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListDefectAssignees handles GET /products/{productId}/defects/{id}/assignees.
func (c *BacklogController) ListDefectAssignees(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.backlog.ListDefectAssignees(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, assignmentToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// AssignDefect handles POST /products/{productId}/defects/{id}/assignees.
func (c *BacklogController) AssignDefect(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		UserID string `json:"user_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.backlog.AssignDefect(r.Context(), user, productID, id, req.UserID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "assigned"})
}

// UnassignDefect handles DELETE /products/{productId}/defects/{id}/assignees/{userId}.
func (c *BacklogController) UnassignDefect(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")
	targetUserID := r.PathValue("userId")

	if err := c.backlog.UnassignDefect(r.Context(), user, productID, id, targetUserID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
}

// ListDefectLinks handles GET /products/{productId}/defects/{id}/links.
func (c *BacklogController) ListDefectLinks(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.backlog.ListDefectLinks(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, defectLinkToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// AddDefectLink handles POST /products/{productId}/defects/{id}/links.
func (c *BacklogController) AddDefectLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, id, req.TargetType, req.TargetID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "linked"})
}

// RemoveDefectLink handles DELETE /products/{productId}/defects/{id}/links/{linkId}.
func (c *BacklogController) RemoveDefectLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	linkID := r.PathValue("linkId")

	if err := c.backlog.RemoveDefectLink(r.Context(), user, productID, linkID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

func requirementToJSON(r *model.Requirement) map[string]any {
	return map[string]any{
		"id":                 r.ID,
		"product_id":         r.ProductID,
		"kind":               r.Kind,
		"parent_id":          r.ParentID,
		"category_id":        r.CategoryID,
		"title":              r.Title,
		"description":        r.Description,
		"definition_of_done": r.DefinitionOfDone,
		"status_id":          r.StatusID,
		"milestone_id":       r.MilestoneID,
		"sprint_id":          r.SprintID,
		"estimation":         r.Estimation,
		"priority":           r.Priority,
		"is_coarse":          r.IsCoarse,
		"is_active":          r.IsActive,
		"created_at":         r.CreatedAt,
		"updated_at":         r.UpdatedAt,
	}
}

func defectToJSON(d *model.Defect) map[string]any {
	return map[string]any{
		"id":             d.ID,
		"product_id":     d.ProductID,
		"title":          d.Title,
		"description":    d.Description,
		"criticality":    d.Criticality,
		"status_id":      d.StatusID,
		"milestone_id":   d.MilestoneID,
		"sprint_id":      d.SprintID,
		"environment_id": d.EnvironmentID,
		"estimation":     d.Estimation,
		"priority":       d.Priority,
		"is_active":      d.IsActive,
		"created_at":     d.CreatedAt,
		"updated_at":     d.UpdatedAt,
	}
}

func assignmentToJSON(a *model.Assignment) map[string]any {
	return map[string]any{
		"id":         a.ID,
		"target_id":  a.TargetID,
		"user_id":    a.UserID,
		"created_at": a.CreatedAt,
	}
}

func defectLinkToJSON(l *model.DefectLink) map[string]any {
	return map[string]any{
		"id":          l.ID,
		"defect_id":   l.DefectID,
		"target_type": l.TargetType,
		"target_id":   l.TargetID,
		"created_at":  l.CreatedAt,
	}
}
