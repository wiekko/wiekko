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
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx           store.Transactor
	requirements store.RequirementStore
	defects      store.DefectStore
	categories   store.CategoryStore
	statuses     store.StatusStore
	// products tells kanban from scrum products, which activate items
	// differently (FR-318). nil is treated as scrum.
	products store.ProductStore
	comments ownerPurger
	retester campaignRetester
	audit    store.AuditStore
	authz    *Authorizer
	owns     *ownership
}

// --- Requirements ---

// CreateRequirementInput contains the fields required to create a requirement.
type CreateRequirementInput struct {
	ProductID        string
	Kind             string // "requirement" (default) or "task"
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
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt      string
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
	if err := s.owns.require(ctx, input.ProductID, ref{kindCategory, input.CategoryID}, ref{kindStatus, input.StatusID}, ref{kindMilestone, input.MilestoneID}, ref{kindSprint, input.SprintID}, ref{kindBacklogItem, input.ParentID}); err != nil {
		return nil, err
	}

	if input.Kind == "" {
		input.Kind = model.RequirementKindRequirement
	}
	if !model.ValidRequirementKind(input.Kind) {
		return nil, fmt.Errorf("invalid kind %q", input.Kind)
	}

	if input.Title == "" {
		return nil, errors.New(input.Kind + " title is required")
	}
	if err := s.resolveCategory(ctx, input.ProductID, input.Kind, &input.CategoryID); err != nil {
		return nil, err
	}
	if err := s.validateParent(ctx, input.ProductID, "", input.Kind, input.ParentID); err != nil {
		return nil, err
	}
	if err := s.resolveDefaultStatus(ctx, input.ProductID, model.RequirementKindRequirement, &input.StatusID); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	r := &model.Requirement{
		ID:               uuid.New().String(),
		ProductID:        input.ProductID,
		Kind:             input.Kind,
		ParentID:         input.ParentID,
		CategoryID:       input.CategoryID,
		Title:            input.Title,
		Description:      input.Description,
		DefinitionOfDone: input.DefinitionOfDone,
		StatusID:         input.StatusID,
		MilestoneID:      input.MilestoneID,
		SprintID:         input.SprintID,
		Estimation:       input.Estimation,
		Priority:         input.Priority,
		IsCoarse:         input.IsCoarse,
		IsActive:         input.SprintID != "", // planning into a sprint activates (FR-318)
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	// The sequence lock holds off concurrent creates between drawing the next
	// number and inserting it.
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockSequence("requirement", input.ProductID)); err != nil {
			return err
		}
		seq, err := s.requirements.NextSequenceNumber(ctx, input.ProductID)
		if err != nil {
			return fmt.Errorf("next sequence number: %w", err)
		}
		r.SequenceNumber = seq
		if err := s.requirements.Create(ctx, r); err != nil {
			return fmt.Errorf("create %s: %w", input.Kind, err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), ProductID: r.ProductID, EntityType: r.Kind, EntityID: r.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	})
	if err != nil {
		return nil, err
	}
	if s.kanbanMode(ctx, input.ProductID) {
		r.IsActive = s.columnActive(ctx, r.StatusID)
	}

	return r, nil
}

// GetRequirement returns a requirement by ID. Requires viewer role.
func (s *BacklogService) GetRequirement(ctx context.Context, user *model.User, productID, id string) (*model.Requirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, id}); err != nil {
		return nil, err
	}
	return s.requirements.GetByID(ctx, id)
}

