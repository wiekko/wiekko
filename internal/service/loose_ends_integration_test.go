// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
	"wiekko.net/internal/store"
)

// These tests cover what deleting a test case, an environment, a backlog item
// with children or a product leaves behind: campaigns keep their history,
// children are moved or deleted as asked, and the audit trail stays reachable
// for administrators until they purge it.

// closedCampaign runs a campaign over tcID through to closed, with every
// result passed, and returns it.
func (e *activationEnv) closedCampaign(t *testing.T, name string, tcID string) *model.TestingCampaign {
	t.Helper()
	ctx := context.Background()
	c, err := e.svc.Testing.CreateCampaign(ctx, e.owner, CreateCampaignInput{
		ProductID: e.product.ID, Name: name, TestCaseIDs: []string{tcID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Testing.ActivateCampaign(ctx, e.owner, e.product.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	results, err := e.svc.Testing.ListCampaignResults(ctx, e.owner, e.product.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if _, err := e.svc.Testing.SetResultStatus(ctx, e.owner, e.product.ID, c.ID, r.ID, model.CampaignResultStatusPassed, ""); err != nil {
			t.Fatal(err)
		}
	}
	if c, err = e.svc.Testing.CloseCampaign(ctx, e.owner, e.product.ID, c.ID, false); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDeletedTestCaseStaysInClosedCampaigns(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		env, err := e.svc.Milestone.CreateRuntimeEnvironment(ctx, e.owner, e.product.ID, "linux")
		if err != nil {
			t.Fatal(err)
		}
		tc, err := e.svc.Testing.CreateTestCase(ctx, e.owner, CreateTestCaseInput{
			ProductID: e.product.ID, Title: "Login works", Description: "Steps", ExecutionMode: model.ExecutionModeManual,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Testing.AddTestCaseEnvironment(ctx, e.owner, e.product.ID, tc.ID, env.ID); err != nil {
			t.Fatal(err)
		}
		closed := e.closedCampaign(t, "1.0", tc.ID)
		draft, err := e.svc.Testing.CreateCampaign(ctx, e.owner, CreateCampaignInput{
			ProductID: e.product.ID, Name: "2.0", TestCaseIDs: []string{tc.ID},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Edits reach the draft, not the closed campaign.
		renamed := "Login still works"
		if _, err := e.svc.Testing.UpdateTestCase(ctx, e.owner, e.product.ID, tc.ID, UpdateTestCaseInput{Title: &renamed}); err != nil {
			t.Fatal(err)
		}
		titleIn := func(c *model.TestingCampaign) string {
			tests, err := e.svc.Testing.ListCampaignTests(ctx, e.owner, e.product.ID, c.ID)
			if err != nil || len(tests) != 1 {
				t.Fatalf("tests of %s = %v, %v", c.Name, tests, err)
			}
			return tests[0].Title
		}
		if got := titleIn(draft); got != renamed {
			t.Fatalf("draft title = %q, want %q", got, renamed)
		}
		if got := titleIn(closed); got != "Login works" {
			t.Fatalf("closed title = %q, want the copy taken when it ran", got)
		}

		events := countRows(t, conn, driver, "campaign_result_events", "campaign_id", closed.ID)
		if err := e.svc.Testing.DeleteTestCase(ctx, e.owner, e.product.ID, tc.ID); err != nil {
			t.Fatalf("DeleteTestCase: %v", err)
		}
		if err := e.svc.Milestone.DeleteRuntimeEnvironment(ctx, e.owner, e.product.ID, env.ID); err != nil {
			t.Fatalf("DeleteRuntimeEnvironment: %v", err)
		}

		kept, err := e.svc.Testing.ListCampaignResults(ctx, e.owner, e.product.ID, closed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(kept) != 1 || kept[0].TestTitle != "Login works" || kept[0].TestCaseID != "" ||
			kept[0].EnvironmentName != "linux" || kept[0].EnvironmentID != "" || kept[0].Status != model.CampaignResultStatusPassed {
			t.Fatalf("closed campaign results = %+v, want the passed linux result, detached", kept)
		}
		if n := countRows(t, conn, driver, "campaign_result_events", "campaign_id", closed.ID); n != events || n == 0 {
			t.Fatalf("closed campaign events = %d, want %d kept", n, events)
		}
		if gone, _ := e.svc.Testing.ListCampaignResults(ctx, e.owner, e.product.ID, draft.ID); len(gone) != 0 {
			t.Fatalf("draft campaign still has %d results", len(gone))
		}
	})
}

func TestDeleteRequirementWithChildren(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		create := func(kind, parent, title string) *model.Requirement {
			t.Helper()
			r, err := e.svc.Backlog.CreateRequirement(ctx, e.owner, CreateRequirementInput{
				ProductID: e.product.ID, Kind: kind, ParentID: parent, Title: title,
			})
			if err != nil {
				t.Fatal(err)
			}
			return r
		}

		t.Run("keep moves children up a level", func(t *testing.T) {
			top := create(model.RequirementKindRequirement, "", "Top")
			mid := create(model.RequirementKindRequirement, top.ID, "Middle")
			task := create(model.RequirementKindTask, mid.ID, "Task")
			sub := create(model.RequirementKindTask, task.ID, "Subtask")

			// The confirmation lists everything below the item, outside any
			// transaction.
			below, err := e.svc.Backlog.ListDescendants(ctx, e.owner, e.product.ID, mid.ID)
			if err != nil || len(below) != 2 || below[0].ID != sub.ID || below[1].ID != task.ID {
				t.Fatalf("descendants = %v, %v; want subtask then task", below, err)
			}

			if err := e.svc.Backlog.DeleteRequirementWith(ctx, e.owner, e.product.ID, mid.ID, KeepChildren); err != nil {
				t.Fatal(err)
			}
			if got := e.reload(t, task.ID).ParentID; got != top.ID {
				t.Fatalf("task parent = %q, want the deleted item's parent %q", got, top.ID)
			}
			if got := e.reload(t, sub.ID).ParentID; got != task.ID {
				t.Fatalf("grandchild parent = %q, want it untouched", got)
			}
			history, err := e.svc.Audit.ListByEntity(ctx, e.owner, e.product.ID, model.RequirementKindTask, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			moved := false
			for _, entry := range history {
				moved = moved || (entry.FieldName == "parent_id" && entry.OldValue == mid.ID && entry.NewValue == top.ID)
			}
			if !moved {
				t.Fatalf("history %+v has no entry for the move", history)
			}
		})

		t.Run("delete removes the subtree", func(t *testing.T) {
			top := create(model.RequirementKindRequirement, "", "Doomed")
			task := create(model.RequirementKindTask, top.ID, "Doomed task")
			sub := create(model.RequirementKindTask, task.ID, "Doomed subtask")
			if _, err := e.svc.Discussion.CreateComment(ctx, e.owner, e.product.ID, model.OwnerTypeTask, sub.ID, "bye"); err != nil {
				t.Fatal(err)
			}
			if err := e.svc.Backlog.DeleteRequirementWith(ctx, e.owner, e.product.ID, top.ID, DeleteChildren); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{top.ID, task.ID, sub.ID} {
				if n := countRows(t, conn, driver, "requirements", "id", id); n != 0 {
					t.Fatalf("item %s still exists", id)
				}
			}
			if n := countRows(t, conn, driver, "comments", "owner_id", sub.ID); n != 0 {
				t.Fatalf("subtask comments = %d, want 0", n)
			}
		})

		t.Run("a failure rolls the whole delete back", func(t *testing.T) {
			top := create(model.RequirementKindRequirement, "", "Survivor")
			task := create(model.RequirementKindTask, top.ID, "Surviving task")
			e.svc.Backlog.audit = failingAudit{AuditStore: e.svc.Backlog.audit, action: "delete"}
			defer func() { e.svc.Backlog.audit = e.svc.Backlog.audit.(failingAudit).AuditStore }()

			if err := e.svc.Backlog.DeleteRequirementWith(ctx, e.owner, e.product.ID, top.ID, DeleteChildren); err == nil {
				t.Fatal("delete succeeded although its audit entry failed")
			}
			for _, id := range []string{top.ID, task.ID} {
				if n := countRows(t, conn, driver, "requirements", "id", id); n != 1 {
					t.Fatalf("item %s gone after a failed delete", id)
				}
			}
		})
	})
}

func TestAdminAuditTrail(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		r := e.requirement(t)

		entries, total, err := e.svc.Audit.AdminList(ctx, e.owner, AuditFilterInput{ProductID: e.product.ID}, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total == 0 || len(entries) != total {
			t.Fatalf("entries = %d of %d", len(entries), total)
		}
		for _, entry := range entries {
			if entry.ProductID != e.product.ID {
				t.Fatalf("entry %+v carries product %q", entry, entry.ProductID)
			}
		}
		if _, _, err := e.svc.Audit.AdminList(ctx, e.dev, AuditFilterInput{}, 10, 0); !errors.Is(err, ErrForbidden) {
			t.Fatalf("non-admin list: err = %v, want ErrForbidden", err)
		}
		if _, err := e.svc.Audit.AdminPurgeProduct(ctx, e.owner, e.product.ID); !errors.Is(err, ErrProductExists) {
			t.Fatalf("purge a live product: err = %v, want ErrProductExists", err)
		}

		if err := e.svc.Product.Delete(ctx, e.owner, e.product.ID); err != nil {
			t.Fatal(err)
		}
		deleted, err := e.svc.Audit.AdminDeletedProducts(ctx, e.owner)
		if err != nil {
			t.Fatal(err)
		}
		if len(deleted) != 1 || deleted[0].ID != e.product.ID || deleted[0].Name != "Board" || deleted[0].Entries != total+1 {
			t.Fatalf("deleted products = %+v, want Board with %d entries", deleted, total+1)
		}
		gone, err := e.svc.Audit.AdminDeletedEntities(ctx, e.owner, []string{r.ID, e.product.ID})
		if err != nil || !gone[e.product.ID] || gone[r.ID] {
			t.Fatalf("deleted entities = %v, %v", gone, err)
		}

		var exported []string
		if err := e.svc.Audit.AdminExport(ctx, e.owner, AuditFilterInput{ProductID: e.product.ID, To: "2999-01-01"},
			func(entry *model.AuditEntry) error {
				exported = append(exported, entry.Action)
				return nil
			}); err != nil {
			t.Fatal(err)
		}
		// Entries within the same second have no defined order, so only the
		// set is checked.
		if len(exported) != total+1 || !strings.Contains(strings.Join(exported, " "), "delete") {
			t.Fatalf("exported %v, want %d entries including the delete", exported, total+1)
		}

		n, err := e.svc.Audit.AdminPurgeProduct(ctx, e.owner, e.product.ID)
		if err != nil {
			t.Fatal(err)
		}
		if int(n) != total+1 {
			t.Fatalf("purged %d, want %d", n, total+1)
		}
		if left, _, _ := e.svc.Audit.AdminList(ctx, e.owner, AuditFilterInput{ProductID: e.product.ID}, 10, 0); len(left) != 0 {
			t.Fatalf("%d entries left after the purge", len(left))
		}
		record, _, err := e.svc.Audit.AdminList(ctx, e.owner, AuditFilterInput{Action: "purge_audit"}, 10, 0)
		if err != nil || len(record) != 1 || record[0].EntityID != e.product.ID || record[0].OldValue != "Board" {
			t.Fatalf("purge record = %+v, %v", record, err)
		}
		if _, _, err := e.svc.Audit.AdminList(ctx, e.owner, AuditFilterInput{From: "yesterday"}, 10, 0); err == nil ||
			!strings.Contains(err.Error(), "invalid date") {
			t.Fatalf("bad date: err = %v", err)
		}
	})
}

// Compile-time check that the failing audit wrapper still satisfies the store.
var _ store.AuditStore = failingAudit{}
