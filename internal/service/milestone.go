package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// MilestoneService handles milestones, sprints, statuses, pyramid levels, and runtime environments.
type MilestoneService struct {
	milestones    store.MilestoneStore
	sprints       store.SprintStore
	statuses      store.StatusStore
	products      store.ProductStore
	pyramidLevels store.PyramidLevelStore
	runtimeEnvs   store.RuntimeEnvironmentStore
	comments      ownerPurger
	audit         store.AuditStore
	authz         *Authorizer
	owns          *ownership
}

// --- Milestone ---

// CreateMilestoneInput contains the fields required to create a milestone.
type CreateMilestoneInput struct {
	ProductID   string
	Name        string
	Description string
	Type        string
	DueDate     string
}

// UpdateMilestoneInput contains the fields that can be updated on a milestone.
type UpdateMilestoneInput struct {
	Name        *string
	Description *string
	Type        *string
	DueDate     *string
}

// CreateMilestone creates a new milestone. Requires owner role.
func (s *MilestoneService) CreateMilestone(ctx context.Context, user *model.User, input CreateMilestoneInput) (*model.Milestone, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("milestone name is required")
	}
	if input.Type == "" {
		input.Type = model.MilestoneTypePlanning
	}

	now := time.Now().UTC().Format(time.RFC3339)
	m := &model.Milestone{
		ID:          uuid.New().String(),
		ProductID:   input.ProductID,
		Name:        input.Name,
		Description: input.Description,
		Type:        input.Type,
		DueDate:     input.DueDate,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.milestones.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("create milestone: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "milestone", EntityID: m.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return m, nil
}

// GetMilestone returns a milestone by ID. Requires viewer role.
func (s *MilestoneService) GetMilestone(ctx context.Context, user *model.User, productID, id string) (*model.Milestone, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindMilestone, id}); err != nil {
		return nil, err
	}
	return s.milestones.GetByID(ctx, id)
}

// UpdateMilestone modifies a milestone. Requires owner role.
func (s *MilestoneService) UpdateMilestone(ctx context.Context, user *model.User, productID, id string, input UpdateMilestoneInput) (*model.Milestone, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindMilestone, id}); err != nil {
		return nil, err
	}

	m, err := s.milestones.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if input.Name != nil {
		m.Name = *input.Name
	}
	if input.Description != nil {
		m.Description = *input.Description
	}
	if input.Type != nil {
		m.Type = *input.Type
	}
	if input.DueDate != nil {
		m.DueDate = *input.DueDate
	}
	m.UpdatedAt = now

	if err := s.milestones.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update milestone: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "milestone", EntityID: m.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return m, nil
}

// DeleteMilestone removes a milestone. Requires owner role.
func (s *MilestoneService) DeleteMilestone(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindMilestone, id}); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.milestones.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete milestone: %w", err)
	}

	// Comments and attachments hang off polymorphic owner columns, so no
	// foreign key cascades them away with the row above.
	if err := s.comments.PurgeOwner(ctx, productID, model.OwnerTypeMilestone, id); err != nil {
		return err
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "milestone", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListMilestones returns all milestones for a product. Requires viewer role.
func (s *MilestoneService) ListMilestones(ctx context.Context, user *model.User, productID string) ([]*model.Milestone, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.milestones.ListByProductID(ctx, productID)
}

// --- Sprint ---

// CreateSprintInput contains the fields required to create a sprint.
type CreateSprintInput struct {
	ProductID   string
	Name        string
	MilestoneID string
	StartDate   string
	EndDate     string
}

// UpdateSprintInput contains the fields that can be updated on a sprint.
type UpdateSprintInput struct {
	Name        *string
	MilestoneID *string
	Status      *string
	StartDate   *string
	EndDate     *string
}

// validateSprintDates checks that both dates are present and ordered. Both are
// RFC 3339, so lexical comparison is chronological.
func validateSprintDates(startDate, endDate string) error {
	if startDate == "" || endDate == "" {
		return errors.New("sprint start and end date are required")
	}
	if endDate < startDate {
		return errors.New("sprint end date must not precede its start date")
	}
	return nil
}

// CreateSprint creates a new sprint. Requires owner role.
func (s *MilestoneService) CreateSprint(ctx context.Context, user *model.User, input CreateSprintInput) (*model.Sprint, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, input.ProductID, ref{kindMilestone, input.MilestoneID}); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("sprint name is required")
	}
	if err := validateSprintDates(input.StartDate, input.EndDate); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sprint := &model.Sprint{
		ID:          uuid.New().String(),
		ProductID:   input.ProductID,
		Name:        input.Name,
		MilestoneID: input.MilestoneID,
		Status:      model.SprintStatusPlanned,
		StartDate:   input.StartDate,
		EndDate:     input.EndDate,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.sprints.Create(ctx, sprint); err != nil {
		return nil, fmt.Errorf("create sprint: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "sprint", EntityID: sprint.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return sprint, nil
}

// GetSprint returns a sprint by ID. Requires viewer role.
func (s *MilestoneService) GetSprint(ctx context.Context, user *model.User, productID, id string) (*model.Sprint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindSprint, id}); err != nil {
		return nil, err
	}
	return s.sprints.GetByID(ctx, id)
}

