// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
)

// --- Task Pages ---
//
// Tasks share the requirements table (model.RequirementKindTask) and the
// requirement status set, so these handlers reuse the backlog service and the
// requirement-facing helpers in page_product.go. What tasks do not share is
// traceability: there are no capability, test case or defect link tabs here,
// because a task carries no product statement to trace.

// TaskDetail renders GET /products/{id}/tasks/{taskId}.
func (c *PageProductController) TaskDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	task, err := c.loadTask(r, user, productID, taskID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/tasks", http.StatusSeeOther)
		return
	}

	props := c.taskDetailProps(r, user, p, task, role)
	props.Meta = templates.PageMeta{
		Title:     p.Name + " - " + task.Title,
		CSRFToken: csrfToken,
	}
	props.DisplayName = user.DisplayName
	props.IsAdmin = user.IsAdmin

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TaskDetailPage(props).Render(r.Context(), w)
}

// TaskCreateForm renders GET /products/{id}/tasks/new.
func (c *PageProductController) TaskCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !c.canEditRequirements(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/tasks", noticeNeedsOwner)
		return
	}

	props := c.taskFormProps(r, user, p, productID)
	props.Meta = templates.PageMeta{
		Title:     p.Name + " - New Task",
		CSRFToken: csrfToken,
	}
	props.DisplayName = user.DisplayName
	props.IsAdmin = user.IsAdmin
	props.StatusID = defaultStatusID(props.Statuses)
	props.Priority = "0"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TaskFormPage(props).Render(r.Context(), w)
}

// TaskCreate handles POST /products/{id}/tasks.
func (c *PageProductController) TaskCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	f := readTaskForm(r)

	_, createErr := c.backlog.CreateRequirement(r.Context(), user, service.CreateRequirementInput{
		ProductID:   productID,
		Kind:        model.RequirementKindTask,
		ParentID:    f.ParentID,
		Title:       f.Title,
		Description: f.Description,
		CategoryID:  f.CategoryID,
		StatusID:    f.StatusID,
		MilestoneID: f.MilestoneID,
		SprintID:    f.SprintID,
		Estimation:  f.Estimation,
		Priority:    f.Priority,
		IsCoarse:    f.IsCoarse,
	})
	if createErr != nil {
		props := c.taskFormProps(r, user, p, productID)
		props.Meta = templates.PageMeta{
			Title:     p.Name + " - New Task",
			CSRFToken: csrfToken,
		}
		props.DisplayName = user.DisplayName
		props.IsAdmin = user.IsAdmin
		props.Error = errorText(createErr)
		f.apply(&props)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TaskFormPage(props).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/tasks", http.StatusSeeOther)
}

// TaskEditForm renders GET /products/{id}/tasks/{taskId}/edit.
func (c *PageProductController) TaskEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !c.canEditRequirements(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/tasks/"+taskID, noticeNeedsOwner)
		return
	}

	task, err := c.loadTask(r, user, productID, taskID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/tasks", http.StatusSeeOther)
		return
	}

	props := c.taskFormProps(r, user, p, productID)
	props.Meta = templates.PageMeta{
		Title:     p.Name + " - Edit " + task.Title,
		CSRFToken: csrfToken,
	}
	props.DisplayName = user.DisplayName
	props.IsAdmin = user.IsAdmin
	props.Task = task
	props.IsEdit = true
	props.ReturnTo = editReturnTarget(r)
	props.Title = task.Title
	props.Description = task.Description
	props.CategoryID = task.CategoryID
	props.ParentID = task.ParentID
	props.StatusID = task.StatusID
	props.MilestoneID = task.MilestoneID
	props.SprintID = task.SprintID
	props.Estimation = task.Estimation
	props.Priority = fmt.Sprintf("%d", task.Priority)
	props.IsCoarse = task.IsCoarse

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TaskFormPage(props).Render(r.Context(), w)
}

