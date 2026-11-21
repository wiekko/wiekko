// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
	"wiekko.net/internal/store"
)

// Archiving (FR-155) on a real database: the archived state lives in SQL and
// list filters read it there.

func archiveFlag(v bool) *bool { return &v }

func (e *activationEnv) child(t *testing.T, parentID, kind string) *model.Requirement {
	t.Helper()
	r, err := e.svc.Backlog.CreateRequirement(context.Background(), e.owner, CreateRequirementInput{
		ProductID: e.product.ID, Title: "Child", ParentID: parentID, Kind: kind,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *activationEnv) setStatus(t *testing.T, id, name string) {
	t.Helper()
	if _, err := e.svc.Backlog.UpdateRequirement(context.Background(), e.owner, e.product.ID, id,
		UpdateRequirementInput{StatusID: e.status("requirement", name)}); err != nil {
		t.Fatalf("move to %s: %v", name, err)
	}
}

func (e *activationEnv) archive(u *model.User, id string, archived bool) error {
	_, err := e.svc.Backlog.UpdateRequirement(context.Background(), u, e.product.ID, id,
		UpdateRequirementInput{Archived: archiveFlag(archived)})
	return err
}

func wantArchived(t *testing.T, e *activationEnv, want bool, items ...*model.Requirement) {
	t.Helper()
	for _, it := range items {
		if got := e.reload(t, it.ID).Archived(); got != want {
			t.Errorf("%s archived = %v, want %v", it.Title, got, want)
		}
	}
}

func TestArchiveRequirements(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()

		t.Run("only closed items", func(t *testing.T) {
			r := e.requirement(t)
			if err := e.archive(e.owner, r.ID, true); !errors.Is(err, errArchiveOpen) {
				t.Fatalf("archiving an open item: err = %v, want %v", err, errArchiveOpen)
			}
			wantArchived(t, e, false, r)
		})

		t.Run("owners only", func(t *testing.T) {
			r := e.requirement(t)
			e.setStatus(t, r.ID, "Done")
			if err := e.archive(e.dev, r.ID, true); !errors.Is(err, ErrForbidden) {
				t.Fatalf("developer archiving: err = %v, want forbidden", err)
			}
			if err := e.archive(e.owner, r.ID, true); err != nil {
				t.Fatal(err)
			}
			if err := e.archive(e.dev, r.ID, false); !errors.Is(err, ErrForbidden) {
				t.Fatalf("developer unarchiving: err = %v, want forbidden", err)
			}
			wantArchived(t, e, true, r)
			if n := countRows(t, conn, driver, "audit_trail", "action", auditActionArchive); n == 0 {
				t.Error("archiving left no audit entry")
			}
		})

		t.Run("subtree must be closed and goes along", func(t *testing.T) {
			parent := e.requirement(t)
			child := e.child(t, parent.ID, model.RequirementKindRequirement)
			task := e.child(t, child.ID, model.RequirementKindTask)
			e.setStatus(t, parent.ID, "Done")
			e.setStatus(t, child.ID, "Done")

			if err := e.archive(e.owner, parent.ID, true); err == nil {
				t.Fatal("archived a requirement with an open task below it")
			}
			wantArchived(t, e, false, parent, child, task)

			e.setStatus(t, task.ID, "Done")
			if err := e.archive(e.owner, parent.ID, true); err != nil {
				t.Fatal(err)
			}
			wantArchived(t, e, true, parent, child, task)

			// Unarchiving brings the subtree back.
			if err := e.archive(e.owner, parent.ID, false); err != nil {
				t.Fatal(err)
			}
			wantArchived(t, e, false, parent, child, task)
		})

		t.Run("reopening unarchives the item, its subtree and its ancestors", func(t *testing.T) {
			parent := e.requirement(t)
			child := e.child(t, parent.ID, model.RequirementKindRequirement)
			task := e.child(t, child.ID, model.RequirementKindTask)
			for _, it := range []*model.Requirement{parent, child, task} {
				e.setStatus(t, it.ID, "Done")
			}
			if err := e.archive(e.owner, parent.ID, true); err != nil {
				t.Fatal(err)
			}
			e.setStatus(t, child.ID, "In Progress")
			wantArchived(t, e, false, parent, child, task)
		})

		t.Run("a new or moved item unarchives its ancestors", func(t *testing.T) {
			parent := e.requirement(t)
			e.setStatus(t, parent.ID, "Done")
			if err := e.archive(e.owner, parent.ID, true); err != nil {
				t.Fatal(err)
			}
			e.child(t, parent.ID, model.RequirementKindTask)
			wantArchived(t, e, false, parent)

			if err := e.archive(e.owner, parent.ID, true); err == nil {
				t.Fatal("archived a requirement whose new task is open")
			}
			other := e.requirement(t)
			e.setStatus(t, other.ID, "Done")
			if err := e.archive(e.owner, other.ID, true); err != nil {
				t.Fatal(err)
			}
			mover := e.requirement(t)
			if _, err := e.svc.Backlog.UpdateRequirement(ctx, e.owner, e.product.ID, mover.ID,
				UpdateRequirementInput{ParentID: &other.ID}); err != nil {
				t.Fatal(err)
			}
			wantArchived(t, e, false, other)
		})

		t.Run("list filters", func(t *testing.T) {
			r := e.requirement(t)
			e.setStatus(t, r.ID, "Done")
			if err := e.archive(e.owner, r.ID, true); err != nil {
				t.Fatal(err)
			}
			has := func(f store.ArchivedFilter) bool {
				list, err := e.svc.Backlog.ListRequirements(ctx, e.owner, e.product.ID, store.RequirementFilter{Archived: f})
				if err != nil {
					t.Fatal(err)
				}
				for _, it := range list {
					if it.ID == r.ID {
						return true
					}
					if f == store.ArchivedOnly && !it.Archived() {
						t.Errorf("only-archived list holds %s", it.Title)
					}
					if f == store.ArchivedExclude && it.Archived() {
						t.Errorf("list without archived items holds %s", it.Title)
					}
				}
				return false
			}
			if !has(store.ArchivedInclude) || has(store.ArchivedExclude) || !has(store.ArchivedOnly) {
				t.Errorf("archived item listed wrongly: include %v, exclude %v, only %v",
					has(store.ArchivedInclude), has(store.ArchivedExclude), has(store.ArchivedOnly))
			}
		})
	})
}

