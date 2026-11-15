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
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx            store.Transactor
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
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt string
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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.milestones.Create(ctx, m); err != nil {
			return fmt.Errorf("create milestone: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "milestone", EntityID: m.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

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

	var m *model.Milestone
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		m, err = s.milestones.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, m.UpdatedAt); err != nil {
			return err
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

		if err := inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.milestones.Update(ctx, m); err != nil {
				return fmt.Errorf("update milestone: %w", err)
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), EntityType: "milestone", EntityID: m.ID,
				UserID: user.ID, Action: "update", CreatedAt: now,
			})
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
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

	return deleteOwned(ctx, s.tx, s.comments, s.audit, user, productID, model.OwnerTypeMilestone, id, s.milestones.Delete)
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
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt string
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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.sprints.Create(ctx, sprint); err != nil {
			return fmt.Errorf("create sprint: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "sprint", EntityID: sprint.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

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

	var sprint *model.Sprint
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		sprint, err = s.sprints.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, sprint.UpdatedAt); err != nil {
			return err
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
			return err
		}
		sprint.UpdatedAt = now

		if err := inTx(ctx, s.tx, func(ctx context.Context) error {
			if input.Status != nil && *input.Status != sprint.Status {
				// Status carries its own rules (one active sprint per product),
				// so it goes through the same path as the start/close actions.
				if err := s.applySprintStatus(ctx, sprint, *input.Status); err != nil {
					return err
				}
			}
			if err := s.sprints.Update(ctx, sprint); err != nil {
				return fmt.Errorf("update sprint: %w", err)
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), EntityType: "sprint", EntityID: sprint.ID,
				UserID: user.ID, Action: "update", CreatedAt: now,
			})
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return sprint, nil
}

// applySprintStatus validates a status change and writes it onto sprint. It
// does not persist — the caller does, so a status change can ride along with
// other edits in a single UPDATE. Call it inside the transaction that
// persists the sprint: starting a sprint takes the product's sprint lock, so
// two sprints cannot both pass the "none active" check (the partial unique
// index idx_sprints_active is the backstop).
func (s *MilestoneService) applySprintStatus(ctx context.Context, sprint *model.Sprint, status string) error {
	if !model.ValidSprintStatus(status) {
		return fmt.Errorf("unknown sprint status %q", status)
	}
	if status == model.SprintStatusActive {
		if err := lock(ctx, s.tx, lockSprints(sprint.ProductID)); err != nil {
			return err
		}
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
	now := time.Now().UTC().Format(time.RFC3339)
	sprint.UpdatedAt = now
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.applySprintStatus(ctx, sprint, status); err != nil {
			return err
		}
		if err := s.sprints.Update(ctx, sprint); err != nil {
			return fmt.Errorf("update sprint status: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "sprint", EntityID: sprint.ID,
			UserID: user.ID, Action: sprintStatusAction(status), CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.sprints.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete sprint: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "sprint", EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		})
	}); err != nil {
		return err
	}

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
	IsActive   bool
}

// UpdateStatusInput contains the fields that can be updated on a status.
type UpdateStatusInput struct {
	Name      *string
	Position  *int
	IsDefault *bool
	IsClosed  *bool
	IsActive  *bool
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
		IsActive:   input.IsActive,
		CreatedAt:  now,
	}
	// With status sync on, the status and its mirror for the other entity
	// type are created together, so the two sets cannot drift apart.
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.statuses.Create(ctx, status); err != nil {
			return fmt.Errorf("create status: %w", err)
		}
		if err := s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "status", EntityID: status.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		}); err != nil {
			return err
		}
		if !s.statusSync(ctx, input.ProductID) {
			return nil
		}
		sibling := *status
		sibling.ID = uuid.New().String()
		sibling.EntityType = otherEntityType(input.EntityType)
		if err := s.statuses.Create(ctx, &sibling); err != nil {
			return fmt.Errorf("status sync: create %s status: %w", sibling.EntityType, err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "status", EntityID: sibling.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	})
	if err != nil {
		return nil, err
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
	if input.IsActive != nil {
		status.IsActive = *input.IsActive
	}

	now := time.Now().UTC().Format(time.RFC3339)
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.statuses.Update(ctx, status); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		if err := s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "status", EntityID: status.ID,
			UserID: user.ID, Action: "update", CreatedAt: now,
		}); err != nil {
			return err
		}
		if !s.statusSync(ctx, productID) {
			return nil
		}
		otherType := otherEntityType(status.EntityType)
		sibling, err := s.statuses.GetByProductAndName(ctx, productID, otherType, oldName)
		if err != nil {
			// Nothing to mirror to: the sets already differ.
			slog.WarnContext(ctx, "status sync: sibling not found for update", "name", oldName, "entityType", otherType, "error", err)
			return nil
		}
		sibling.Name = status.Name
		sibling.Position = status.Position
		sibling.IsDefault = status.IsDefault
		sibling.IsClosed = status.IsClosed
		sibling.IsActive = status.IsActive
		if err := s.statuses.Update(ctx, sibling); err != nil {
			return fmt.Errorf("status sync: update %s status: %w", otherType, err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "status", EntityID: sibling.ID,
			UserID: user.ID, Action: "update", CreatedAt: now,
		})
	})
	if err != nil {
		return nil, err
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
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.statuses.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete status: %w", err)
		}
		if err := s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "status", EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		}); err != nil {
			return err
		}
		if !s.statusSync(ctx, productID) {
			return nil
		}
		otherType := otherEntityType(status.EntityType)
		sibling, err := s.statuses.GetByProductAndName(ctx, productID, otherType, status.Name)
		if err != nil {
			slog.WarnContext(ctx, "status sync: sibling not found for delete", "name", status.Name, "entityType", otherType, "error", err)
			return nil
		}
		if err := s.statuses.Delete(ctx, sibling.ID); err != nil {
			return fmt.Errorf("status sync: delete %s status: %w", otherType, err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "status", EntityID: sibling.ID,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		})
	})
}

// statusSync reports whether the product keeps requirement and defect
// statuses in step. A product that cannot be read is treated as not synced.
func (s *MilestoneService) statusSync(ctx context.Context, productID string) bool {
	product, err := s.products.GetByID(ctx, productID)
	return err == nil && product.StatusSync
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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.pyramidLevels.Create(ctx, pl); err != nil {
			return fmt.Errorf("create pyramid level: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "pyramid_level", EntityID: pl.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

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

	now := time.Now().UTC().Format(time.RFC3339)
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.pyramidLevels.Update(ctx, pl); err != nil {
			return fmt.Errorf("update pyramid level: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "pyramid_level", EntityID: pl.ID,
			UserID: user.ID, Action: "update", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.pyramidLevels.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete pyramid level: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "pyramid_level", EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		})
	}); err != nil {
		return err
	}

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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.runtimeEnvs.Create(ctx, env); err != nil {
			return fmt.Errorf("create runtime environment: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "runtime_environment", EntityID: env.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.runtimeEnvs.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete runtime environment: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "runtime_environment", EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		})
	}); err != nil {
		return err
	}

	return nil
}

// ListRuntimeEnvironments returns all runtime environments for a product. Requires viewer role.
func (s *MilestoneService) ListRuntimeEnvironments(ctx context.Context, user *model.User, productID string) ([]*model.RuntimeEnvironment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.runtimeEnvs.ListByProductID(ctx, productID)
}