// TaskUpdate handles POST /products/{id}/tasks/{taskId}.
func (c *PageProductController) TaskUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	task, err := c.loadTask(r, user, productID, taskID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/tasks", http.StatusSeeOther)
		return
	}

	f := readTaskForm(r)

	_, updateErr := c.backlog.UpdateRequirement(r.Context(), user, productID, taskID, service.UpdateRequirementInput{
		IfUpdatedAt: r.FormValue("updated_at"),
		ParentID:    &f.ParentID,
		Title:       &f.Title,
		Description: &f.Description,
		CategoryID:  &f.CategoryID,
		StatusID:    &f.StatusID,
		MilestoneID: &f.MilestoneID,
		SprintID:    &f.SprintID,
		Estimation:  &f.Estimation,
		Priority:    &f.Priority,
		IsCoarse:    &f.IsCoarse,
	})
	if updateErr != nil {
		props := c.taskFormProps(r, user, p, productID)
		props.Meta = templates.PageMeta{
			Title:     p.Name + " - Edit " + task.Title,
			CSRFToken: csrfToken,
		}
		props.DisplayName = user.DisplayName
		props.IsAdmin = user.IsAdmin
		props.Task = task
		props.IsEdit = true
		props.ReturnTo = r.FormValue("return")
		props.Error = errorText(updateErr)
		f.apply(&props)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TaskFormPage(props).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		"/products/"+productID+"/tasks/"+taskID,
		"/products/"+productID+"/tasks",
	), http.StatusSeeOther)
}

// TaskDelete handles DELETE /products/{id}/tasks/{taskId}.
func (c *PageProductController) TaskDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	if _, err := c.loadTask(r, user, productID, taskID); err == nil {
		policy, _ := service.ParseChildPolicy(r.FormValue("children"))
		_ = c.backlog.DeleteRequirementWith(r.Context(), user, productID, taskID, policy)
	}
	w.Header().Set("HX-Redirect", "/products/"+productID+"/tasks")
	w.WriteHeader(http.StatusOK)
}

// TaskDeleteConfirm renders GET /products/{id}/tasks/{taskId}/delete/confirm.
func (c *PageProductController) TaskDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	task, err := c.loadTask(r, user, productID, taskID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	children, keepTarget := c.deleteChildren(r, user, productID, task)
	_ = productpage.TaskDeleteConfirmation(task.Title, productID, taskID, middleware.CSRFTokenFromContext(r.Context()), children, keepTarget).Render(r.Context(), w)
}

// TaskOverviewTab renders GET /products/{id}/tasks/{taskId}/tabs/overview.
func (c *PageProductController) TaskOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	task, err := c.loadTask(r, user, productID, taskID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TaskOverviewTabContent(c.taskDetailProps(r, user, p, task, role)).Render(r.Context(), w)
}