// UpdateRequirement modifies a requirement. Developers can change the status of
// an activated item (FR-407); activating it and full field updates require the
// owner role.
func (s *BacklogService) UpdateRequirement(ctx context.Context, user *model.User, productID, id string, input UpdateRequirementInput) (*model.Requirement, error) {
	role, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper)
	if err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, id}); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCategory, opt(input.CategoryID)}, ref{kindStatus, opt(input.StatusID)}, ref{kindMilestone, opt(input.MilestoneID)}, ref{kindSprint, opt(input.SprintID)}, ref{kindBacklogItem, opt(input.ParentID)}); err != nil {
		return nil, err
	}

	var r *model.Requirement
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		r, err = s.requirements.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, r.UpdatedAt); err != nil {
			return err
		}

		isDeveloper := roleRank(role) < roleRank(model.RoleOwner)
		kanban := s.kanbanMode(ctx, productID)
		if err := checkActivationChange(r.Kind, isDeveloper, kanban, r.IsActive, input.IsActive); err != nil {
			return err
		}
		if isDeveloper {
			if err := requireActivatedForStatus(r.Kind, r.IsActive, r.StatusID, input.StatusID); err != nil {
				return err
			}
		}

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
				return fmt.Errorf("%w: requires owner role for full updates", ErrForbidden)
			}
			if input.ParentID != nil {
				if err := s.validateParent(ctx, productID, r.ID, r.Kind, *input.ParentID); err != nil {
					return err
				}
				r.ParentID = *input.ParentID
			}
			if input.CategoryID != nil {
				if err := s.checkCategoryKind(ctx, r.Kind, *input.CategoryID); err != nil {
					return err
				}
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
				r.IsActive = activationAfterSprintMove(r.SprintID, *input.SprintID, r.IsActive, input.IsActive != nil)
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
		if kanban {
			r.IsActive = s.columnActive(ctx, r.StatusID)
		}
		r.UpdatedAt = now

		if err := inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.requirements.Update(ctx, r); err != nil {
				return fmt.Errorf("update %s: %w", r.Kind, err)
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), ProductID: r.ProductID, EntityType: r.Kind, EntityID: r.ID,
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
	return r, nil
}

// DeleteRequirement removes a requirement. Requires owner role.
func (s *BacklogService) DeleteRequirement(ctx context.Context, user *model.User, productID, id string) error {
	return s.DeleteRequirementWith(ctx, user, productID, id, KeepChildren)
}

// ChildPolicy decides what happens to the children of a deleted requirement
// or task.
type ChildPolicy int

const (
	// KeepChildren moves the direct children up one level, under the deleted
	// item's own parent, or to the top level if it had none.
	KeepChildren ChildPolicy = iota
	// DeleteChildren deletes the whole subtree with the item.
	DeleteChildren
)

// ParseChildPolicy reads "keep" (or "") and "delete".
func ParseChildPolicy(v string) (ChildPolicy, error) {
	switch v {
	case "", "keep":
		return KeepChildren, nil
	case "delete":
		return DeleteChildren, nil
	}
	return KeepChildren, fmt.Errorf("children must be keep or delete, not %q", v)
}

// DeleteRequirementWith deletes a requirement or task, and its children as
// policy says, in one transaction. Requires owner role.
//
// Kept children are re-parented explicitly, each with an audit entry, rather
// than left to the foreign key's SET NULL, which would turn them into
// top-level items without a trace in their history.
func (s *BacklogService) DeleteRequirementWith(ctx context.Context, user *model.User, productID, id string, policy ChildPolicy) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, id}); err != nil {
		return err
	}

	var afterCommit []func()
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// Held across the walk, so no child is added or moved under the item
		// while its subtree is being resolved.
		if err := lock(ctx, s.tx, lockOwner(id)); err != nil {
			return err
		}
		r, err := s.requirements.GetByID(ctx, id)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)

		var doomed []*model.Requirement
		if policy == DeleteChildren {
			if doomed, err = s.subtree(ctx, r, true); err != nil {
				return err
			}
		} else {
			children, err := s.requirements.ListByParentID(ctx, id)
			if err != nil {
				return err
			}
			for _, child := range children {
				if err := s.requirements.SetParent(ctx, child.ID, r.ParentID, now); err != nil {
					return fmt.Errorf("move %s up: %w", child.Kind, err)
				}
				if err := s.audit.Create(ctx, &model.AuditEntry{
					ID: uuid.New().String(), ProductID: productID, EntityType: child.Kind, EntityID: child.ID,
					UserID: user.ID, Action: "update", FieldName: "parent_id",
					OldValue: id, NewValue: r.ParentID, CreatedAt: now,
				}); err != nil {
					return err
				}
			}
		}

		// Leaves first, the item itself last.
		for _, item := range append(doomed, r) {
			after, err := deleteOwnedInTx(ctx, s.comments, s.audit, user, productID, item.Kind, item.ID, s.requirements.Delete)
			if err != nil {
				return err
			}
			if after != nil {
				afterCommit = append(afterCommit, after)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, fn := range afterCommit {
		fn()
	}
	return nil
}

// subtree returns every descendant of r, deepest first, so each one is
// deleted before its parent. With lockItems, which needs a transaction, each
// descendant's lock is taken on the way, so none gains a child meanwhile.
func (s *BacklogService) subtree(ctx context.Context, r *model.Requirement, lockItems bool) ([]*model.Requirement, error) {
	var out []*model.Requirement
	var walk func(parent *model.Requirement, depth int) error
	walk = func(parent *model.Requirement, depth int) error {
		if depth > maxParentDepth {
			return errors.New("child items are nested too deeply")
		}
		children, err := s.requirements.ListByParentID(ctx, parent.ID)
		if err != nil {
			return err
		}
		for _, child := range children {
			if lockItems {
				if err := lock(ctx, s.tx, lockOwner(child.ID)); err != nil {
					return err
				}
			}
			if err := walk(child, depth+1); err != nil {
				return err
			}
			out = append(out, child)
		}
		return nil
	}
	return out, walk(r, 0)
}

// ListDescendants returns every item below a requirement or task, for the
// delete confirmation. Requires viewer role.
func (s *BacklogService) ListDescendants(ctx context.Context, user *model.User, productID, id string) ([]*model.Requirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, id}); err != nil {
		return nil, err
	}
	r, err := s.requirements.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.subtree(ctx, r, false)
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
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, parentID}); err != nil {
		return nil, err
	}
	return s.requirements.ListByParentID(ctx, parentID)
}

