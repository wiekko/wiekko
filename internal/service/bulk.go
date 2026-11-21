// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"wiekko.net/internal/model"
	"wiekko.net/internal/store"
)

// Bulk operations (FR-130, FR-131, FR-169, FR-170) change many requirements,
// tasks, defects, test cases or constraints at once. They hold to SC-92:
// every item goes through the same BacklogService, TestingService or
// ConstraintService call as the individual change, so the role checks, activation rules, kanban
// columns, archive cascade and history entries apply unchanged, and one
// item's refusal never blocks the others.

// BulkAction names a bulk operation.
type BulkAction string

// The bulk operations. Value carries the new status, category, criticality,
// priority ("0" for none), milestone or sprint ("" for none) or the user to
// assign or unassign.
const (
	BulkStatus      BulkAction = "status"
	BulkCategory    BulkAction = "category"    // requirements and tasks
	BulkCriticality BulkAction = "criticality" // defects
	BulkPriority    BulkAction = "priority"
	BulkMilestone   BulkAction = "milestone"
	BulkSprint      BulkAction = "sprint"
	BulkAssign      BulkAction = "assign"
	BulkUnassign    BulkAction = "unassign"
	BulkArchive     BulkAction = "archive"
	BulkUnarchive   BulkAction = "unarchive"
	BulkDelete      BulkAction = "delete"

	// Test cases only (FR-169). Value is the requirement or constraint to
	// link or unlink, or the capability, component or pyramid level ("" for
	// none).
	BulkLinkRequirement   BulkAction = "link_requirement"
	BulkUnlinkRequirement BulkAction = "unlink_requirement"
	BulkLinkConstraint    BulkAction = "link_constraint"
	BulkUnlinkConstraint  BulkAction = "unlink_constraint"
	BulkCapability        BulkAction = "capability"
	BulkComponent         BulkAction = "component"
	BulkPyramidLevel      BulkAction = "pyramid_level"
)

// BulkCompliance sets the compliance of constraints (FR-170); Value is the
// state. Constraints also take BulkCategory, BulkPriority and BulkDelete.
const BulkCompliance BulkAction = "compliance"

// BulkKindTestCase and BulkKindConstraint are the BulkRequest.Kind of test
// cases and constraints.
const (
	BulkKindTestCase   = "test_case"
	BulkKindConstraint = "constraint"
)

// BulkMaxItems caps one bulk operation. Selecting everything that matches a
// filter can reach far more items than anyone means to change in one go.
const BulkMaxItems = 500

// ErrBulkTooMany is returned for a request over BulkMaxItems.
var ErrBulkTooMany = fmt.Errorf("a bulk operation can change at most %d items at once; narrow the selection", BulkMaxItems)

// errBulkInvalid is returned for an action the item kind does not have, or a
// value it needs and lacks.
var errBulkInvalid = errors.New("invalid bulk operation")

// errDryRun rolls a preview's transaction back after the change succeeded.
var errDryRun = errors.New("dry run")

// BulkRequest describes one bulk operation.
type BulkRequest struct {
	ProductID string
	// Kind is model.RequirementKindRequirement, model.RequirementKindTask,
	// "defect" or BulkKindTestCase; items of another kind count as not found.
	Kind   string
	Action BulkAction
	Value  string
	// Children is what deleting a requirement or task does to the items
	// below it, as for a single delete.
	Children ChildPolicy
	IDs      []string
}

// BulkOutcome is what the operation did, or would do, to one item: Err is nil
// when the item is (or can be) changed, and says why not otherwise.
type BulkOutcome struct {
	ID  string
	Err error
}

// BulkService applies bulk operations to backlog items.
type BulkService struct {
	tx           store.Transactor
	backlog      *BacklogService
	testing      *TestingService
	constraints  *ConstraintService
	authz        *Authorizer
	productRoles store.ProductRoleStore
}