// UpdateSprint modifies a sprint. Requires owner role.
func (s *MilestoneService) UpdateSprint(ctx context.Context, user *model.User, productID, id string, input UpdateSprintInput) (*model.Sprint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindSprint, id}); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindMilestone, opt(input.MilestoneID)}); err != nil {
		return nil, err
	}

	sprint, err := s.sprints.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if input.Name != nil {
		sprint.Name = *input.Name
	}
	if input.MilestoneID != nil {
		sprint.MilestoneID = *input.MilestoneID
	}
	if input.StartDate != nil {
		sprint.StartDate = *input.StartDate
	}
	if input.EndDate != nil {
		sprint.EndDate = *input.EndDate
	}
	if err := validateSprintDates(sprint.StartDate, sprint.EndDate); err != nil {
		return nil, err
	}
	if input.Status != nil && *input.Status != sprint.Status {
		// Status carries its own rules (one active sprint per product), so it
		// goes through the same path as the start/close actions.
		if err := s.applySprintStatus(ctx, sprint, *input.Status); err != nil {
			return nil, err
		}
	}
	sprint.UpdatedAt = now

	if err := s.sprints.Update(ctx, sprint); err != nil {
		return nil, fmt.Errorf("update sprint: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "sprint", EntityID: sprint.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return sprint, nil
}

// applySprintStatus validates a status change and writes it onto sprint. It
// does not persist — the caller does, so a status change can ride along with
// other edits in a single UPDATE.
func (s *MilestoneService) applySprintStatus(ctx context.Context, sprint *model.Sprint, status string) error {
	if !model.ValidSprintStatus(status) {
		return fmt.Errorf("unknown sprint status %q", status)
	}
	if status == model.SprintStatusActive {
		active, err := s.sprints.GetActiveByProductID(ctx, sprint.ProductID)
		if err != nil {
			return fmt.Errorf("check active sprint: %w", err)
		}
		if active != nil && active.ID != sprint.ID {
			return fmt.Errorf("sprint %q is already active; close it first", active.Name)
		}
	}
	sprint.Status = status
	return nil
}

// SetSprintStatus moves a sprint through planned -> active -> completed.
// At most one sprint per product may be active at a time. Requires owner role.
// The audit action mirrors the transition ("start" / "close"), so the sprint's
// history reads as a lifecycle rather than a series of anonymous updates.
func (s *MilestoneService) SetSprintStatus(ctx context.Context, user *model.User, productID, id, status string) (*model.Sprint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindSprint, id}); err != nil {
		return nil, err
	}

	sprint, err := s.sprints.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sprint.ProductID != productID {
		return nil, errors.New("sprint not found")
	}
	if err := s.applySprintStatus(ctx, sprint, status); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sprint.UpdatedAt = now
	if err := s.sprints.Update(ctx, sprint); err != nil {
		return nil, fmt.Errorf("update sprint status: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "sprint", EntityID: sprint.ID,
		UserID: user.ID, Action: sprintStatusAction(status), CreatedAt: now,
	})

	return sprint, nil
}

func sprintStatusAction(status string) string {
	switch status {
	case model.SprintStatusActive:
		return "start"
	case model.SprintStatusCompleted:
		return "close"
	default:
		return "reopen"
	}
}

// GetActiveSprint returns the product's active sprint, or nil when none is
// active. Requires viewer role.
func (s *MilestoneService) GetActiveSprint(ctx context.Context, user *model.User, productID string) (*model.Sprint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.sprints.GetActiveByProductID(ctx, productID)
}

// DeleteSprint removes a sprint. Requires owner role.
func (s *MilestoneService) DeleteSprint(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindSprint, id}); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.sprints.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete sprint: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "sprint", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListSprints returns all sprints for a product. Requires viewer role.
func (s *MilestoneService) ListSprints(ctx context.Context, user *model.User, productID string) ([]*model.Sprint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.sprints.ListByProductID(ctx, productID)
}

// ListSprintsByMilestone returns the sprints targeting a milestone.
// Rows belonging to another product are skipped. Requires viewer role.
func (s *MilestoneService) ListSprintsByMilestone(ctx context.Context, user *model.User, productID, milestoneID string) ([]*model.Sprint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	all, err := s.sprints.ListByMilestoneID(ctx, milestoneID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Sprint, 0, len(all))
	for _, sp := range all {
		if sp.ProductID != productID {
			continue
		}
		out = append(out, sp)
	}
	return out, nil
}

