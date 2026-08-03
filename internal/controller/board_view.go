package controller

import (
	"nexmap.net/internal/model"
	"nexmap.net/web/templates"
)

// This file holds the projection from domain entities onto board columns and
// cards. Three surfaces share it: the product board, the defects board and the
// sprint detail's board tab.

// buildBoardSurface projects requirements, tasks and defects onto columns,
// honouring the product's StatusSync setting (one merged board vs two tabbed
// ones). Pass nil for either entity slice to leave that type off the board.
func buildBoardSurface(
	productID string,
	p *model.Product,
	canMove bool,
	reqStatuses, defStatuses []*model.Status,
	items []*model.Requirement,
	defects []*model.Defect,
	prefixes map[string]string,
	groupPrefix string,
) templates.BoardSurfaceProps {
	props := templates.BoardSurfaceProps{
		ProductID:   productID,
		CanMove:     canMove,
		GroupPrefix: groupPrefix,
	}

	reqCards, strandedReq, strandedTask := buildBacklogCards(productID, items, prefixes)
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
	return props
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
func buildBacklogCards(productID string, items []*model.Requirement, prefixes map[string]string) (byStatus map[string][]templates.BoardCard, strandedReq, strandedTask []templates.BoardCard) {
	byStatus = make(map[string][]templates.BoardCard)
	for _, item := range items {
		card := backlogCard(productID, item, prefixes)
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

// backlogCard builds a single requirement/task card whose MoveURL points at the
// product-level status endpoint. A status change is the same operation whichever
// board triggered it, so every board reuses those handlers.
func backlogCard(productID string, item *model.Requirement, prefixes map[string]string) templates.BoardCard {
	segment := "requirements"
	if item.Kind == model.RequirementKindTask {
		segment = "tasks"
	}
	return templates.BoardCard{
		EntityType: item.Kind,
		ID:         item.ID,
		Identifier: model.EntityIdentifier(categoryPrefix(prefixes, item.CategoryID), item.SequenceNumber),
		Title:      item.Title,
		Priority:   item.Priority,
		MoveURL:    "/products/" + productID + "/board/" + segment + "/" + item.ID + "/status",
	}
}

func buildDefectCards(productID string, defects []*model.Defect) (byStatus map[string][]templates.BoardCard, stranded []templates.BoardCard) {
	byStatus = make(map[string][]templates.BoardCard)
	for _, d := range defects {
		card := defectCard(productID, d)
		if d.StatusID == "" {
			stranded = append(stranded, card)
			continue
		}
		byStatus[d.StatusID] = append(byStatus[d.StatusID], card)
	}
	return byStatus, stranded
}

func defectCard(productID string, d *model.Defect) templates.BoardCard {
	return templates.BoardCard{
		EntityType: "defect",
		ID:         d.ID,
		Title:      d.Title,
		Priority:   d.Priority,
		MoveURL:    "/products/" + productID + "/board/defects/" + d.ID + "/status",
	}
}

// mergedBoardColumns pairs requirement and defect statuses by position and
// returns a single column set covering both entity types. Used when
// Product.StatusSync is true (parallel sets guaranteed by the service layer).
func mergedBoardColumns(reqStatuses, defStatuses []*model.Status, reqCards, defCards map[string][]templates.BoardCard) []templates.BoardColumn {
	defByPosition := make(map[int]*model.Status, len(defStatuses))
	for _, s := range defStatuses {
		defByPosition[s.Position] = s
	}
	cols := make([]templates.BoardColumn, 0, len(reqStatuses))
	for _, rs := range reqStatuses {
		col := templates.BoardColumn{
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
func entityBoardColumns(statuses []*model.Status, cards map[string][]templates.BoardCard, forRequirements bool) []templates.BoardColumn {
	cols := make([]templates.BoardColumn, 0, len(statuses))
	for _, s := range statuses {
		col := templates.BoardColumn{
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
