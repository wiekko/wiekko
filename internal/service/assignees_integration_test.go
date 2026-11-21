// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
)

// Assignments (UR-171): owners and admins assign any member, developers only
// themselves on items activated for work; only members can be assigned;
// changes leave history entries, repeats change nothing, and assignments end
// with the membership.
func TestAssignees(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		pid := e.product.ID
		assignees := func(id string) map[string]bool {
			t.Helper()
			as, err := e.svc.Backlog.ListRequirementAssignees(ctx, e.owner, pid, id)
			if err != nil {
				t.Fatal(err)
			}
			m := map[string]bool{}
			for _, a := range as {
				m[a.UserID] = true
			}
			return m
		}
		entries := func(action string) int {
			t.Helper()
			return countRows(t, conn, driver, "audit_trail", "action", action)
		}

		t.Run("developers assign themselves on activated items only", func(t *testing.T) {
			open, active := e.requirement(t), e.requirement(t)
			e.setStatus(t, active.ID, "In Progress") // an active column (kanban)
			if err := e.svc.Backlog.AssignRequirement(ctx, e.dev, pid, open.ID, e.dev.ID); !errors.Is(err, ErrForbidden) {
				t.Errorf("self-assigning an item not activated: %v", err)
			}
			if err := e.svc.Backlog.AssignRequirement(ctx, e.dev, pid, active.ID, e.dev.ID); err != nil {
				t.Fatalf("self-assigning an activated item: %v", err)
			}
			if !assignees(active.ID)[e.dev.ID] {
				t.Fatal("developer not assigned")
			}
			other := e.member(t, "dora", model.RoleDeveloper)
			if err := e.svc.Backlog.AssignRequirement(ctx, e.dev, pid, active.ID, other.ID); !errors.Is(err, ErrForbidden) {
				t.Errorf("a developer assigning someone else: %v", err)
			}
			if err := e.svc.Backlog.UnassignRequirement(ctx, e.dev, pid, active.ID, e.dev.ID); err != nil || assignees(active.ID)[e.dev.ID] {
				t.Errorf("self-unassigning: %v", err)
			}
		})

		t.Run("members only, history, harmless repeats", func(t *testing.T) {
			r := e.requirement(t)
			outsider := e.mkOutsider(t)
			if err := e.svc.Backlog.AssignRequirement(ctx, e.owner, pid, r.ID, outsider.ID); !errors.Is(err, ErrNotMember) {
				t.Errorf("assigning a non-member: %v", err)
			}
			before := entries(AuditActionAssign)
			for range 2 {
				if err := e.svc.Backlog.AssignRequirement(ctx, e.owner, pid, r.ID, e.dev.ID); err != nil {
					t.Fatal(err)
				}
			}
			if got := entries(AuditActionAssign) - before; got != 1 {
				t.Errorf("%d assign entries for one change, want 1", got)
			}
			before = entries(AuditActionUnassign)
			for range 2 {
				if err := e.svc.Backlog.UnassignRequirement(ctx, e.owner, pid, r.ID, e.dev.ID); err != nil {
					t.Fatal(err)
				}
			}
			if got := entries(AuditActionUnassign) - before; got != 1 {
				t.Errorf("%d unassign entries for one change, want 1", got)
			}
		})

		t.Run("removing a member ends their assignments", func(t *testing.T) {
			leaver := e.member(t, "lea", model.RoleDeveloper)
			r := e.requirement(t)
			d, err := e.svc.Backlog.CreateDefect(ctx, e.owner, CreateDefectInput{ProductID: pid, Title: "Crash"})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.svc.Backlog.AssignRequirement(ctx, e.owner, pid, r.ID, leaver.ID); err != nil {
				t.Fatal(err)
			}
			if err := e.svc.Backlog.AssignDefect(ctx, e.owner, pid, d.ID, leaver.ID); err != nil {
				t.Fatal(err)
			}
			before := entries(AuditActionUnassign)
			if err := e.svc.Product.RemoveMember(ctx, e.owner, pid, leaver.ID); err != nil {
				t.Fatal(err)
			}
			if assignees(r.ID)[leaver.ID] {
				t.Error("still assigned to the requirement")
			}
			das, _ := e.svc.Backlog.ListDefectAssignees(ctx, e.owner, pid, d.ID)
			if len(das) != 0 {
				t.Error("still assigned to the defect")
			}
			if got := entries(AuditActionUnassign) - before; got != 2 {
				t.Errorf("%d unassign entries, want 2", got)
			}
		})

		t.Run("defects follow the same rules", func(t *testing.T) {
			d, err := e.svc.Backlog.CreateDefect(ctx, e.owner, CreateDefectInput{ProductID: pid, Title: "Hang"})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.svc.Backlog.AssignDefect(ctx, e.dev, pid, d.ID, e.dev.ID); !errors.Is(err, ErrForbidden) {
				t.Errorf("self-assigning a defect not activated: %v", err)
			}
			if err := e.svc.Backlog.AssignDefect(ctx, e.owner, pid, d.ID, e.dev.ID); err != nil {
				t.Fatal(err)
			}
		})
	})
}

// mkOutsider creates a user who is no member of the product.
func (e *activationEnv) mkOutsider(t *testing.T) *model.User {
	t.Helper()
	u := &model.User{
		ID: uuid.NewString(), Username: "outsider" + e.product.ID[:8], Email: "outsider@example.com", DisplayName: "Outsider",
		PasswordHash: "x", Status: model.UserStatusActive, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}
	if err := e.st.User.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}
