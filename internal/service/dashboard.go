package service

import (
	"context"
	"time"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// ProductSummary holds aggregated data for a single product on the dashboard.
type ProductSummary struct {
	Product          *model.Product
	Role             string // user's role in this product, or "admin" for admin bypass
	OpenRequirements int
	ActiveDefects    int
	CurrentSprint    string // name of the active sprint, or ""
}

// DashboardService provides data for the dashboard page.
type DashboardService struct {
	products     store.ProductStore
	productRoles store.ProductRoleStore
	requirements store.RequirementStore
	defects      store.DefectStore
	statuses     store.StatusStore
	sprints      store.SprintStore
}

// ProductSummaries returns a summary for each product the user can access.
func (s *DashboardService) ProductSummaries(ctx context.Context, user *model.User) ([]ProductSummary, error) {
	products, err := s.listProducts(ctx, user)
	if err != nil {
		return nil, err
	}

	summaries := make([]ProductSummary, 0, len(products))
	for _, p := range products {
		summary := ProductSummary{Product: p}

		// Resolve user's role.
		summary.Role = s.resolveRole(ctx, user, p.ID)

		// Count open requirements.
		closedStatuses, err := s.closedStatusIDs(ctx, p.ID)
		if err == nil {
			summary.OpenRequirements = s.countOpenRequirements(ctx, p.ID, closedStatuses)
			summary.ActiveDefects = s.countActiveDefects(ctx, p.ID, closedStatuses)
		}

		// Find current sprint (scrum mode only).
		if p.WorkflowMode == model.WorkflowModeScrum {
			summary.CurrentSprint = s.currentSprintName(ctx, p.ID)
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func (s *DashboardService) listProducts(ctx context.Context, user *model.User) ([]*model.Product, error) {
	if user.IsAdmin {
		return s.products.List(ctx)
	}

	roles, err := s.productRoles.ListByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	var products []*model.Product
	for _, role := range roles {
		p, err := s.products.GetByID(ctx, role.ProductID)
		if err != nil {
			continue
		}
		products = append(products, p)
	}
	return products, nil
}

func (s *DashboardService) resolveRole(ctx context.Context, user *model.User, productID string) string {
	if user.IsAdmin {
		role, err := s.productRoles.GetByUserAndProduct(ctx, user.ID, productID)
		if err == nil {
			return role.Role
		}
		return "admin"
	}

	role, err := s.productRoles.GetByUserAndProduct(ctx, user.ID, productID)
	if err != nil {
		return ""
	}
	return role.Role
}

func (s *DashboardService) closedStatusIDs(ctx context.Context, productID string) (map[string]bool, error) {
	closed := make(map[string]bool)

	reqStatuses, err := s.statuses.ListByProductID(ctx, productID, "requirement")
	if err != nil {
		return nil, err
	}
	for _, st := range reqStatuses {
		if st.IsClosed {
			closed[st.ID] = true
		}
	}

	defStatuses, err := s.statuses.ListByProductID(ctx, productID, "defect")
	if err != nil {
		return nil, err
	}
	for _, st := range defStatuses {
		if st.IsClosed {
			closed[st.ID] = true
		}
	}

	return closed, nil
}

func (s *DashboardService) countOpenRequirements(ctx context.Context, productID string, closed map[string]bool) int {
	reqs, err := s.requirements.ListByProductID(ctx, productID, store.RequirementFilter{
		Kind: model.RequirementKindRequirement,
	})
	if err != nil {
		return 0
	}
	count := 0
	for _, r := range reqs {
		if !closed[r.StatusID] {
			count++
		}
	}
	return count
}

func (s *DashboardService) countActiveDefects(ctx context.Context, productID string, closed map[string]bool) int {
	defects, err := s.defects.ListByProductID(ctx, productID, store.DefectFilter{})
	if err != nil {
		return 0
	}
	count := 0
	for _, d := range defects {
		if !closed[d.StatusID] {
			count++
		}
	}
	return count
}

func (s *DashboardService) currentSprintName(ctx context.Context, productID string) string {
	sprints, err := s.sprints.ListByProductID(ctx, productID)
	if err != nil {
		return ""
	}

	now := time.Now().UTC()
	for _, sp := range sprints {
		start, err1 := time.Parse(time.RFC3339, sp.StartDate)
		end, err2 := time.Parse(time.RFC3339, sp.EndDate)
		if err1 != nil || err2 != nil {
			continue
		}
		if !now.Before(start) && now.Before(end) {
			return sp.Name
		}
	}
	return ""
}