// resolveCategory fills in categoryID with the product's first category for
// kind when the caller left it empty, and otherwise checks the one supplied.
func (s *BacklogService) resolveCategory(ctx context.Context, productID, kind string, categoryID *string) error {
	if *categoryID != "" {
		return s.checkCategoryKind(ctx, kind, *categoryID)
	}
	cats, err := s.categories.ListByProductID(ctx, productID, kind)
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}
	if len(cats) == 0 {
		return fmt.Errorf("no %s categories defined for this product", kind)
	}
	*categoryID = cats[0].ID
	return nil
}

// resolveDefaultStatus fills an empty status with the product's default status
// for the entity type ("requirement" covers tasks too, "defect" defects). If the
// product has no default status, the item is left without one.
func (s *BacklogService) resolveDefaultStatus(ctx context.Context, productID, entityType string, statusID *string) error {
	if *statusID != "" {
		return nil
	}
	statuses, err := s.statuses.ListByProductID(ctx, productID, entityType)
	if err != nil {
		return fmt.Errorf("list statuses: %w", err)
	}
	for _, st := range statuses {
		if st.IsDefault {
			*statusID = st.ID
			return nil
		}
	}
	return nil
}

// checkCategoryKind rejects a category belonging to a different kind, so a
// task cannot be filed under FR and a requirement cannot be filed under MT.
func (s *BacklogService) checkCategoryKind(ctx context.Context, kind, categoryID string) error {
	cat, err := s.categories.GetByID(ctx, categoryID)
	if err != nil {
		return fmt.Errorf("category not found: %w", err)
	}
	if cat.EntityType != kind {
		return fmt.Errorf("category is not a %s category", kind)
	}
	return nil
}

// maxParentDepth caps the ancestor walk in validateParent. A backlog tree
// deeper than this is a data problem in its own right.
const maxParentDepth = 100

// validateParent checks a proposed parent link. childID is empty on create.
//
// A requirement's parent must be a requirement: requirements decompose into
// requirements, and both requirements and tasks may decompose into tasks, but
// a requirement never hangs off a task. Cross-product parents and cycles are
// rejected for both kinds — neither was checked before this.
func (s *BacklogService) validateParent(ctx context.Context, productID, childID, childKind, parentID string) error {
	if parentID == "" {
		return nil
	}
	if parentID == childID {
		return errors.New("a backlog item cannot be its own parent")
	}

	parent, err := s.requirements.GetByID(ctx, parentID)
	if err != nil {
		return fmt.Errorf("parent not found: %w", err)
	}
	if parent.ProductID != productID {
		return errors.New("parent belongs to a different product")
	}
	if childKind == model.RequirementKindRequirement && parent.Kind != model.RequirementKindRequirement {
		return errors.New("a requirement's parent must be a requirement")
	}

	// Walk up from the proposed parent; reaching the child means the link
	// would close a cycle.
	for i, cur := 0, parent; i < maxParentDepth; i++ {
		if cur.ParentID == "" {
			return nil
		}
		if cur.ParentID == childID {
			return errors.New("parent link would create a cycle")
		}
		cur, err = s.requirements.GetByID(ctx, cur.ParentID)
		if err != nil {
			// A dangling ancestor is pre-existing damage, not a reason to
			// block this edit.
			return nil
		}
	}
	return errors.New("parent chain is too deep")
}

