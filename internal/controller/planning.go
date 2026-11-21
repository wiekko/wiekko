// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"wiekko.net/internal/httputil"
	"wiekko.net/internal/middleware"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
)

// PlanningController handles milestones, sprints, statuses, pyramid levels, and runtime environments.
type PlanningController struct {
	milestone *service.MilestoneService
}

// NewPlanning creates a PlanningController.
func NewPlanning(milestone *service.MilestoneService) *PlanningController {
	return &PlanningController{milestone: milestone}
}

// --- Milestones ---

// ListMilestones handles GET /products/{productId}/milestones.
func (c *PlanningController) ListMilestones(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.milestone.ListMilestones(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, milestoneToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateMilestone handles POST /products/{productId}/milestones.
func (c *PlanningController) CreateMilestone(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		DueDate     string `json:"due_date"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.CreateMilestone(r.Context(), user, service.CreateMilestoneInput{
		ProductID:   productID,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		DueDate:     req.DueDate,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, milestoneToJSON(item))
}

// GetMilestone handles GET /products/{productId}/milestones/{id}.
func (c *PlanningController) GetMilestone(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.milestone.GetMilestone(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, milestoneToJSON(item))
}

// UpdateMilestone handles PATCH /products/{productId}/milestones/{id}.
func (c *PlanningController) UpdateMilestone(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		// IfUpdatedAt is the updated_at the client edited: a change made
		// meanwhile turns the update into 409 CONFLICT.
		IfUpdatedAt string  `json:"if_updated_at"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Type        *string `json:"type"`
		DueDate     *string `json:"due_date"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.UpdateMilestone(r.Context(), user, productID, id, service.UpdateMilestoneInput{
		IfUpdatedAt: req.IfUpdatedAt,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		DueDate:     req.DueDate,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, milestoneToJSON(item))
}

// DeleteMilestone handles DELETE /products/{productId}/milestones/{id}.
func (c *PlanningController) DeleteMilestone(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.milestone.DeleteMilestone(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Sprints ---

// ListSprints handles GET /products/{productId}/sprints.
func (c *PlanningController) ListSprints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.milestone.ListSprints(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, sprintToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateSprint handles POST /products/{productId}/sprints.
func (c *PlanningController) CreateSprint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Name        string `json:"name"`
		MilestoneID string `json:"milestone_id"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.CreateSprint(r.Context(), user, service.CreateSprintInput{
		ProductID:   productID,
		Name:        req.Name,
		MilestoneID: req.MilestoneID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, sprintToJSON(item))
}

// GetSprint handles GET /products/{productId}/sprints/{id}.
func (c *PlanningController) GetSprint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.milestone.GetSprint(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, sprintToJSON(item))
}

// UpdateSprint handles PATCH /products/{productId}/sprints/{id}.
func (c *PlanningController) UpdateSprint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		// IfUpdatedAt is the updated_at the client edited: a change made
		// meanwhile turns the update into 409 CONFLICT.
		IfUpdatedAt string  `json:"if_updated_at"`
		Name        *string `json:"name"`
		MilestoneID *string `json:"milestone_id"`
		Status      *string `json:"status"`
		StartDate   *string `json:"start_date"`
		EndDate     *string `json:"end_date"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.UpdateSprint(r.Context(), user, productID, id, service.UpdateSprintInput{
		IfUpdatedAt: req.IfUpdatedAt,
		Name:        req.Name,
		MilestoneID: req.MilestoneID,
		Status:      req.Status,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, sprintToJSON(item))
}

// DeleteSprint handles DELETE /products/{productId}/sprints/{id}.
func (c *PlanningController) DeleteSprint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.milestone.DeleteSprint(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Statuses ---

// ListStatuses handles GET /products/{productId}/statuses.
func (c *PlanningController) ListStatuses(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	entityType := r.URL.Query().Get("entity_type")

	items, err := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, statusToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateStatus handles POST /products/{productId}/statuses.
func (c *PlanningController) CreateStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		EntityType string `json:"entity_type"`
		Name       string `json:"name"`
		Position   int    `json:"position"`
		IsDefault  bool   `json:"is_default"`
		IsClosed   bool   `json:"is_closed"`
		IsActive   bool   `json:"is_active"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.CreateStatus(r.Context(), user, service.CreateStatusInput{
		ProductID:  productID,
		EntityType: req.EntityType,
		Name:       req.Name,
		Position:   req.Position,
		IsDefault:  req.IsDefault,
		IsClosed:   req.IsClosed,
		IsActive:   req.IsActive,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, statusToJSON(item))
}

// GetStatus handles GET /products/{productId}/statuses/{id}.
func (c *PlanningController) GetStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.milestone.GetStatus(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, statusToJSON(item))
}

// UpdateStatus handles PATCH /products/{productId}/statuses/{id}.
func (c *PlanningController) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Name      *string `json:"name"`
		Position  *int    `json:"position"`
		IsDefault *bool   `json:"is_default"`
		IsClosed  *bool   `json:"is_closed"`
		IsActive  *bool   `json:"is_active"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.UpdateStatus(r.Context(), user, productID, id, service.UpdateStatusInput{
		Name:      req.Name,
		Position:  req.Position,
		IsDefault: req.IsDefault,
		IsClosed:  req.IsClosed,
		IsActive:  req.IsActive,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, statusToJSON(item))
}

// DeleteStatus handles DELETE /products/{productId}/statuses/{id}.
func (c *PlanningController) DeleteStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.milestone.DeleteStatus(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Pyramid Levels ---

// ListPyramidLevels handles GET /products/{productId}/pyramid-levels.
func (c *PlanningController) ListPyramidLevels(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, pyramidLevelToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreatePyramidLevel handles POST /products/{productId}/pyramid-levels.
func (c *PlanningController) CreatePyramidLevel(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
		Position int    `json:"position"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.CreatePyramidLevel(r.Context(), user, service.CreatePyramidLevelInput{
		ProductID: productID,
		ParentID:  req.ParentID,
		Name:      req.Name,
		Position:  req.Position,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, pyramidLevelToJSON(item))
}

// GetPyramidLevel handles GET /products/{productId}/pyramid-levels/{id}.
func (c *PlanningController) GetPyramidLevel(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.milestone.GetPyramidLevel(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, pyramidLevelToJSON(item))
}

// UpdatePyramidLevel handles PATCH /products/{productId}/pyramid-levels/{id}.
func (c *PlanningController) UpdatePyramidLevel(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Name     *string `json:"name"`
		Position *int    `json:"position"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.UpdatePyramidLevel(r.Context(), user, productID, id, service.UpdatePyramidLevelInput{
		Name:     req.Name,
		Position: req.Position,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, pyramidLevelToJSON(item))
}

// DeletePyramidLevel handles DELETE /products/{productId}/pyramid-levels/{id}.
func (c *PlanningController) DeletePyramidLevel(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.milestone.DeletePyramidLevel(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Runtime Environments ---

// ListRuntimeEnvironments handles GET /products/{productId}/runtime-environments.
func (c *PlanningController) ListRuntimeEnvironments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, runtimeEnvironmentToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateRuntimeEnvironment handles POST /products/{productId}/runtime-environments.
func (c *PlanningController) CreateRuntimeEnvironment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Name string `json:"name"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.milestone.CreateRuntimeEnvironment(r.Context(), user, productID, req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, runtimeEnvironmentToJSON(item))
}

// DeleteRuntimeEnvironment handles DELETE /products/{productId}/runtime-environments/{id}.
func (c *PlanningController) DeleteRuntimeEnvironment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.milestone.DeleteRuntimeEnvironment(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func milestoneToJSON(m *model.Milestone) map[string]any {
	return map[string]any{
		"id":          m.ID,
		"product_id":  m.ProductID,
		"name":        m.Name,
		"description": m.Description,
		"type":        m.Type,
		"due_date":    m.DueDate,
		"created_at":  m.CreatedAt,
		"updated_at":  m.UpdatedAt,
	}
}

func sprintToJSON(s *model.Sprint) map[string]any {
	return map[string]any{
		"id":           s.ID,
		"product_id":   s.ProductID,
		"name":         s.Name,
		"milestone_id": s.MilestoneID,
		"status":       s.Status,
		"start_date":   s.StartDate,
		"end_date":     s.EndDate,
		"created_at":   s.CreatedAt,
		"updated_at":   s.UpdatedAt,
	}
}

func statusToJSON(s *model.Status) map[string]any {
	return map[string]any{
		"id":          s.ID,
		"product_id":  s.ProductID,
		"entity_type": s.EntityType,
		"name":        s.Name,
		"position":    s.Position,
		"is_default":  s.IsDefault,
		"is_closed":   s.IsClosed,
		"is_active":   s.IsActive,
		"created_at":  s.CreatedAt,
	}
}

func pyramidLevelToJSON(pl *model.PyramidLevel) map[string]any {
	return map[string]any{
		"id":         pl.ID,
		"product_id": pl.ProductID,
		"parent_id":  pl.ParentID,
		"name":       pl.Name,
		"position":   pl.Position,
		"created_at": pl.CreatedAt,
	}
}

func runtimeEnvironmentToJSON(env *model.RuntimeEnvironment) map[string]any {
	return map[string]any{
		"id":         env.ID,
		"product_id": env.ProductID,
		"name":       env.Name,
		"created_at": env.CreatedAt,
	}
}
