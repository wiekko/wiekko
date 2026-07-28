package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// campaignRetester reopens campaign results for re-testing once the defect that
// failed them is closed. Narrow interface so the backlog service does not depend
// on the whole testing service (FR-504).
type campaignRetester interface {
	MarkResultsForRetest(ctx context.Context, defectID, userID string) error
}

// BacklogService handles requirements and defects.
type BacklogService struct {
	requirements store.RequirementStore
	defects      store.DefectStore
	categories   store.CategoryStore
	statuses     store.StatusStore
	comments     ownerPurger
	retester     campaignRetester
	audit        store.AuditStore
	authz        *Authorizer
}

// --- Requirements ---

// CreateRequirementInput contains the fields required to create a requirement.
type CreateRequirementInput struct {
	ProductID        string
	ParentID         string
	CategoryID       string
	Title            string
	Description      string
	DefinitionOfDone string
	StatusID         string
	MilestoneID      string
	SprintID         string
	Estimation       string
	Priority         int
	IsCoarse         bool
}

// UpdateRequirementInput contains the fields that can be updated on a requirement.
type UpdateRequirementInput struct {
	ParentID         *string
	CategoryID       *string
	Title            *string
	Description      *string
	DefinitionOfDone *string
	StatusID         *string
	MilestoneID      *string
	SprintID         *string
	Estimation       *string
	Priority         *int
	IsCoarse         *bool
	IsActive         *bool
}

