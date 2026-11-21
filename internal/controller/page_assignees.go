package controller

import (
	"errors"
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/web/templates/patterns"
)

// Assignees on the detail pages of requirements, tasks and defects (UR-171):
// shown in the Overview and changed from the page header, without the edit
// form. The service decides who may change whose assignment.

// assigneesProps describes an item's assignees for the viewer: the current
// assignees and, as choices, whom the viewer may assign or unassign. kind is
// a requirement kind or "defect"; collection is its URL segment.
func (c *PageProductController) assigneesProps(r *http.Request, user *model.User, productID, collection, itemID, kind string, active bool) patterns.AssigneesProps {
	props := patterns.AssigneesProps{URL: "/products/" + productID + "/" + collection + "/" + itemID + "/assignees"}

	var assignments []*model.Assignment
	if kind == "defect" {
		assignments, _ = c.backlog.ListDefectAssignees(r.Context(), user, productID, itemID)
	} else {
		assignments, _ = c.backlog.ListRequirementAssignees(r.Context(), user, productID, itemID)
	}
	assigned := make(map[string]bool, len(assignments))
	for _, a := range assignments {
		assigned[a.UserID] = true
	}

	members := c.buildTeamMembers(r, user, productID)
	names := make(map[string]string, len(members))
	for _, m := range members {
		names[m.UserID] = m.DisplayName
	}
	for _, a := range assignments {
		props.Assigned = append(props.Assigned, patterns.Assignee{UserID: a.UserID, Name: userName(names, a.UserID), Assigned: true})
	}

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return props
	}
	switch {
	case user.IsAdmin || role == model.RoleOwner:
		for _, m := range members {
			props.Choices = append(props.Choices, patterns.Assignee{UserID: m.UserID, Name: m.DisplayName, Assigned: assigned[m.UserID]})
		}
	case role == model.RoleDeveloper && active:
		props.SelfOnly = true
		props.Choices = []patterns.Assignee{{UserID: user.ID, Name: user.DisplayName, Assigned: assigned[user.ID]}}
	}
	return props
}

// ItemAssignees handles POST /products/{id}/{requirements|tasks|defects}/{itemId}/assignees:
// it assigns (assign=true) or unassigns the user_id and answers with the
// header control and, out of band, the Overview field (UR-171).
func (c *PageProductController) ItemAssignees(collection, kind string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		productID := r.PathValue("id")
		itemID := r.PathValue("itemId")
		target := r.FormValue("user_id")
		assign := r.FormValue("assign") == "true"

		var active bool
		var err error
		if kind == "defect" {
			if assign {
				err = c.backlog.AssignDefect(r.Context(), user, productID, itemID, target)
			} else {
				err = c.backlog.UnassignDefect(r.Context(), user, productID, itemID, target)
			}
			if d, gErr := c.backlog.GetDefect(r.Context(), user, productID, itemID); gErr == nil {
				active = d.IsActive
			}
		} else {
			item, gErr := c.backlog.GetRequirement(r.Context(), user, productID, itemID)
			if gErr != nil || item.Kind != kind {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if assign {
				err = c.backlog.AssignRequirement(r.Context(), user, productID, itemID, target)
			} else {
				err = c.backlog.UnassignRequirement(r.Context(), user, productID, itemID, target)
			}
			active = item.IsActive
		}
		if err != nil {
			status := http.StatusUnprocessableEntity
			switch {
			case errors.Is(err, service.ErrForbidden):
				status = http.StatusForbidden
			case errors.Is(err, service.ErrNotFound):
				status = http.StatusNotFound
			}
			http.Error(w, errorText(err), status)
			return
		}

		props := c.assigneesProps(r, user, productID, collection, itemID, kind, active)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = patterns.AssigneeMenu(props).Render(r.Context(), w)
		_ = patterns.AssigneeFieldOOB(props.Assigned).Render(r.Context(), w)
	})
}
