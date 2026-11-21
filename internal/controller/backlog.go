// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"wiekko.net/internal/httputil"
	"wiekko.net/internal/middleware"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
)

// BacklogController handles requirement and defect endpoints.
type BacklogController struct {
	backlog   *service.BacklogService
	documents *service.DocumentService
}

// UseDocuments makes single-item responses name the documents linked to the
// item (FR-141).
func (c *BacklogController) UseDocuments(docs *service.DocumentService) { c.documents = docs }

// NewBacklog creates a BacklogController.
func NewBacklog(backlog *service.BacklogService) *BacklogController {
	return &BacklogController{backlog: backlog}
}

// --- Requirements ---

// ListRequirements handles GET /products/{productId}/requirements.
func (c *BacklogController) ListRequirements(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	archived, ok := parseArchivedParam(w, r)
	if !ok {
		return
	}
	filter := store.RequirementFilter{
		Kind:       r.URL.Query().Get("kind"),
		CategoryID: r.URL.Query().Get("category_id"),
		StatusID:   r.URL.Query().Get("status_id"),
		SprintID:   r.URL.Query().Get("sprint_id"),
		Archived:   archived,
	}

	items, err := c.backlog.ListRequirements(r.Context(), user, productID, filter)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	prefixes := c.prefixes(r, user, productID)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, requirementToJSON(item, prefixes))
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
		// CapabilityIDs links a new requirement to capabilities (FR-156).
		CapabilityIDs []string `json:"capability_ids"`
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
		CapabilityIDs:    req.CapabilityIDs,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, requirementToJSON(item, c.prefixes(r, user, productID)))
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

	m := requirementToJSON(item, c.prefixes(r, user, productID))
	if item.Kind == model.RequirementKindRequirement {
		m = withLinkedDocuments(r, c.documents, model.DocumentLinkRequirement, item.ID, m)
	}
	httputil.JSON(w, http.StatusOK, m)
}

// UpdateRequirement handles PATCH /products/{productId}/requirements/{id}.
func (c *BacklogController) UpdateRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		// IfUpdatedAt is the updated_at the client edited: a change made
		// meanwhile turns the update into 409 CONFLICT.
		IfUpdatedAt string `json:"if_updated_at"`
		// ParentID moves the item below another one, "" to the top level
		// (FR-152); the service applies the same checks as on create.
		ParentID         *string `json:"parent_id"`
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
		Archived         *bool   `json:"archived"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.backlog.UpdateRequirement(r.Context(), user, productID, id, service.UpdateRequirementInput{
		IfUpdatedAt:      req.IfUpdatedAt,
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
		IsActive:         req.IsActive,
		Archived:         req.Archived,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, requirementToJSON(item, c.prefixes(r, user, productID)))
}

// DeleteRequirement handles DELETE /products/{productId}/requirements/{id}
// ?children=keep|delete: kept children (the default) move up one level.
func (c *BacklogController) DeleteRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	policy, err := service.ParseChildPolicy(r.URL.Query().Get("children"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_CHILDREN", err.Error())
		return
	}
	if err := c.backlog.DeleteRequirementWith(r.Context(), user, productID, id, policy); err != nil {
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

	prefixes := c.prefixes(r, user, productID)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, requirementToJSON(item, prefixes))
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

// LinkedDefects returns a handler for GET .../{targetType}s/{id}/defects:
// the defects linked to that entity (FR-29, FR-120).
func (c *BacklogController) LinkedDefects(targetType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		productID := r.PathValue("productId")

		items, _, err := c.backlog.ListLinkedDefects(r.Context(), user, productID, targetType, r.PathValue("id"))
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
}

// --- Defects ---

// ListDefects handles GET /products/{productId}/defects.
func (c *BacklogController) ListDefects(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	archived, ok := parseArchivedParam(w, r)
	if !ok {
		return
	}
	filter := store.DefectFilter{
		Criticality: r.URL.Query().Get("criticality"),
		StatusID:    r.URL.Query().Get("status_id"),
		SprintID:    r.URL.Query().Get("sprint_id"),
		Archived:    archived,
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
		// IfUpdatedAt is the updated_at the client edited: a change made
		// meanwhile turns the update into 409 CONFLICT.
		IfUpdatedAt   string  `json:"if_updated_at"`
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
		Archived      *bool   `json:"archived"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.backlog.UpdateDefect(r.Context(), user, productID, id, service.UpdateDefectInput{
		IfUpdatedAt:   req.IfUpdatedAt,
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
		Archived:      req.Archived,
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

// prefixes returns the category prefix map used to render identifiers. The
// request was already authorized by the call that produced the items, so a
// failure here only drops the identifiers rather than failing the request.
func (c *BacklogController) prefixes(r *http.Request, user *model.User, productID string) map[string]string {
	m, err := c.backlog.CategoryPrefixes(r.Context(), user, productID)
	if err != nil {
		return nil
	}
	return m
}

// requirementToJSON renders a requirement or task. identifier ("FR-12") is
// empty when the category prefix is unknown.
func requirementToJSON(r *model.Requirement, prefixes map[string]string) map[string]any {
	identifier := ""
	if p, ok := prefixes[r.CategoryID]; ok {
		identifier = model.EntityIdentifier(p, r.SequenceNumber)
	}
	return map[string]any{
		"id":                 r.ID,
		"identifier":         identifier,
		"sequence_number":    r.SequenceNumber,
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
		"archived":           r.Archived(),
		"archived_at":        r.ArchivedAt,
		"created_at":         r.CreatedAt,
		"updated_at":         r.UpdatedAt,
	}
}

func defectToJSON(d *model.Defect) map[string]any {
	return map[string]any{
		"id":              d.ID,
		"product_id":      d.ProductID,
		"identifier":      d.Identifier,
		"sequence_number": d.SequenceNumber,
		"title":           d.Title,
		"description":     d.Description,
		"criticality":     d.Criticality,
		"status_id":       d.StatusID,
		"milestone_id":    d.MilestoneID,
		"sprint_id":       d.SprintID,
		"environment_id":  d.EnvironmentID,
		"estimation":      d.Estimation,
		"priority":        d.Priority,
		"is_active":       d.IsActive,
		"archived":        d.Archived(),
		"archived_at":     d.ArchivedAt,
		"created_at":      d.CreatedAt,
		"updated_at":      d.UpdatedAt,
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

// parseArchivedParam reads the archived query parameter of the list endpoints
// (FR-155): archived items are left out unless it is "include" or "only".
func parseArchivedParam(w http.ResponseWriter, r *http.Request) (store.ArchivedFilter, bool) {
	archived, ok := store.ParseArchivedFilter(r.URL.Query().Get("archived"), store.ArchivedExclude)
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "INVALID_ARCHIVED", "archived must be include, exclude or only")
	}
	return archived, ok
}