func TestArchiveDefects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		d, err := e.svc.Backlog.CreateDefect(ctx, e.owner, CreateDefectInput{ProductID: e.product.ID, Title: "Crash"})
		if err != nil {
			t.Fatal(err)
		}
		update := func(in UpdateDefectInput) (*model.Defect, error) {
			return e.svc.Backlog.UpdateDefect(ctx, e.owner, e.product.ID, d.ID, in)
		}

		if _, err := update(UpdateDefectInput{Archived: archiveFlag(true)}); !errors.Is(err, errArchiveOpen) {
			t.Fatalf("archiving an open defect: err = %v", err)
		}
		if _, err := update(UpdateDefectInput{StatusID: e.status("defect", "Done")}); err != nil {
			t.Fatal(err)
		}
		got, err := update(UpdateDefectInput{Archived: archiveFlag(true)})
		if err != nil || !got.Archived() {
			t.Fatalf("archive defect: %v, archived %v", err, got != nil && got.Archived())
		}
		list, err := e.svc.Backlog.ListDefects(ctx, e.owner, e.product.ID, store.DefectFilter{Archived: store.ArchivedExclude})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Errorf("archived defect listed without archived ones: %d", len(list))
		}
		got, err = update(UpdateDefectInput{StatusID: e.status("defect", "New")})
		if err != nil || got.Archived() {
			t.Fatalf("reopening kept the defect archived: %v", err)
		}
	})
}

// A status that stops being closed, or is deleted, reopens its items; they
// must not stay archived.
func TestArchiveFollowsStatusChanges(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		archivedIn := func(status string) *model.Requirement {
			r := e.requirement(t)
			e.setStatus(t, r.ID, status)
			if err := e.archive(e.owner, r.ID, true); err != nil {
				t.Fatal(err)
			}
			return r
		}

		// A second closed status to edit and delete, mirrored to defects.
		closed := true
		shipped, err := e.svc.Milestone.CreateStatus(ctx, e.owner, CreateStatusInput{
			ProductID: e.product.ID, EntityType: "requirement", Name: "Shipped", Position: 9, IsClosed: closed,
		})
		if err != nil {
			t.Fatal(err)
		}
		e.statuses["requirement"]["Shipped"] = shipped

		r := archivedIn("Shipped")
		open := false
		if _, err := e.svc.Milestone.UpdateStatus(ctx, e.owner, e.product.ID, shipped.ID, UpdateStatusInput{IsClosed: &open}); err != nil {
			t.Fatal(err)
		}
		wantArchived(t, e, false, r)

		if _, err := e.svc.Milestone.UpdateStatus(ctx, e.owner, e.product.ID, shipped.ID, UpdateStatusInput{IsClosed: &closed}); err != nil {
			t.Fatal(err)
		}
		r = archivedIn("Shipped")
		if err := e.svc.Milestone.DeleteStatus(ctx, e.owner, e.product.ID, shipped.ID); err != nil {
			t.Fatal(err)
		}
		wantArchived(t, e, false, r)

		// Items in other closed statuses are left alone.
		kept := archivedIn("Done")
		wantArchived(t, e, true, kept)
	})
}
