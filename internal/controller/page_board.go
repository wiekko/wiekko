package controller

import (
	"net/http"
	"strings"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
)

// Board renders GET /products/{id}/board — the kanban board for a product.
// Scrum-mode products are redirected to /sprints (matches the sidebar).
func (c *PageProductController) Board(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if p.WorkflowMode == model.WorkflowModeScrum {
		http.Redirect(w, r, "/products/"+productID+"/sprints", http.StatusSeeOther)
		return
	}

	reqStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, "failed to load statuses", http.StatusInternalServerError)
		return
	}
	defStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	if err != nil {
		http.Error(w, "failed to load statuses", http.StatusInternalServerError)
		return
	}
	// Requirements and tasks share the requirement status set, so they share
	// the board columns too; an empty kind filter returns both.
	backlogItems, err := c.backlog.ListRequirements(r.Context(), user, productID, store.RequirementFilter{})
	if err != nil {
		http.Error(w, "failed to load requirements", http.StatusInternalServerError)
		return
	}
	defects, err := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{})
	if err != nil {
		http.Error(w, "failed to load defects", http.StatusInternalServerError)
		return
	}

	prefixes := c.buildBacklogPrefixMap(r, user, productID)

	props := productpage.BoardPageProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Board",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		CanMove:     canMoveCards(user, role),
	}

	reqCards, strandedReq, strandedTask := buildBacklogCards(productID, backlogItems, prefixes)
	defCards, strandedDef := buildDefectCards(productID, defects)
	props.StrandedReq = strandedReq
	props.StrandedTask = strandedTask
	props.StrandedDef = strandedDef

	if p.StatusSync {
		props.Split = false
		props.Columns = mergedBoardColumns(reqStatuses, defStatuses, reqCards, defCards)
	} else {
		props.Split = true
		props.ReqColumns = entityBoardColumns(reqStatuses, reqCards, true)
		props.DefColumns = entityBoardColumns(defStatuses, defCards, false)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.BoardPage(props).Render(r.Context(), w)
}

// BoardRequirementStatus handles PATCH /products/{id}/board/requirements/{reqId}/status.
// Expects a form-encoded `status_id`. Returns 204 on success.
func (c *PageProductController) BoardRequirementStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	reqID := r.PathValue("reqId")

	statusID := strings.TrimSpace(r.FormValue("status_id"))
	_, err := c.backlog.UpdateRequirement(r.Context(), user, productID, reqID, service.UpdateRequirementInput{
		StatusID: &statusID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BoardTaskStatus handles PATCH /products/{id}/board/tasks/{taskId}/status.
// Tasks live in the requirements table and use the requirement status set, so
// this is the requirement move with a URL that reads honestly.
func (c *PageProductController) BoardTaskStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	statusID := strings.TrimSpace(r.FormValue("status_id"))
	_, err := c.backlog.UpdateRequirement(r.Context(), user, productID, taskID, service.UpdateRequirementInput{
		StatusID: &statusID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BoardDefectStatus handles PATCH /products/{id}/board/defects/{defectId}/status.
func (c *PageProductController) BoardDefectStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	statusID := strings.TrimSpace(r.FormValue("status_id"))
	_, err := c.backlog.UpdateDefect(r.Context(), user, productID, defectID, service.UpdateDefectInput{
		StatusID: &statusID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func canMoveCards(user *model.User, role string) bool {
	if user.IsAdmin {
		return true
	}
	return role == model.RoleDeveloper || role == model.RoleOwner
}

// buildBacklogCards turns requirements and tasks into board cards. Both kinds
// carry requirement status IDs, so they land in one map and share the columns;
// the card's EntityType is what tells them apart on screen.
func buildBacklogCards(productID string, items []*model.Requirement, prefixes map[string]string) (byStatus map[string][]productpage.BoardCard, strandedReq, strandedTask []productpage.BoardCard) {
	byStatus = make(map[string][]productpage.BoardCard)
	for _, item := range items {
		segment := "requirements"
		if item.Kind == model.RequirementKindTask {
			segment = "tasks"
		}
		card := productpage.BoardCard{
			EntityType: item.Kind,
			ID:         item.ID,
			Identifier: model.EntityIdentifier(categoryPrefix(prefixes, item.CategoryID), item.SequenceNumber),
			Title:      item.Title,
			Priority:   item.Priority,
			MoveURL:    "/products/" + productID + "/board/" + segment + "/" + item.ID + "/status",
		}
		if item.StatusID == "" {
			if item.Kind == model.RequirementKindTask {
				strandedTask = append(strandedTask, card)
			} else {
				strandedReq = append(strandedReq, card)
			}
			continue
		}
		byStatus[item.StatusID] = append(byStatus[item.StatusID], card)
	}
	return byStatus, strandedReq, strandedTask
}

func buildDefectCards(productID string, defects []*model.Defect) (byStatus map[string][]productpage.BoardCard, stranded []productpage.BoardCard) {
	byStatus = make(map[string][]productpage.BoardCard)
	for _, d := range defects {
		card := productpage.BoardCard{
			EntityType: "defect",
			ID:         d.ID,
			Title:      d.Title,
			Priority:   d.Priority,
			MoveURL:    "/products/" + productID + "/board/defects/" + d.ID + "/status",
		}
		if d.StatusID == "" {
			stranded = append(stranded, card)
			continue
		}
		byStatus[d.StatusID] = append(byStatus[d.StatusID], card)
	}
	return byStatus, stranded
}

// mergedBoardColumns pairs requirement and defect statuses by position and
// returns a single column set covering both entity types. Used when
// Product.StatusSync is true (parallel sets guaranteed by the service layer).
func mergedBoardColumns(reqStatuses, defStatuses []*model.Status, reqCards, defCards map[string][]productpage.BoardCard) []productpage.BoardColumn {
	defByPosition := make(map[int]*model.Status, len(defStatuses))
	for _, s := range defStatuses {
		defByPosition[s.Position] = s
	}
	cols := make([]productpage.BoardColumn, 0, len(reqStatuses))
	for _, rs := range reqStatuses {
		col := productpage.BoardColumn{
			Name:        rs.Name,
			ReqStatusID: rs.ID,
			IsClosed:    rs.IsClosed,
		}
		if ds, ok := defByPosition[rs.Position]; ok {
			col.DefectStatusID = ds.ID
			col.Cards = append(col.Cards, defCards[ds.ID]...)
		}
		col.Cards = append(reqCards[rs.ID], col.Cards...)
		cols = append(cols, col)
	}
	return cols
}

// entityBoardColumns returns columns for a single entity type (split mode).
func entityBoardColumns(statuses []*model.Status, cards map[string][]productpage.BoardCard, forRequirements bool) []productpage.BoardColumn {
	cols := make([]productpage.BoardColumn, 0, len(statuses))
	for _, s := range statuses {
		col := productpage.BoardColumn{
			Name:     s.Name,
			IsClosed: s.IsClosed,
			Cards:    cards[s.ID],
		}
		if forRequirements {
			col.ReqStatusID = s.ID
		} else {
			col.DefectStatusID = s.ID
		}
		cols = append(cols, col)
	}
	return cols
}