// Preview reports, item by item, whether the operation can be applied, by
// applying it in a transaction that is then rolled back: the outcome is that
// of the real call, without a second copy of its rules. Deleting is the
// exception. It unlinks attachment files once its own transaction ends, which
// a preview must never reach, so a delete is previewed by its role check and
// the items' existence alone; that is all a delete checks.
func (s *BulkService) Preview(ctx context.Context, user *model.User, req BulkRequest) ([]BulkOutcome, error) {
	if err := s.validate(ctx, req); err != nil {
		return nil, err
	}
	if s.tx == nil {
		return nil, errors.New("bulk preview needs a transactional store")
	}
	out := make([]BulkOutcome, 0, len(req.IDs))
	for _, id := range req.IDs {
		var err error
		if req.Action == BulkDelete {
			err = s.previewDelete(ctx, user, req, id)
		} else {
			err = s.tx.InTx(ctx, func(ctx context.Context) error {
				if err := s.one(ctx, user, req, id); err != nil {
					return err
				}
				return errDryRun
			})
			if errors.Is(err, errDryRun) {
				err = nil
			}
		}
		out = append(out, BulkOutcome{ID: id, Err: err})
	}
	return out, nil
}

// Apply applies the operation to each item in turn, each in a transaction of
// its own, and reports the outcome per item.
func (s *BulkService) Apply(ctx context.Context, user *model.User, req BulkRequest) ([]BulkOutcome, error) {
	if err := s.validate(ctx, req); err != nil {
		return nil, err
	}
	out := make([]BulkOutcome, 0, len(req.IDs))
	for _, id := range req.IDs {
		err := s.one(ctx, user, req, id)
		if req.Action == BulkDelete && errors.Is(err, ErrNotFound) && s.deletedAlong(ctx, user, req, out) {
			// Deleted along with an item above it that was deleted first.
			err = nil
		}
		out = append(out, BulkOutcome{ID: id, Err: err})
	}
	return out, nil
}

// deletedAlong reports whether an item that is gone may have been deleted
// with its parent earlier in the same request.
func (s *BulkService) deletedAlong(_ context.Context, _ *model.User, req BulkRequest, done []BulkOutcome) bool {
	if req.Kind != model.RequirementKindRequirement && req.Kind != model.RequirementKindTask || req.Children != DeleteChildren {
		return false
	}
	for _, o := range done {
		if o.Err == nil {
			return true
		}
	}
	return false
}

// validate checks the request as a whole: its size, and that the action
// exists for the kind and has the value it needs.
func (s *BulkService) validate(ctx context.Context, req BulkRequest) error {
	if len(req.IDs) > BulkMaxItems {
		return ErrBulkTooMany
	}
	if req.Kind == BulkKindTestCase {
		return validateTestCaseBulk(req)
	}
	if req.Kind == BulkKindConstraint {
		return validateConstraintBulk(req)
	}
	defect := req.Kind == "defect"
	if !defect && req.Kind != model.RequirementKindRequirement && req.Kind != model.RequirementKindTask {
		return fmt.Errorf("%w: unknown kind %q", errBulkInvalid, req.Kind)
	}
	switch req.Action {
	case BulkStatus, BulkAssign, BulkUnassign:
		if req.Value == "" {
			return fmt.Errorf("%w: %s needs a value", errBulkInvalid, req.Action)
		}
	case BulkCategory:
		if defect || req.Value == "" {
			return fmt.Errorf("%w: defects have no category", errBulkInvalid)
		}
	case BulkCriticality:
		if !defect || !model.ValidDefectCriticality(req.Value) {
			return fmt.Errorf("%w: criticality %q", errBulkInvalid, req.Value)
		}
	case BulkPriority:
		if _, err := priorityValue(req.Value); err != nil {
			return err
		}
	case BulkMilestone, BulkSprint, BulkArchive, BulkUnarchive, BulkDelete:
	default:
		return fmt.Errorf("%w: unknown action %q", errBulkInvalid, req.Action)
	}
	if req.Action == BulkAssign {
		// Only members can be assigned; the single assignment leaves that to
		// its callers, which offer members only.
		if _, err := s.productRoles.GetByUserAndProduct(ctx, req.Value, req.ProductID); err != nil {
			return fmt.Errorf("%w: the user is not a member of the product", errBulkInvalid)
		}
	}
	return nil
}