// AssignRequirement assigns a user to a requirement. Requires owner role.
func (s *BacklogService) AssignRequirement(ctx context.Context, user *model.User, productID, reqID, targetUserID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, reqID}); err != nil {
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
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, reqID}); err != nil {
		return err
	}
	return s.requirements.Unassign(ctx, reqID, targetUserID)
}

// ListRequirementAssignees returns assignees for a requirement. Requires viewer role.
func (s *BacklogService) ListRequirementAssignees(ctx context.Context, user *model.User, productID, reqID string) ([]*model.Assignment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, reqID}); err != nil {
		return nil, err
	}
	return s.requirements.ListAssignees(ctx, reqID)
}

// --- Defects ---

// CreateDefectInput contains the fields required to create a defect.
type CreateDefectInput struct {
	ProductID     string
	Title         string
	Description   string
	Criticality   string
	StatusID      string
	MilestoneID   string
	SprintID      string
	EnvironmentID string
	Estimation    string
	Priority      int
}

// UpdateDefectInput contains the fields that can be updated on a defect.
type UpdateDefectInput struct {
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt   string
	Title         *string
	Description   *string
	Criticality   *string
	StatusID      *string
	MilestoneID   *string
	SprintID      *string
	EnvironmentID *string
	Estimation    *string
	Priority      *int
	IsActive      *bool
}

// CreateDefect creates a new defect. Requires tester role.
func (s *BacklogService) CreateDefect(ctx context.Context, user *model.User, input CreateDefectInput) (*model.Defect, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleTester); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, input.ProductID, ref{kindStatus, input.StatusID}, ref{kindMilestone, input.MilestoneID}, ref{kindSprint, input.SprintID}, ref{kindEnvironment, input.EnvironmentID}); err != nil {
		return nil, err
	}

	if input.Title == "" {
		return nil, errors.New("defect title is required")
	}
	if input.Criticality == "" {
		input.Criticality = model.DefectCriticalityMedium
	}
	if err := s.resolveDefaultStatus(ctx, input.ProductID, "defect", &input.StatusID); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	d := &model.Defect{
		ID:            uuid.New().String(),
		ProductID:     input.ProductID,
		Title:         input.Title,
		Description:   input.Description,
		Criticality:   input.Criticality,
		StatusID:      input.StatusID,
		MilestoneID:   input.MilestoneID,
		SprintID:      input.SprintID,
		EnvironmentID: input.EnvironmentID,
		Estimation:    input.Estimation,
		Priority:      input.Priority,
		IsActive:      input.SprintID != "", // planning into a sprint activates (FR-318)
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.insertNumbered(ctx, d); err != nil {
			return err
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), ProductID: d.ProductID, EntityType: "defect", EntityID: d.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}
	d.Identifier = model.EntityIdentifier(s.defectPrefix(ctx, input.ProductID), d.SequenceNumber)
	if s.kanbanMode(ctx, input.ProductID) {
		d.IsActive = s.columnActive(ctx, d.StatusID)
	}

	return d, nil
}

// GetDefect returns a defect by ID. Requires viewer role.
func (s *BacklogService) GetDefect(ctx context.Context, user *model.User, productID, id string) (*model.Defect, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDefect, id}); err != nil {
		return nil, err
	}
	return s.defects.GetByID(ctx, id)
}