// TaskHistoryTab renders GET /products/{id}/tasks/{taskId}/tabs/history.
func (c *PageProductController) TaskHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	if _, err := c.loadTask(r, user, productID, taskID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	entries := c.buildAuditViews(r, user, productID, model.RequirementKindTask, taskID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TaskHistoryTabContent(entries).Render(r.Context(), w)
}

// --- Task helpers ---

// loadTask fetches a backlog item and rejects it unless it is a task, so a
// requirement ID cannot be driven through the task routes.
func (c *PageProductController) loadTask(r *http.Request, user *model.User, productID, taskID string) (*model.Requirement, error) {
	task, err := c.backlog.GetRequirement(r.Context(), user, productID, taskID)
	if err != nil {
		return nil, err
	}
	if task.Kind != model.RequirementKindTask {
		return nil, fmt.Errorf("not a task: %s", taskID)
	}
	return task, nil
}

// taskDetailProps assembles the shared detail/overview props.
func (c *PageProductController) taskDetailProps(r *http.Request, user *model.User, p *model.Product, task *model.Requirement, role string) productpage.TaskDetailProps {
	prefixes := c.buildCategoryPrefixMap(r, user, p.ID, model.RequirementKindTask)
	catNames := c.buildCategoryNameMap(r, user, p.ID, model.RequirementKindTask)

	// A task's parent may be a requirement or another task, so resolve its
	// prefix against the merged map.
	var parentName, parentIdentifier, parentPath string
	if task.ParentID != "" {
		if parent, pErr := c.backlog.GetRequirement(r.Context(), user, p.ID, task.ParentID); pErr == nil {
			merged := c.buildBacklogPrefixMap(r, user, p.ID)
			parentName = parent.Title
			parentIdentifier = model.EntityIdentifier(categoryPrefix(merged, parent.CategoryID), parent.SequenceNumber)
			parentPath = backlogItemPath(p.ID, parent)
		}
	}
	children, _ := c.backlog.ListChildRequirements(r.Context(), user, p.ID, task.ID)

	return productpage.TaskDetailProps{
		Product:          p,
		Task:             task,
		CategoryPrefixes: prefixes,
		CategoryNames:    catNames,
		Status:           c.resolveStatus(r, user, p.ID, task.StatusID, model.RequirementKindRequirement),
		MilestoneName:    c.resolveMilestoneName(r, user, p.ID, task.MilestoneID),
		SprintName:       c.resolveSprintName(r, user, p.ID, task.SprintID),
		ParentName:       parentName,
		ParentIdentifier: parentIdentifier,
		ParentPath:       parentPath,
		Children:         children,
		TabCounts:        c.taskTabCounts(r, user, p.ID, task),
		CanEdit:          c.canEditRequirements(user, role),
		Assignees:        c.assigneesProps(r, user, p.ID, "tasks", task.ID, task.Kind, task.IsActive),
	}
}

// taskFormProps assembles the select-option sources shared by the create and
// edit forms. The parent list holds requirements and tasks alike: a task may
// hang off either, only a requirement is restricted to requirement parents.
func (c *PageProductController) taskFormProps(r *http.Request, user *model.User, p *model.Product, productID string) productpage.TaskFormProps {
	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, model.RequirementKindRequirement)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	parents, _ := c.backlog.ListRequirements(r.Context(), user, productID, store.RequirementFilter{})
	categories, _ := c.category.ListCategories(r.Context(), user, productID, model.RequirementKindTask)

	return productpage.TaskFormProps{
		Product:          p,
		Parents:          parents,
		Categories:       categories,
		CategoryPrefixes: c.buildBacklogPrefixMap(r, user, productID),
		Statuses:         statuses,
		Milestones:       milestones,
		Sprints:          sprints,
	}
}

// taskForm holds the parsed task create/edit form.
type taskForm struct {
	Title       string
	Description string
	CategoryID  string
	ParentID    string
	StatusID    string
	MilestoneID string
	SprintID    string
	Estimation  string
	PriorityStr string
	Priority    int
	IsCoarse    bool
}

func readTaskForm(r *http.Request) taskForm {
	f := taskForm{
		Title:       strings.TrimSpace(r.FormValue("title")),
		Description: r.FormValue("description"),
		CategoryID:  r.FormValue("category_id"),
		ParentID:    r.FormValue("parent_id"),
		StatusID:    r.FormValue("status_id"),
		MilestoneID: r.FormValue("milestone_id"),
		SprintID:    r.FormValue("sprint_id"),
		Estimation:  strings.TrimSpace(r.FormValue("estimation")),
		PriorityStr: r.FormValue("priority"),
		IsCoarse:    r.FormValue("is_coarse") == "on",
	}
	f.Priority, _ = strconv.Atoi(f.PriorityStr)
	return f
}

// apply copies the submitted values back onto the form props so a failed
// submission re-renders with the user's input intact.
func (f taskForm) apply(props *productpage.TaskFormProps) {
	props.Title = f.Title
	props.Description = f.Description
	props.CategoryID = f.CategoryID
	props.ParentID = f.ParentID
	props.StatusID = f.StatusID
	props.MilestoneID = f.MilestoneID
	props.SprintID = f.SprintID
	props.Estimation = f.Estimation
	props.Priority = f.PriorityStr
	props.IsCoarse = f.IsCoarse
}

// backlogItemPath returns the detail URL for a backlog item of either kind.
func backlogItemPath(productID string, item *model.Requirement) string {
	if item.Kind == model.RequirementKindTask {
		return "/products/" + productID + "/tasks/" + item.ID
	}
	return "/products/" + productID + "/requirements/" + item.ID
}