// validateTestCaseBulk checks the operations of test cases (FR-169).
func validateTestCaseBulk(req BulkRequest) error {
	switch req.Action {
	case BulkLinkRequirement, BulkUnlinkRequirement, BulkLinkConstraint, BulkUnlinkConstraint:
		if req.Value == "" {
			return fmt.Errorf("%w: %s needs a value", errBulkInvalid, req.Action)
		}
	case BulkCapability, BulkComponent, BulkPyramidLevel, BulkDelete:
	default:
		return fmt.Errorf("%w: test cases have no bulk %q", errBulkInvalid, req.Action)
	}
	return nil
}

// validateConstraintBulk checks the operations of constraints (FR-170).
func validateConstraintBulk(req BulkRequest) error {
	switch req.Action {
	case BulkCompliance:
		switch req.Value {
		case model.ComplianceNotAssessed, model.ComplianceMet, model.ComplianceViolated:
		default:
			return fmt.Errorf("%w: compliance %q", errBulkInvalid, req.Value)
		}
	case BulkCategory:
		if req.Value == "" {
			return fmt.Errorf("%w: category needs a value", errBulkInvalid)
		}
	case BulkPriority:
		if _, err := priorityValue(req.Value); err != nil {
			return err
		}
	case BulkDelete:
	default:
		return fmt.Errorf("%w: constraints have no bulk %q", errBulkInvalid, req.Action)
	}
	return nil
}

// priorityValue reads a priority: 0 for none, 1 the highest (FR-149).
func priorityValue(v string) (int, error) {
	p, err := strconv.Atoi(v)
	if err != nil || p < 0 {
		return 0, fmt.Errorf("%w: priority must be 0 (none) or a positive number, not %q", errBulkInvalid, v)
	}
	return p, nil
}

// IsBulkInvalid reports whether err rejects the request as a whole rather
// than one of its items.
func IsBulkInvalid(err error) bool {
	return errors.Is(err, errBulkInvalid) || errors.Is(err, ErrBulkTooMany)
}

// one applies the operation to one item through the individual operation.
func (s *BulkService) one(ctx context.Context, user *model.User, req BulkRequest, id string) error {
	if req.Kind == BulkKindTestCase {
		return s.oneTestCase(ctx, user, req, id)
	}
	if req.Kind == BulkKindConstraint {
		return s.oneConstraint(ctx, user, req, id)
	}
	if req.Kind == "defect" {
		return s.oneDefect(ctx, user, req, id)
	}
	r, err := s.backlog.GetRequirement(ctx, user, req.ProductID, id)
	if err != nil {
		return err
	}
	if r.Kind != req.Kind {
		return ErrNotFound
	}
	value := req.Value
	var in UpdateRequirementInput
	switch req.Action {
	case BulkStatus:
		in.StatusID = &value
	case BulkCategory:
		in.CategoryID = &value
	case BulkPriority:
		p, _ := priorityValue(value) // validated
		in.Priority = &p
	case BulkMilestone:
		in.MilestoneID = &value
	case BulkSprint:
		in.SprintID = &value
	case BulkArchive, BulkUnarchive:
		archived := req.Action == BulkArchive
		in.Archived = &archived
	case BulkAssign:
		return s.backlog.AssignRequirement(ctx, user, req.ProductID, id, req.Value)
	case BulkUnassign:
		return s.backlog.UnassignRequirement(ctx, user, req.ProductID, id, req.Value)
	case BulkDelete:
		return s.backlog.DeleteRequirementWith(ctx, user, req.ProductID, id, req.Children)
	}
	_, err = s.backlog.UpdateRequirement(ctx, user, req.ProductID, id, in)
	return err
}