// --- Status ---

// otherEntityType returns the counterpart entity type for status sync.
func otherEntityType(entityType string) string {
	if entityType == "requirement" {
		return "defect"
	}
	return "requirement"
}

// CreateStatusInput contains the fields required to create a status.
type CreateStatusInput struct {
	ProductID  string
	EntityType string
	Name       string
	Position   int
	IsDefault  bool
	IsClosed   bool
}

// UpdateStatusInput contains the fields that can be updated on a status.
type UpdateStatusInput struct {
	Name      *string
	Position  *int
	IsDefault *bool
	IsClosed  *bool
}

// CreateStatus creates a new status. Requires owner role.
func (s *MilestoneService) CreateStatus(ctx context.Context, user *model.User, input CreateStatusInput) (*model.Status, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("status name is required")
	}
	if input.EntityType == "" {
		return nil, errors.New("entity type is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	status := &model.Status{
		ID:         uuid.New().String(),
		ProductID:  input.ProductID,
		EntityType: input.EntityType,
		Name:       input.Name,
		Position:   input.Position,
		IsDefault:  input.IsDefault,
		IsClosed:   input.IsClosed,
		CreatedAt:  now,
	}
	if err := s.statuses.Create(ctx, status); err != nil {
		return nil, fmt.Errorf("create status: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "status", EntityID: status.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	// Sync: mirror to the other entity type if product has status_sync enabled.
	product, err := s.products.GetByID(ctx, input.ProductID)
	if err == nil && product.StatusSync {
		sibling := &model.Status{
			ID:         uuid.New().String(),
			ProductID:  input.ProductID,
			EntityType: otherEntityType(input.EntityType),
			Name:       input.Name,
			Position:   input.Position,
			IsDefault:  input.IsDefault,
			IsClosed:   input.IsClosed,
			CreatedAt:  now,
		}
		if err := s.statuses.Create(ctx, sibling); err != nil {
			slog.Warn("status sync: failed to create sibling", "error", err)
		} else {
			_ = s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), EntityType: "status", EntityID: sibling.ID,
				UserID: user.ID, Action: "create", CreatedAt: now,
			})
		}
	}

	return status, nil
}

// GetStatus returns a status by ID. Requires viewer role.
func (s *MilestoneService) GetStatus(ctx context.Context, user *model.User, productID, id string) (*model.Status, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindStatus, id}); err != nil {
		return nil, err
	}
	return s.statuses.GetByID(ctx, id)
}

// UpdateStatus modifies a status. Requires owner role.
func (s *MilestoneService) UpdateStatus(ctx context.Context, user *model.User, productID, id string, input UpdateStatusInput) (*model.Status, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindStatus, id}); err != nil {
		return nil, err
	}

	status, err := s.statuses.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Capture old name before mutation for sibling lookup.
	oldName := status.Name

	if input.Name != nil {
		status.Name = *input.Name
	}
	if input.Position != nil {
		status.Position = *input.Position
	}
	if input.IsDefault != nil {
		status.IsDefault = *input.IsDefault
	}
	if input.IsClosed != nil {
		status.IsClosed = *input.IsClosed
	}

	if err := s.statuses.Update(ctx, status); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "status", EntityID: status.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	// Sync: mirror changes to sibling if product has status_sync enabled.
	product, err := s.products.GetByID(ctx, productID)
	if err == nil && product.StatusSync {
		otherType := otherEntityType(status.EntityType)
		sibling, err := s.statuses.GetByProductAndName(ctx, productID, otherType, oldName)
		if err != nil {
			slog.Warn("status sync: sibling not found for update", "name", oldName, "entityType", otherType, "error", err)
		} else {
			sibling.Name = status.Name
			sibling.Position = status.Position
			sibling.IsDefault = status.IsDefault
			sibling.IsClosed = status.IsClosed
			if err := s.statuses.Update(ctx, sibling); err != nil {
				slog.Warn("status sync: failed to update sibling", "error", err)
			} else {
				_ = s.audit.Create(ctx, &model.AuditEntry{
					ID: uuid.New().String(), EntityType: "status", EntityID: sibling.ID,
					UserID: user.ID, Action: "update", CreatedAt: now,
				})
			}
		}
	}

	return status, nil
}

