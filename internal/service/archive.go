// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"wiekko.net/internal/model"
	"wiekko.net/internal/store"
)

// Archiving (FR-155) takes closed backlog items out of the default views
// without deleting them: they stay linked to test cases, campaigns and the
// rest of the product state.
//
// Two invariants hold at all times:
//
//   - An archived item is in a closed status. Archiving requires one, and
//     every way an item can become open again (a status change, a status that
//     stops being closed, a deleted status) unarchives it.
//   - The descendants of an archived requirement are archived. Archiving
//     takes the whole subtree along, and an item that is unarchived, or that
//     is created or moved below an archived requirement, unarchives its
//     archived ancestors.
//
// Only product owners archive and unarchive explicitly; an implicit unarchive
// follows whatever change caused it.

// Audit actions of archiving.
const (
	auditActionArchive   = "archive"
	auditActionUnarchive = "unarchive"
)

// errArchiveOpen is returned when an item that is not in a closed status is to
// be archived.
var errArchiveOpen = errors.New("only items in a closed status can be archived")

// archiver keeps the archived state of requirements, tasks and defects. It is
// shared by the backlog service and the services that change statuses.
type archiver struct {
	tx           store.Transactor
	requirements store.RequirementStore
	defects      store.DefectStore
	statuses     store.StatusStore
	categories   store.CategoryStore
	audit        store.AuditStore
}

// identify returns an item's identifier, e.g. FR-12, for messages.
func (a *archiver) identify(ctx context.Context, r *model.Requirement) string {
	prefix := "?"
	if a.categories != nil {
		if c, err := a.categories.GetByID(ctx, r.CategoryID); err == nil {
			prefix = c.Prefix
		}
	}
	return model.EntityIdentifier(prefix, r.SequenceNumber)
}

// closed reports whether statusID names a closed status. No status, or one
// that cannot be read, is open.
func (a *archiver) closed(ctx context.Context, statusID string) bool {
	if statusID == "" || a.statuses == nil {
		return false
	}
	st, err := a.statuses.GetByID(ctx, statusID)
	return err == nil && st.IsClosed
}

// setRequirement archives (archivedAt set) or unarchives (archivedAt empty) one
// requirement or task with an audit entry; an item already in that state is
// left alone.
func (a *archiver) setRequirement(ctx context.Context, user *model.User, r *model.Requirement, archivedAt, now string) error {
	if r.Archived() == (archivedAt != "") {
		return nil
	}
	if err := a.requirements.SetArchivedAt(ctx, r.ID, archivedAt, now); err != nil {
		return fmt.Errorf("archive %s: %w", r.Kind, err)
	}
	r.ArchivedAt = archivedAt
	r.UpdatedAt = now
	return a.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), ProductID: r.ProductID, EntityType: r.Kind, EntityID: r.ID,
		UserID: user.ID, Action: archiveAction(archivedAt), CreatedAt: now,
	})
}

// setDefect is setRequirement for a defect.
func (a *archiver) setDefect(ctx context.Context, user *model.User, d *model.Defect, archivedAt, now string) error {
	if d.Archived() == (archivedAt != "") {
		return nil
	}
	if err := a.defects.SetArchivedAt(ctx, d.ID, archivedAt, now); err != nil {
		return fmt.Errorf("archive defect: %w", err)
	}
	d.ArchivedAt = archivedAt
	d.UpdatedAt = now
	return a.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), ProductID: d.ProductID, EntityType: "defect", EntityID: d.ID,
		UserID: user.ID, Action: archiveAction(archivedAt), CreatedAt: now,
	})
}

func archiveAction(archivedAt string) string {
	if archivedAt == "" {
		return auditActionUnarchive
	}
	return auditActionArchive
}