// CreateRequirement creates a new requirement. Requires owner role.
func (s *BacklogService) CreateRequirement(ctx context.Context, user *model.User, input CreateRequirementInput) (*model.Requirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Title == "" {
		return nil, errors.New("requirement title is required")
	}
	if input.CategoryID == "" {
		cats, err := s.categories.ListByProductID(ctx, input.ProductID, "requirement")
		if err != nil {
			return nil, fmt.Errorf("list categories: %w", err)
		}
		if len(cats) == 0 {
			return nil, errors.New("no requirement categories defined for this product")
		}
		input.CategoryID = cats[0].ID
	}

	seq, err := s.requirements.NextSequenceNumber(ctx, input.ProductID)
	if err != nil {
		return nil, fmt.Errorf("next sequence number: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	r := &model.Requirement{
		ID:               uuid.New().String(),
		ProductID:        input.ProductID,
		ParentID:         input.ParentID,
		CategoryID:       input.CategoryID,
		SequenceNumber:   seq,
		Title:            input.Title,
		Description:      input.Description,
		DefinitionOfDone: input.DefinitionOfDone,
		StatusID:         input.StatusID,
		MilestoneID:      input.MilestoneID,
		SprintID:         input.SprintID,
		Estimation:       input.Estimation,
		Priority:         input.Priority,
		IsCoarse:         input.IsCoarse,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.requirements.Create(ctx, r); err != nil {
		return nil, fmt.Errorf("create requirement: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "requirement", EntityID: r.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return r, nil
}

// GetRequirement returns a requirement by ID. Requires viewer role.
func (s *BacklogService) GetRequirement(ctx context.Context, user *model.User, productID, id string) (*model.Requirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.requirements.GetByID(ctx, id)
}

// UpdateRequirement modifies a requirement. Developer can update status/is_active (FR-407);
// full field updates require owner.
func (s *BacklogService) UpdateRequirement(ctx context.Context, user *model.User, productID, id string, input UpdateRequirementInput) (*model.Requirement, error) {
	role, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper)
	if err != nil {
		return nil, err
	}

	r, err := s.requirements.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	isDeveloper := roleRank(role) < roleRank(model.RoleOwner)

	now := time.Now().UTC().Format(time.RFC3339)
	if input.StatusID != nil {
		r.StatusID = *input.StatusID
	}
	if input.IsActive != nil {
		r.IsActive = *input.IsActive
	}

	// Fields that require owner role.
	if input.ParentID != nil || input.CategoryID != nil || input.Title != nil || input.Description != nil ||
		input.DefinitionOfDone != nil || input.MilestoneID != nil ||
		input.SprintID != nil || input.Estimation != nil ||
		input.Priority != nil || input.IsCoarse != nil {
		if isDeveloper {
			return nil, fmt.Errorf("%w: requires owner role for full updates", ErrForbidden)
		}
		if input.ParentID != nil {
			r.ParentID = *input.ParentID
		}
		if input.CategoryID != nil {
			r.CategoryID = *input.CategoryID
		}
		if input.Title != nil {
			r.Title = *input.Title
		}
		if input.Description != nil {
			r.Description = *input.Description
		}
		if input.DefinitionOfDone != nil {
			r.DefinitionOfDone = *input.DefinitionOfDone
		}
		if input.MilestoneID != nil {
			r.MilestoneID = *input.MilestoneID
		}
		if input.SprintID != nil {
			r.SprintID = *input.SprintID
		}
		if input.Estimation != nil {
			r.Estimation = *input.Estimation
		}
		if input.Priority != nil {
			r.Priority = *input.Priority
		}
		if input.IsCoarse != nil {
			r.IsCoarse = *input.IsCoarse
		}
	}
	r.UpdatedAt = now

	if err := s.requirements.Update(ctx, r); err != nil {
		return nil, fmt.Errorf("update requirement: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "requirement", EntityID: r.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return r, nil
}

// DeleteRequirement removes a requirement. Requires owner role.
func (s *BacklogService) DeleteRequirement(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.requirements.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete requirement: %w", err)
	}

	// Comments and attachments hang off polymorphic owner columns, so no
	// foreign key cascades them away with the row above.
	if err := s.comments.PurgeOwner(ctx, productID, model.OwnerTypeRequirement, id); err != nil {
		return err
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "requirement", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListRequirements returns requirements for a product. Requires viewer role.
func (s *BacklogService) ListRequirements(ctx context.Context, user *model.User, productID string, filter store.RequirementFilter) ([]*model.Requirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.requirements.ListByProductID(ctx, productID, filter)
}

// ListChildRequirements returns child requirements. Requires viewer role.
// ListRequirementsByMilestone returns the requirements assigned to a milestone.
// Rows belonging to another product are skipped. Requires viewer role.
func (s *BacklogService) ListRequirementsByMilestone(ctx context.Context, user *model.User, productID, milestoneID string) ([]*model.Requirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	all, err := s.requirements.ListByMilestoneID(ctx, milestoneID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Requirement, 0, len(all))
	for _, r := range all {
		if r.ProductID != productID {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *BacklogService) ListChildRequirements(ctx context.Context, user *model.User, productID, parentID string) ([]*model.Requirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.requirements.ListByParentID(ctx, parentID)
}

// AssignRequirement assigns a user to a requirement. Requires owner role.
func (s *BacklogService) AssignRequirement(ctx context.Context, user *model.User, productID, reqID, targetUserID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return s.requirements.Assign(ctx, &model.Assignment{
		ID:        uuid.New().String(),
		TargetID:  reqID,
		UserID:    targetUserID,
		CreatedAt: now,
	})
}

// UnassignRequirement removes a user from a requirement. Requires owner role.
func (s *BacklogService) UnassignRequirement(ctx context.Context, user *model.User, productID, reqID, targetUserID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	return s.requirements.Unassign(ctx, reqID, targetUserID)
}

// ListRequirementAssignees returns assignees for a requirement. Requires viewer role.
func (s *BacklogService) ListRequirementAssignees(ctx context.Context, user *model.User, productID, reqID string) ([]*model.Assignment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.requirements.ListAssignees(ctx, reqID)
}

// --- Defects ---

// CreateDefectInput contains the fields required to create a defect.
type CreateDefectInput struct {
	ProductID   string
	Title       string
	Description string
	Criticality string
	StatusID    string
	MilestoneID string
	SprintID    string
	Estimation  string
	Priority    int
}

// UpdateDefectInput contains the fields that can be updated on a defect.
type UpdateDefectInput struct {
	Title       *string
	Description *string
	Criticality *string
	StatusID    *string
	MilestoneID *string
	SprintID    *string
	Estimation  *string
	Priority    *int
	IsActive    *bool
}

// CreateDefect creates a new defect. Requires tester role.
func (s *BacklogService) CreateDefect(ctx context.Context, user *model.User, input CreateDefectInput) (*model.Defect, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleTester); err != nil {
		return nil, err
	}

	if input.Title == "" {
		return nil, errors.New("defect title is required")
	}
	if input.Criticality == "" {
		input.Criticality = model.DefectCriticalityMedium
	}

	now := time.Now().UTC().Format(time.RFC3339)
	d := &model.Defect{
		ID:          uuid.New().String(),
		ProductID:   input.ProductID,
		Title:       input.Title,
		Description: input.Description,
		Criticality: input.Criticality,
		StatusID:    input.StatusID,
		MilestoneID: input.MilestoneID,
		SprintID:    input.SprintID,
		Estimation:  input.Estimation,
		Priority:    input.Priority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.defects.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("create defect: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "defect", EntityID: d.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return d, nil
}

// GetDefect returns a defect by ID. Requires viewer role.
func (s *BacklogService) GetDefect(ctx context.Context, user *model.User, productID, id string) (*model.Defect, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.defects.GetByID(ctx, id)
}

// UpdateDefect modifies a defect. Developer can update status/is_active (FR-407);
// full field updates require owner.
func (s *BacklogService) UpdateDefect(ctx context.Context, user *model.User, productID, id string, input UpdateDefectInput) (*model.Defect, error) {
	role, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper)
	if err != nil {
		return nil, err
	}

	d, err := s.defects.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	isDeveloper := roleRank(role) < roleRank(model.RoleOwner)

	now := time.Now().UTC().Format(time.RFC3339)
	previousStatusID := d.StatusID
	if input.StatusID != nil {
		d.StatusID = *input.StatusID
	}
	if input.IsActive != nil {
		d.IsActive = *input.IsActive
	}

	// Fields that require owner role.
	if input.Title != nil || input.Description != nil || input.Criticality != nil ||
		input.MilestoneID != nil || input.SprintID != nil ||
		input.Estimation != nil || input.Priority != nil {
		if isDeveloper {
			return nil, fmt.Errorf("%w: requires owner role for full updates", ErrForbidden)
		}
		if input.Title != nil {
			d.Title = *input.Title
		}
		if input.Description != nil {
			d.Description = *input.Description
		}
		if input.Criticality != nil {
			d.Criticality = *input.Criticality
		}
		if input.MilestoneID != nil {
			d.MilestoneID = *input.MilestoneID
		}
		if input.SprintID != nil {
			d.SprintID = *input.SprintID
		}
		if input.Estimation != nil {
			d.Estimation = *input.Estimation
		}
		if input.Priority != nil {
			d.Priority = *input.Priority
		}
	}
	d.UpdatedAt = now

	if err := s.defects.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("update defect: %w", err)
	}

	if s.defectJustClosed(ctx, previousStatusID, d.StatusID) && s.retester != nil {
		if err := s.retester.MarkResultsForRetest(ctx, d.ID, user.ID); err != nil {
			return nil, err
		}
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "defect", EntityID: d.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return d, nil
}

// defectJustClosed reports whether a status change moved a defect into a closed
// state. Statuses are configured per product, so "closed" is whatever the
// product marked as such.
func (s *BacklogService) defectJustClosed(ctx context.Context, fromID, toID string) bool {
	if s.statuses == nil || toID == "" || fromID == toID {
		return false
	}
	to, err := s.statuses.GetByID(ctx, toID)
	if err != nil || !to.IsClosed {
		return false
	}
	if fromID == "" {
		return true
	}
	from, err := s.statuses.GetByID(ctx, fromID)
	// An unreadable previous status is treated as open: re-testing a result
	// that was already settled is recoverable, silently skipping it is not.
	return err != nil || !from.IsClosed
}

// DeleteDefect removes a defect. Requires owner role.
func (s *BacklogService) DeleteDefect(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.defects.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete defect: %w", err)
	}

	// Comments and attachments hang off polymorphic owner columns, so no
	// foreign key cascades them away with the row above.
	if err := s.comments.PurgeOwner(ctx, productID, model.OwnerTypeDefect, id); err != nil {
		return err
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "defect", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListDefects returns defects for a product. Requires viewer role.
func (s *BacklogService) ListDefects(ctx context.Context, user *model.User, productID string, filter store.DefectFilter) ([]*model.Defect, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.defects.ListByProductID(ctx, productID, filter)
}

// ListDefectsByMilestone returns the defects assigned to a milestone.
// Rows belonging to another product are skipped. Requires viewer role.
func (s *BacklogService) ListDefectsByMilestone(ctx context.Context, user *model.User, productID, milestoneID string) ([]*model.Defect, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	all, err := s.defects.ListByMilestoneID(ctx, milestoneID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Defect, 0, len(all))
	for _, d := range all {
		if d.ProductID != productID {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// AssignDefect assigns a user to a defect. Requires owner role.
func (s *BacklogService) AssignDefect(ctx context.Context, user *model.User, productID, defID, targetUserID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return s.defects.Assign(ctx, &model.Assignment{
		ID:        uuid.New().String(),
		TargetID:  defID,
		UserID:    targetUserID,
		CreatedAt: now,
	})
}

// UnassignDefect removes a user from a defect. Requires owner role.
func (s *BacklogService) UnassignDefect(ctx context.Context, user *model.User, productID, defID, targetUserID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	return s.defects.Unassign(ctx, defID, targetUserID)
}

// ListDefectAssignees returns assignees for a defect. Requires viewer role.
func (s *BacklogService) ListDefectAssignees(ctx context.Context, user *model.User, productID, defID string) ([]*model.Assignment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.defects.ListAssignees(ctx, defID)
}

// AddDefectLink adds a link from a defect to another entity. Requires tester role.
func (s *BacklogService) AddDefectLink(ctx context.Context, user *model.User, productID, defID, targetType, targetID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleTester); err != nil {
		return err
	}
	if !model.ValidDefectLinkTarget(targetType) {
		return fmt.Errorf("invalid defect link target type: %q", targetType)
	}
	if targetID == "" {
		return errors.New("target id is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return s.defects.AddLink(ctx, &model.DefectLink{
		ID:         uuid.New().String(),
		DefectID:   defID,
		TargetType: targetType,
		TargetID:   targetID,
		CreatedAt:  now,
	})
}

// RemoveDefectLink removes a defect link. Requires developer role.
func (s *BacklogService) RemoveDefectLink(ctx context.Context, user *model.User, productID, linkID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}
	return s.defects.RemoveLink(ctx, linkID)
}

// ListDefectLinks returns links for a defect. Requires viewer role.
func (s *BacklogService) ListDefectLinks(ctx context.Context, user *model.User, productID, defID string) ([]*model.DefectLink, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.defects.ListLinks(ctx, defID)
}

// ListLinkedDefects returns defects whose DefectLink rows reference the
// given target (e.g. a requirement or capability), along with a map from
// defect ID to the link row's ID (needed to unlink). Requires viewer role.
func (s *BacklogService) ListLinkedDefects(ctx context.Context, user *model.User, productID, targetType, targetID string) ([]*model.Defect, map[string]string, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, nil, err
	}
	links, err := s.defects.ListLinksByTarget(ctx, targetType, targetID)
	if err != nil {
		return nil, nil, err
	}
	defects := make([]*model.Defect, 0, len(links))
	linkIDs := make(map[string]string, len(links))
	for _, link := range links {
		d, err := s.defects.GetByID(ctx, link.DefectID)
		if err != nil {
			continue
		}
		if d.ProductID != productID {
			continue
		}
		defects = append(defects, d)
		linkIDs[d.ID] = link.ID
	}
	return defects, linkIDs, nil
}

// ListDefectLinkTargets returns the IDs of the entities of the given type linked
// to a defect, along with a map from target ID to the link row's ID (needed to
// unlink). Resolving the IDs to full records is left to the caller, which owns
// the services for the respective target types. Requires viewer role.
func (s *BacklogService) ListDefectLinkTargets(ctx context.Context, user *model.User, productID, defID, targetType string) ([]string, map[string]string, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, nil, err
	}
	links, err := s.defects.ListLinks(ctx, defID)
	if err != nil {
		return nil, nil, err
	}
	targetIDs := make([]string, 0, len(links))
	linkIDs := make(map[string]string, len(links))
	for _, link := range links {
		if link.TargetType != targetType {
			continue
		}
		targetIDs = append(targetIDs, link.TargetID)
		linkIDs[link.TargetID] = link.ID
	}
	return targetIDs, linkIDs, nil
}