// UpdateDefect modifies a defect. Developers can change the status of an
// activated defect (FR-405, FR-407); activating it and full field updates
// require the owner role.
func (s *BacklogService) UpdateDefect(ctx context.Context, user *model.User, productID, id string, input UpdateDefectInput) (*model.Defect, error) {
	role, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper)
	if err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDefect, id}); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindStatus, opt(input.StatusID)}, ref{kindMilestone, opt(input.MilestoneID)}, ref{kindSprint, opt(input.SprintID)}, ref{kindEnvironment, opt(input.EnvironmentID)}); err != nil {
		return nil, err
	}

	var d *model.Defect
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		d, err = s.defects.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, d.UpdatedAt); err != nil {
			return err
		}

		isDeveloper := roleRank(role) < roleRank(model.RoleOwner)
		kanban := s.kanbanMode(ctx, productID)
		if err := checkActivationChange("defect", isDeveloper, kanban, d.IsActive, input.IsActive); err != nil {
			return err
		}
		if isDeveloper {
			if err := requireActivatedForStatus("defect", d.IsActive, d.StatusID, input.StatusID); err != nil {
				return err
			}
		}

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
			input.MilestoneID != nil || input.SprintID != nil || input.EnvironmentID != nil ||
			input.Estimation != nil || input.Priority != nil {
			if isDeveloper {
				return fmt.Errorf("%w: requires owner role for full updates", ErrForbidden)
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
				d.IsActive = activationAfterSprintMove(d.SprintID, *input.SprintID, d.IsActive, input.IsActive != nil)
				d.SprintID = *input.SprintID
			}
			if input.EnvironmentID != nil {
				d.EnvironmentID = *input.EnvironmentID
			}
			if input.Estimation != nil {
				d.Estimation = *input.Estimation
			}
			if input.Priority != nil {
				d.Priority = *input.Priority
			}
		}
		if kanban {
			d.IsActive = s.columnActive(ctx, d.StatusID)
		}
		d.UpdatedAt = now

		// Closing a defect re-queues the results it failed (FR-504). Both happen
		// or neither: a saved close whose re-test was lost could not be repeated,
		// since the defect would no longer be "just closed".
		err = inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.defects.Update(ctx, d); err != nil {
				return fmt.Errorf("update defect: %w", err)
			}
			if s.defectJustClosed(ctx, previousStatusID, d.StatusID) && s.retester != nil {
				if err := s.retester.MarkResultsForRetest(ctx, d.ID, user.ID); err != nil {
					return err
				}
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), ProductID: d.ProductID, EntityType: "defect", EntityID: d.ID,
				UserID: user.ID, Action: "update", CreatedAt: now,
			})
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// activationAfterSprintMove returns an item's is_active after its sprint changes
// from oldSprintID to newSprintID (FR-318). Planning into a sprint, or moving
// to another one, activates the item. Taking it out of a sprint deactivates it:
// back in the backlog, it is the product owner's responsibility again. An
// unchanged sprint (forms re-send it on every save) keeps the current state, as
// does an explicit is_active in the same request.
func activationAfterSprintMove(oldSprintID, newSprintID string, isActive, explicit bool) bool {
	if explicit || oldSprintID == newSprintID {
		return isActive
	}
	return newSprintID != ""
}

// requireActivatedForStatus enforces FR-407 for developers: they may change the
// status of a backlog item only if it was activated upfront. isActive is the
// item's activation as read from the store: in kanban mode, whether its
// current column is active; in scrum mode, the stored flag, set when the item
// is planned into a sprint and cleared when it leaves one (FR-318). So a
// developer may move an item out of an active column to any column, but not
// out of an inactive one. "Upfront" means the state before this update, so
// activating and moving an item in the same request is refused. Re-sending the
// current status is not a change.
func requireActivatedForStatus(kind string, isActive bool, currentStatusID string, newStatusID *string) error {
	if newStatusID == nil || *newStatusID == currentStatusID {
		return nil
	}
	if isActive {
		return nil
	}
	return fmt.Errorf("%w: the %s has not been activated for work, so only a product owner can change its status", ErrForbidden, kind)
}

// checkActivationChange decides whether is_active may be set directly to
// requested (FR-318). Re-sending the current value is not a change. Only
// owners activate; in kanban mode not even they set the flag, since an item is
// active exactly while it sits in an active column.
func checkActivationChange(kind string, isDeveloper, kanban, current bool, requested *bool) error {
	if requested == nil || *requested == current {
		return nil
	}
	if isDeveloper {
		return fmt.Errorf("%w: only a product owner can activate or deactivate a %s", ErrForbidden, kind)
	}
	if kanban {
		return fmt.Errorf("in kanban mode a %s is active while it is in an active column: move it into one instead of setting is_active", kind)
	}
	return nil
}

// insertNumbered stores a new defect under the product's next defect number
// (FR-043). The sequence lock makes concurrent inserts — parallel CI jobs
// filing defects for the same product — draw their numbers one after another
// instead of colliding on the unique index. It joins the caller's
// transaction, if any.
func (s *BacklogService) insertNumbered(ctx context.Context, d *model.Defect) error {
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockSequence("defect", d.ProductID)); err != nil {
			return err
		}
		seq, err := s.defects.NextSequenceNumber(ctx, d.ProductID)
		if err != nil {
			return fmt.Errorf("next defect number: %w", err)
		}
		d.SequenceNumber = seq
		if err := s.defects.Create(ctx, d); err != nil {
			return fmt.Errorf("create defect: %w", err)
		}
		return nil
	})
}