// descendants returns every item below r, taking each one's lock on the way so
// none gains a child meanwhile (see BacklogService.subtree).
func (a *archiver) descendants(ctx context.Context, r *model.Requirement) ([]*model.Requirement, error) {
	var out []*model.Requirement
	var walk func(parentID string, depth int) error
	walk = func(parentID string, depth int) error {
		if depth > maxParentDepth {
			return errors.New("child items are nested too deeply")
		}
		children, err := a.requirements.ListByParentID(ctx, parentID)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := lock(ctx, a.tx, lockOwner(child.ID)); err != nil {
				return err
			}
			out = append(out, child)
			if err := walk(child.ID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return out, walk(r.ID, 0)
}

// archiveRequirement archives r together with its subtree. r and every
// descendant must be in a closed status; the error names the first open one.
func (a *archiver) archiveRequirement(ctx context.Context, user *model.User, r *model.Requirement, now string) error {
	if !a.closed(ctx, r.StatusID) {
		return errArchiveOpen
	}
	items, err := a.descendants(ctx, r)
	if err != nil {
		return err
	}
	for _, d := range items {
		if !a.closed(ctx, d.StatusID) {
			return fmt.Errorf("%s %s is not closed yet; an item can only be archived together with all items below it, once they are all closed", d.Kind, a.identify(ctx, d))
		}
	}
	for _, item := range append(items, r) {
		if err := a.setRequirement(ctx, user, item, now, now); err != nil {
			return err
		}
	}
	return nil
}

// unarchiveRequirement unarchives r, its subtree and its archived ancestors.
func (a *archiver) unarchiveRequirement(ctx context.Context, user *model.User, r *model.Requirement, now string) error {
	items, err := a.descendants(ctx, r)
	if err != nil {
		return err
	}
	for _, item := range append(items, r) {
		if err := a.setRequirement(ctx, user, item, "", now); err != nil {
			return err
		}
	}
	return a.unarchiveAncestors(ctx, user, r.ParentID, now)
}

// unarchiveAncestors unarchives parentID and the archived requirements above
// it, so an item that is not archived never sits below an archived one. The
// walk stops at the first ancestor that is not archived: by the invariant,
// none above it is.
func (a *archiver) unarchiveAncestors(ctx context.Context, user *model.User, parentID, now string) error {
	for i := 0; parentID != "" && i < maxParentDepth; i++ {
		if err := lock(ctx, a.tx, lockEntity(parentID)); err != nil {
			return err
		}
		parent, err := a.requirements.GetByID(ctx, parentID)
		if err != nil {
			// A dangling ancestor is pre-existing damage, not a reason to
			// block this change.
			return nil
		}
		if !parent.Archived() {
			return nil
		}
		if err := a.setRequirement(ctx, user, parent, "", now); err != nil {
			return err
		}
		parentID = parent.ParentID
	}
	return nil
}

// reopened unarchives what a status no longer being closed brings back: every
// archived item in status st, with its subtree and ancestors. It runs when a
// status stops being closed or is deleted (its items lose their status).
func (a *archiver) reopened(ctx context.Context, user *model.User, st *model.Status, now string) error {
	if a == nil || !st.IsClosed {
		return nil
	}
	if st.EntityType == "defect" {
		defects, err := a.defects.ListByProductID(ctx, st.ProductID, store.DefectFilter{StatusID: st.ID, Archived: store.ArchivedOnly})
		if err != nil {
			return err
		}
		for _, d := range defects {
			if err := a.setDefect(ctx, user, d, "", now); err != nil {
				return err
			}
		}
		return nil
	}
	reqs, err := a.requirements.ListByProductID(ctx, st.ProductID, store.RequirementFilter{StatusID: st.ID, Archived: store.ArchivedOnly})
	if err != nil {
		return err
	}
	for _, r := range reqs {
		// An earlier item's subtree or ancestors may have covered this one.
		fresh, err := a.requirements.GetByID(ctx, r.ID)
		if err != nil {
			return err
		}
		if !fresh.Archived() {
			continue
		}
		if err := a.unarchiveRequirement(ctx, user, fresh, now); err != nil {
			return err
		}
	}
	return nil
}

// checkArchiveChange decides whether archived may be set to requested.
// Re-sending the current state is not a change; changing it is for owners.
func checkArchiveChange(isDeveloper, current bool, requested *bool) error {
	if requested == nil || *requested == current || !isDeveloper {
		return nil
	}
	return fmt.Errorf("%w: only a product owner can archive or unarchive an item", ErrForbidden)
}

// applyRequirementArchive brings r's archived state in line after an update
// that has been stored: an explicit archive or unarchive, an implicit
// unarchive when r was reopened, and unarchiving the ancestors of an item
// moved below an archived one.
func (s *BacklogService) applyRequirementArchive(ctx context.Context, user *model.User, r *model.Requirement, requested *bool, parentChanged bool, now string) error {
	a := s.archiver()
	switch {
	case requested != nil && *requested && !r.Archived():
		return a.archiveRequirement(ctx, user, r, now)
	case requested != nil && !*requested && r.Archived():
		return a.unarchiveRequirement(ctx, user, r, now)
	case r.Archived() && !a.closed(ctx, r.StatusID):
		return a.unarchiveRequirement(ctx, user, r, now)
	case !r.Archived() && parentChanged:
		return a.unarchiveAncestors(ctx, user, r.ParentID, now)
	}
	return nil
}

// applyDefectArchive is applyRequirementArchive for a defect, which has no
// hierarchy.
func (s *BacklogService) applyDefectArchive(ctx context.Context, user *model.User, d *model.Defect, requested *bool, now string) error {
	a := s.archiver()
	switch {
	case requested != nil && *requested && !d.Archived():
		if !a.closed(ctx, d.StatusID) {
			return errArchiveOpen
		}
		return a.setDefect(ctx, user, d, now, now)
	case requested != nil && !*requested && d.Archived(),
		d.Archived() && !a.closed(ctx, d.StatusID):
		return a.setDefect(ctx, user, d, "", now)
	}
	return nil
}

// archiver returns the backlog service's view of the archived state.
func (s *BacklogService) archiver() *archiver {
	return &archiver{
		tx: s.tx, requirements: s.requirements, defects: s.defects,
		statuses: s.statuses, categories: s.categories, audit: s.audit,
	}
}
