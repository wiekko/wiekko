// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
)

// Test case to constraint links (FR-150) on a real database.

func (e *activationEnv) constraint(t *testing.T, title string) *model.Constraint {
	t.Helper()
	ctx := context.Background()
	cats, err := e.svc.Category.ListCategories(ctx, e.owner, e.product.ID, "constraint")
	if err != nil || len(cats) == 0 {
		t.Fatalf("constraint categories: %v (%d)", err, len(cats))
	}
	c, err := e.svc.Constraint.CreateConstraint(ctx, e.owner, CreateConstraintInput{
		ProductID: e.product.ID, CategoryID: cats[0].ID, Title: title,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *activationEnv) testCase(t *testing.T, title, capabilityID string) *model.TestCase {
	t.Helper()
	tc, err := e.svc.Testing.CreateTestCase(context.Background(), e.owner, CreateTestCaseInput{
		ProductID: e.product.ID, Title: title, ExecutionMode: "manual", CapabilityID: capabilityID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tc
}

// member adds a user with role to the product.
func (e *activationEnv) member(t *testing.T, name, role string) *model.User {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	u := &model.User{
		ID: uuid.NewString(), Username: name, Email: name + "@example.com", DisplayName: name,
		PasswordHash: "x", Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := e.st.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Product.AddMember(ctx, e.owner, e.product.ID, u.ID, role); err != nil {
		t.Fatal(err)
	}
	return u
}

func evidenceTitles(evs []TestCaseEvidence) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ev.TestCase.Title)
	}
	return out
}

func TestTestCaseConstraintLinks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		pid := e.product.ID

		cookies := e.constraint(t, "Session cookies are HttpOnly")
		csrf := e.constraint(t, "CSRF on every page route")
		tc := e.testCase(t, "Cookie flags", "")

		t.Run("developers link, both sides list", func(t *testing.T) {
			if err := e.svc.Testing.LinkConstraint(ctx, e.dev, pid, tc.ID, cookies.ID); err != nil {
				t.Fatal(err)
			}
			// Linking a linked pair again changes nothing.
			if err := e.svc.Testing.LinkConstraint(ctx, e.dev, pid, tc.ID, cookies.ID); err != nil {
				t.Fatalf("linking again: %v", err)
			}
			if err := e.svc.Testing.LinkConstraint(ctx, e.owner, pid, tc.ID, csrf.ID); err != nil {
				t.Fatal(err)
			}
			cons, err := e.svc.Testing.ListTestCaseConstraints(ctx, e.dev, pid, tc.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(cons) != 2 || cons[0].ID != cookies.ID || cons[1].ID != csrf.ID {
				t.Fatalf("test case lists %d constraints, want %s then %s", len(cons), cookies.Title, csrf.Title)
			}
			tcs, err := e.svc.Testing.ListConstraintTestCases(ctx, e.dev, pid, cookies.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(tcs.Linked) != 1 || tcs.Linked[0].TestCase.ID != tc.ID || tcs.Linked[0].Latest != nil {
				t.Fatalf("constraint lists %v, want only %q without a result", evidenceTitles(tcs.Linked), tc.Title)
			}
		})

		t.Run("unlink", func(t *testing.T) {
			if err := e.svc.Testing.UnlinkConstraint(ctx, e.dev, pid, tc.ID, csrf.ID); err != nil {
				t.Fatal(err)
			}
			tcs, err := e.svc.Testing.ListConstraintTestCases(ctx, e.dev, pid, csrf.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(tcs.Linked) != 0 {
				t.Errorf("unlinked constraint still lists %v", evidenceTitles(tcs.Linked))
			}
		})

		t.Run("viewers read, cannot link", func(t *testing.T) {
			viewer := e.member(t, "vera", model.RoleViewer)
			if _, err := e.svc.Testing.ListConstraintTestCases(ctx, viewer, pid, cookies.ID); err != nil {
				t.Fatalf("viewer reading: %v", err)
			}
			for name, err := range map[string]error{
				"link":   e.svc.Testing.LinkConstraint(ctx, viewer, pid, tc.ID, csrf.ID),
				"unlink": e.svc.Testing.UnlinkConstraint(ctx, viewer, pid, tc.ID, cookies.ID),
			} {
				if !errors.Is(err, ErrForbidden) {
					t.Errorf("viewer %s: err = %v, want forbidden", name, err)
				}
			}
		})

		t.Run("items of another product", func(t *testing.T) {
			other, err := e.svc.Product.Create(ctx, e.owner, CreateProductInput{Name: "Other", WorkflowMode: model.WorkflowModeKanban})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.svc.Testing.LinkConstraint(ctx, e.owner, other.ID, tc.ID, cookies.ID); !errors.Is(err, ErrNotFound) {
				t.Errorf("linking through another product: err = %v, want not found", err)
			}
		})

		t.Run("latest result", func(t *testing.T) {
			camp, err := e.svc.Testing.CreateCampaign(ctx, e.owner, CreateCampaignInput{ProductID: pid, Name: "R1", TestCaseIDs: []string{tc.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.svc.Testing.ActivateCampaign(ctx, e.owner, pid, camp.ID); err != nil {
				t.Fatal(err)
			}
			results, err := e.svc.Testing.ListCampaignResults(ctx, e.owner, pid, camp.ID)
			if err != nil || len(results) != 1 {
				t.Fatalf("campaign results: %v (%d)", err, len(results))
			}
			if _, err := e.svc.Testing.SetResultStatus(ctx, e.owner, pid, camp.ID, results[0].ID, model.CampaignResultStatusPassed, ""); err != nil {
				t.Fatal(err)
			}
			tcs, err := e.svc.Testing.ListConstraintTestCases(ctx, e.dev, pid, cookies.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := tcs.Linked[0].Latest; got == nil || got.Status != model.CampaignResultStatusPassed {
				t.Fatalf("latest result = %+v, want passed", got)
			}
		})

		t.Run("derived through capabilities", func(t *testing.T) {
			auth, err := e.svc.Component.CreateCapability(ctx, e.owner, CreateCapabilityInput{ProductID: pid, Name: "Authentication"})
			if err != nil {
				t.Fatal(err)
			}
			sessions, err := e.svc.Component.CreateCapability(ctx, e.owner, CreateCapabilityInput{ProductID: pid, Name: "Sessions"})
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range []*model.Capability{auth, sessions} {
				if err := e.svc.Constraint.LinkCapability(ctx, e.owner, pid, cookies.ID, c.ID); err != nil {
					t.Fatal(err)
				}
			}
			login := e.testCase(t, "Login works", auth.ID)
			e.testCase(t, "Session expiry", sessions.ID)
			// A linked test case of a capability stays in the linked list only.
			if _, err := e.svc.Testing.UpdateTestCase(ctx, e.owner, pid, tc.ID, UpdateTestCaseInput{CapabilityID: &auth.ID}); err != nil {
				t.Fatal(err)
			}

			tcs, err := e.svc.Testing.ListConstraintTestCases(ctx, e.dev, pid, cookies.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := evidenceTitles(tcs.Linked); len(got) != 1 || got[0] != tc.Title {
				t.Errorf("linked = %v, want [%s]", got, tc.Title)
			}
			var titles []string
			for _, d := range tcs.Derived {
				titles = append(titles, d.TestCase.Title)
			}
			if len(titles) != 2 || titles[0] != "Login works" || titles[1] != "Session expiry" {
				t.Fatalf("derived = %v, want [Login works Session expiry]", titles)
			}
			if via := tcs.Derived[0].Via; len(via) != 1 || via[0].ID != auth.ID {
				t.Errorf("%s comes via %v, want %s", login.Title, via, auth.Name)
			}
		})

		t.Run("deleting a side removes the link", func(t *testing.T) {
			gone := e.testCase(t, "Temporary", "")
			if err := e.svc.Testing.LinkConstraint(ctx, e.owner, pid, gone.ID, csrf.ID); err != nil {
				t.Fatal(err)
			}
			if err := e.svc.Testing.DeleteTestCase(ctx, e.owner, pid, gone.ID); err != nil {
				t.Fatal(err)
			}
			if n := countRows(t, conn, driver, "test_case_constraints", "test_case_id", gone.ID); n != 0 {
				t.Errorf("%d links left after deleting the test case", n)
			}
			if err := e.svc.Constraint.DeleteConstraint(ctx, e.owner, pid, cookies.ID); err != nil {
				t.Fatal(err)
			}
			if n := countRows(t, conn, driver, "test_case_constraints", "constraint_id", cookies.ID); n != 0 {
				t.Errorf("%d links left after deleting the constraint", n)
			}
		})
	})
}

func TestLatestResult(t *testing.T) {
	at := func(d time.Duration) string {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(d).Format(time.RFC3339)
	}
	results := []*model.CampaignResult{
		{ID: "old", Status: model.CampaignResultStatusFailed, ExecutedAt: at(0)},
		{ID: "new", Status: model.CampaignResultStatusPassed, ExecutedAt: at(time.Hour)},
		{ID: "pending", Status: model.CampaignResultStatusPending},
		{ID: "reset", Status: model.CampaignResultStatusPending, ExecutedAt: at(2 * time.Hour)},
	}
	if got := LatestResult(results); got == nil || got.ID != "new" {
		t.Errorf("LatestResult = %+v, want the newest executed one", got)
	}
	if got := LatestResult(results[2:3]); got != nil {
		t.Errorf("LatestResult of pending results = %+v, want nil", got)
	}
}