// defectPrefix returns the product's defect identifier prefix.
func (s *BacklogService) defectPrefix(ctx context.Context, productID string) string {
	if s.products != nil {
		if p, err := s.products.GetByID(ctx, productID); err == nil && p.DefectPrefix != "" {
			return p.DefectPrefix
		}
	}
	return model.DefaultDefectPrefix
}

// kanbanMode reports whether the product activates items by column.
func (s *BacklogService) kanbanMode(ctx context.Context, productID string) bool {
	if s.products == nil {
		return false
	}
	p, err := s.products.GetByID(ctx, productID)
	return err == nil && p.WorkflowMode == model.WorkflowModeKanban
}

// columnActive reports whether the status is an active column; an item
// without a status, or with one that cannot be read, is not active.
func (s *BacklogService) columnActive(ctx context.Context, statusID string) bool {
	if statusID == "" || s.statuses == nil {
		return false
	}
	st, err := s.statuses.GetByID(ctx, statusID)
	return err == nil && st.IsActive
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
	if err := s.owns.require(ctx, productID, ref{kindDefect, id}); err != nil {
		return err
	}

	return deleteOwned(ctx, s.tx, s.comments, s.audit, user, productID, model.OwnerTypeDefect, id, s.defects.Delete)
}

// ListDefects returns defects for a product. Requires viewer role.
func (s *BacklogService) ListDefects(ctx context.Context, user *model.User, productID string, filter store.DefectFilter) ([]*model.Defect, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.defects.ListByProductID(ctx, productID, filter)
}

// ListDefectsPage returns one page of defects along with the number of defects
// the filter matches overall, which the caller needs to size its pagination.
// Requires viewer role.
//
// The count is taken from the same filter as the page rather than from
// len(items): items is capped by params.Limit and says nothing about how many
// rows lie beyond it.
func (s *BacklogService) ListDefectsPage(ctx context.Context, user *model.User, productID string, filter store.DefectFilter, params store.ListParams) ([]*model.Defect, int, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, 0, err
	}
	total, err := s.defects.CountByProductID(ctx, productID, filter, params.Search)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.defects.ListPageByProductID(ctx, productID, filter, params)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// CountDefects reports how many defects a product has in total, ignoring any
// filter. The list page uses it to tell "this product has no defects yet" apart
// from "no defect matches the current filter", which need different empty states.
// Requires viewer role.
func (s *BacklogService) CountDefects(ctx context.Context, user *model.User, productID string) (int, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return 0, err
	}
	return s.defects.CountByProductID(ctx, productID, store.DefectFilter{}, "")
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
	if err := s.owns.require(ctx, productID, ref{kindDefect, defID}); err != nil {
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
	if err := s.owns.require(ctx, productID, ref{kindDefect, defID}); err != nil {
		return err
	}
	return s.defects.Unassign(ctx, defID, targetUserID)
}

// ListDefectAssignees returns assignees for a defect. Requires viewer role.
func (s *BacklogService) ListDefectAssignees(ctx context.Context, user *model.User, productID, defID string) ([]*model.Assignment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDefect, defID}); err != nil {
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
	// The target's lock (taken by its delete as well, in PurgeOwner) keeps
	// the target from vanishing between the check and the insert: target_id
	// has no foreign key to stop a dangling link.
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockOwner(targetID)); err != nil {
			return err
		}
		if err := s.owns.require(ctx, productID, ref{kindDefect, defID}, ref{defectLinkKind(targetType), targetID}); err != nil {
			return err
		}
		return s.defects.AddLink(ctx, &model.DefectLink{
			ID:         uuid.New().String(),
			DefectID:   defID,
			TargetType: targetType,
			TargetID:   targetID,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		})
	})
}

// RemoveDefectLink removes a defect link. Requires developer role.
func (s *BacklogService) RemoveDefectLink(ctx context.Context, user *model.User, productID, linkID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindDefectLink, linkID}); err != nil {
		return err
	}
	return s.defects.RemoveLink(ctx, linkID)
}

// ListDefectLinks returns links for a defect. Requires viewer role.
func (s *BacklogService) ListDefectLinks(ctx context.Context, user *model.User, productID, defID string) ([]*model.DefectLink, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDefect, defID}); err != nil {
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
	if err := s.owns.require(ctx, productID, ref{kindDefect, defID}); err != nil {
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