func (s *BulkService) oneDefect(ctx context.Context, user *model.User, req BulkRequest, id string) error {
	if _, err := s.backlog.GetDefect(ctx, user, req.ProductID, id); err != nil {
		return err
	}
	value := req.Value
	var in UpdateDefectInput
	switch req.Action {
	case BulkStatus:
		in.StatusID = &value
	case BulkCriticality:
		in.Criticality = &value
	case BulkPriority:
		p, _ := priorityValue(value) // validated
		in.Priority = &p
	case BulkMilestone:
		in.MilestoneID = &value
	case BulkSprint:
		in.SprintID = &value
	case BulkArchive, BulkUnarchive:
		archived := req.Action == BulkArchive
		in.Archived = &archived
	case BulkAssign:
		return s.backlog.AssignDefect(ctx, user, req.ProductID, id, req.Value)
	case BulkUnassign:
		return s.backlog.UnassignDefect(ctx, user, req.ProductID, id, req.Value)
	case BulkDelete:
		return s.backlog.DeleteDefect(ctx, user, req.ProductID, id)
	}
	_, err := s.backlog.UpdateDefect(ctx, user, req.ProductID, id, in)
	return err
}

// oneTestCase applies the operation to one test case. Linking a linked pair
// and unlinking an unlinked one change nothing and succeed, as they do one at
// a time (FR-167).
func (s *BulkService) oneTestCase(ctx context.Context, user *model.User, req BulkRequest, id string) error {
	if _, err := s.testing.GetTestCase(ctx, user, req.ProductID, id); err != nil {
		return err
	}
	value := req.Value
	var in UpdateTestCaseInput
	switch req.Action {
	case BulkLinkRequirement:
		return s.testing.LinkRequirement(ctx, user, req.ProductID, id, req.Value, "")
	case BulkUnlinkRequirement:
		return s.testing.UnlinkRequirement(ctx, user, req.ProductID, id, req.Value)
	case BulkLinkConstraint:
		return s.testing.LinkConstraint(ctx, user, req.ProductID, id, req.Value)
	case BulkUnlinkConstraint:
		return s.testing.UnlinkConstraint(ctx, user, req.ProductID, id, req.Value)
	case BulkDelete:
		return s.testing.DeleteTestCase(ctx, user, req.ProductID, id)
	case BulkCapability:
		in.CapabilityID = &value
	case BulkComponent:
		in.ComponentID = &value
	case BulkPyramidLevel:
		in.PyramidLevelID = &value
	}
	_, err := s.testing.UpdateTestCase(ctx, user, req.ProductID, id, in)
	return err
}

// oneConstraint applies the operation to one constraint.
func (s *BulkService) oneConstraint(ctx context.Context, user *model.User, req BulkRequest, id string) error {
	if _, err := s.constraints.GetConstraint(ctx, user, req.ProductID, id); err != nil {
		return err
	}
	if req.Action == BulkDelete {
		return s.constraints.DeleteConstraint(ctx, user, req.ProductID, id)
	}
	value := req.Value
	var in UpdateConstraintInput
	switch req.Action {
	case BulkCompliance:
		in.ComplianceState = &value
	case BulkCategory:
		in.CategoryID = &value
	case BulkPriority:
		p, _ := priorityValue(value) // validated
		in.Priority = &p
	}
	_, err := s.constraints.UpdateConstraint(ctx, user, req.ProductID, id, in)
	return err
}

// previewDelete checks what deleting one item checks: the role (owner for
// backlog items, developer for test cases) and that the item exists and is of
// the kind.
func (s *BulkService) previewDelete(ctx context.Context, user *model.User, req BulkRequest, id string) error {
	if req.Kind == BulkKindTestCase {
		if _, err := s.authz.RequireRole(ctx, user, req.ProductID, model.RoleDeveloper); err != nil {
			return err
		}
		_, err := s.testing.GetTestCase(ctx, user, req.ProductID, id)
		return err
	}
	if req.Kind == BulkKindConstraint {
		if _, err := s.authz.RequireRole(ctx, user, req.ProductID, model.RoleOwner); err != nil {
			return err
		}
		_, err := s.constraints.GetConstraint(ctx, user, req.ProductID, id)
		return err
	}
	if _, err := s.authz.RequireRole(ctx, user, req.ProductID, model.RoleOwner); err != nil {
		return err
	}
	if req.Kind == "defect" {
		_, err := s.backlog.GetDefect(ctx, user, req.ProductID, id)
		return err
	}
	r, err := s.backlog.GetRequirement(ctx, user, req.ProductID, id)
	if err == nil && r.Kind != req.Kind {
		err = ErrNotFound
	}
	return err
}