// DeleteStatus removes a status. Requires owner role.
func (s *MilestoneService) DeleteStatus(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindStatus, id}); err != nil {
		return err
	}

	// Get the status before deletion for sync lookup.
	status, err := s.statuses.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delete status: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.statuses.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete status: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "status", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	// Sync: delete sibling if product has status_sync enabled.
	product, err := s.products.GetByID(ctx, productID)
	if err == nil && product.StatusSync {
		otherType := otherEntityType(status.EntityType)
		sibling, err := s.statuses.GetByProductAndName(ctx, productID, otherType, status.Name)
		if err != nil {
			slog.Warn("status sync: sibling not found for delete", "name", status.Name, "entityType", otherType, "error", err)
		} else {
			if err := s.statuses.Delete(ctx, sibling.ID); err != nil {
				slog.Warn("status sync: failed to delete sibling", "error", err)
			} else {
				_ = s.audit.Create(ctx, &model.AuditEntry{
					ID: uuid.New().String(), EntityType: "status", EntityID: sibling.ID,
					UserID: user.ID, Action: "delete", CreatedAt: now,
				})
			}
		}
	}

	return nil
}

// ListStatuses returns all statuses for a product and entity type. Requires viewer role.
func (s *MilestoneService) ListStatuses(ctx context.Context, user *model.User, productID, entityType string) ([]*model.Status, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.statuses.ListByProductID(ctx, productID, entityType)
}

// --- Pyramid Level ---

// CreatePyramidLevelInput contains the fields required to create a pyramid level.
type CreatePyramidLevelInput struct {
	ProductID string
	ParentID  string
	Name      string
	Position  int
}

// UpdatePyramidLevelInput contains the fields that can be updated on a pyramid level.
type UpdatePyramidLevelInput struct {
	Name     *string
	Position *int
}

// CreatePyramidLevel creates a new pyramid level. Requires owner role.
func (s *MilestoneService) CreatePyramidLevel(ctx context.Context, user *model.User, input CreatePyramidLevelInput) (*model.PyramidLevel, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, input.ProductID, ref{kindPyramidLevel, input.ParentID}); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("pyramid level name is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	pl := &model.PyramidLevel{
		ID:        uuid.New().String(),
		ProductID: input.ProductID,
		ParentID:  input.ParentID,
		Name:      input.Name,
		Position:  input.Position,
		CreatedAt: now,
	}
	if err := s.pyramidLevels.Create(ctx, pl); err != nil {
		return nil, fmt.Errorf("create pyramid level: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "pyramid_level", EntityID: pl.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return pl, nil
}

// GetPyramidLevel returns a pyramid level by ID. Requires viewer role.
func (s *MilestoneService) GetPyramidLevel(ctx context.Context, user *model.User, productID, id string) (*model.PyramidLevel, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindPyramidLevel, id}); err != nil {
		return nil, err
	}
	return s.pyramidLevels.GetByID(ctx, id)
}

// UpdatePyramidLevel modifies a pyramid level. Requires owner role.
func (s *MilestoneService) UpdatePyramidLevel(ctx context.Context, user *model.User, productID, id string, input UpdatePyramidLevelInput) (*model.PyramidLevel, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindPyramidLevel, id}); err != nil {
		return nil, err
	}

	pl, err := s.pyramidLevels.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		pl.Name = *input.Name
	}
	if input.Position != nil {
		pl.Position = *input.Position
	}

	if err := s.pyramidLevels.Update(ctx, pl); err != nil {
		return nil, fmt.Errorf("update pyramid level: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "pyramid_level", EntityID: pl.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return pl, nil
}

// DeletePyramidLevel removes a pyramid level. Requires owner role.
func (s *MilestoneService) DeletePyramidLevel(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindPyramidLevel, id}); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.pyramidLevels.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete pyramid level: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "pyramid_level", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListPyramidLevels returns all pyramid levels for a product. Requires viewer role.
func (s *MilestoneService) ListPyramidLevels(ctx context.Context, user *model.User, productID string) ([]*model.PyramidLevel, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.pyramidLevels.ListByProductID(ctx, productID)
}

// --- Runtime Environment ---

// CreateRuntimeEnvironment creates a new runtime environment. Requires owner role.
func (s *MilestoneService) CreateRuntimeEnvironment(ctx context.Context, user *model.User, productID, name string) (*model.RuntimeEnvironment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, errors.New("runtime environment name is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	env := &model.RuntimeEnvironment{
		ID:        uuid.New().String(),
		ProductID: productID,
		Name:      name,
		CreatedAt: now,
	}
	if err := s.runtimeEnvs.Create(ctx, env); err != nil {
		return nil, fmt.Errorf("create runtime environment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "runtime_environment", EntityID: env.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return env, nil
}

// DeleteRuntimeEnvironment removes a runtime environment. Requires owner role.
func (s *MilestoneService) DeleteRuntimeEnvironment(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindEnvironment, id}); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.runtimeEnvs.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete runtime environment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "runtime_environment", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListRuntimeEnvironments returns all runtime environments for a product. Requires viewer role.
func (s *MilestoneService) ListRuntimeEnvironments(ctx context.Context, user *model.User, productID string) ([]*model.RuntimeEnvironment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.runtimeEnvs.ListByProductID(ctx, productID)
}
