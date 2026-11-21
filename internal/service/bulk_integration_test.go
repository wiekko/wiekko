package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
)

// Bulk operations (FR-130, FR-131) apply the individual operation to each
// item (SC-92): the same refusals, one item never blocking another, and a
// preview that reports the outcome without changing anything.

func TestBulkOperations(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		bulk := e.svc.Bulk
		// Errorf, not Fatal: the subtests below call it with the parent's t.
		outcomes := func(outs []BulkOutcome, err error) map[string]error {
			if err != nil {
				t.Errorf("bulk request refused: %v", err)
			}
			m := make(map[string]error, len(outs))
			for _, o := range outs {
				m[o.ID] = o.Err
			}
			return m
		}
		req := func(action BulkAction, value string, ids ...string) BulkRequest {
			return BulkRequest{ProductID: e.product.ID, Kind: model.RequirementKindRequirement, Action: action, Value: value, IDs: ids}
		}

		t.Run("developers change the status of activated items only", func(t *testing.T) {
			open, active := e.requirement(t), e.requirement(t)
			e.setStatus(t, active.ID, "In Progress") // an active column (kanban)
			r := req(BulkStatus, *e.status("requirement", "Done"), open.ID, active.ID)

			got := outcomes(bulk.Preview(ctx, e.dev, r))
			if !errors.Is(got[open.ID], ErrForbidden) || got[active.ID] != nil {
				t.Fatalf("preview: %v", got)
			}
			if e.reload(t, active.ID).StatusID == *e.status("requirement", "Done") {
				t.Fatal("the preview changed the item")
			}

			got = outcomes(bulk.Apply(ctx, e.dev, r))
			if !errors.Is(got[open.ID], ErrForbidden) || got[active.ID] != nil {
				t.Fatalf("apply: %v", got)
			}
			if e.reload(t, active.ID).StatusID != *e.status("requirement", "Done") {
				t.Error("the permitted item was not changed")
			}
			if e.reload(t, open.ID).StatusID == *e.status("requirement", "Done") {
				t.Error("the refused item was changed")
			}
		})

		t.Run("archiving takes closed items and refuses open ones", func(t *testing.T) {
			open, done := e.requirement(t), e.requirement(t)
			e.setStatus(t, done.ID, "Done")
			got := outcomes(bulk.Apply(ctx, e.owner, req(BulkArchive, "", open.ID, done.ID)))
			if !errors.Is(got[open.ID], errArchiveOpen) || got[done.ID] != nil {
				t.Fatalf("archive: %v", got)
			}
			wantArchived(t, e, true, done)
			wantArchived(t, e, false, open)
		})

		t.Run("delete previews without deleting", func(t *testing.T) {
			a, b := e.requirement(t), e.requirement(t)
			r := req(BulkDelete, "", a.ID, b.ID)
			got := outcomes(bulk.Preview(ctx, e.dev, r))
			if !errors.Is(got[a.ID], ErrForbidden) {
				t.Errorf("developer delete preview: %v", got)
			}
			got = outcomes(bulk.Preview(ctx, e.owner, r))
			if got[a.ID] != nil || got[b.ID] != nil {
				t.Fatalf("owner delete preview: %v", got)
			}
			e.reload(t, a.ID) // still there

			parent := e.requirement(t)
			child := e.child(t, parent.ID, model.RequirementKindRequirement)
			r = req(BulkDelete, "", parent.ID, child.ID)
			r.Children = DeleteChildren
			got = outcomes(bulk.Apply(ctx, e.owner, r))
			if got[parent.ID] != nil || got[child.ID] != nil {
				t.Fatalf("deleting a parent with its child selected too: %v", got)
			}
			if _, err := e.svc.Backlog.GetRequirement(ctx, e.owner, e.product.ID, child.ID); !errors.Is(err, ErrNotFound) {
				t.Errorf("child still there: %v", err)
			}
		})

		t.Run("assigning takes members only and repeats harmlessly", func(t *testing.T) {
			a := e.requirement(t)
			if _, err := bulk.Apply(ctx, e.owner, req(BulkAssign, "nobody", a.ID)); !IsBulkInvalid(err) {
				t.Errorf("assigning a non-member: %v", err)
			}
			for range 2 {
				if got := outcomes(bulk.Apply(ctx, e.owner, req(BulkAssign, e.dev.ID, a.ID))); got[a.ID] != nil {
					t.Fatalf("assign: %v", got)
				}
			}
			as, _ := e.svc.Backlog.ListRequirementAssignees(ctx, e.owner, e.product.ID, a.ID)
			if len(as) != 1 {
				t.Fatalf("%d assignees, want 1", len(as))
			}
			if got := outcomes(bulk.Apply(ctx, e.owner, req(BulkUnassign, e.dev.ID, a.ID))); got[a.ID] != nil {
				t.Fatalf("unassign: %v", got)
			}
			if as, _ = e.svc.Backlog.ListRequirementAssignees(ctx, e.owner, e.product.ID, a.ID); len(as) != 0 {
				t.Errorf("%d assignees after unassigning", len(as))
			}
		})

		t.Run("priority is the owners' to set", func(t *testing.T) {
			a, b := e.requirement(t), e.requirement(t)
			e.setStatus(t, a.ID, "In Progress") // activated, yet still not the developer's field
			got := outcomes(bulk.Apply(ctx, e.dev, req(BulkPriority, "2", a.ID)))
			if !errors.Is(got[a.ID], ErrForbidden) {
				t.Errorf("developer setting priority: %v", got)
			}
			got = outcomes(bulk.Apply(ctx, e.owner, req(BulkPriority, "2", a.ID, b.ID)))
			if got[a.ID] != nil || got[b.ID] != nil {
				t.Fatalf("owner setting priority: %v", got)
			}
			if p := e.reload(t, b.ID).Priority; p != 2 {
				t.Errorf("priority %d, want 2", p)
			}
			if got = outcomes(bulk.Apply(ctx, e.owner, req(BulkPriority, "0", b.ID))); got[b.ID] != nil || e.reload(t, b.ID).Priority != 0 {
				t.Errorf("removing the priority: %v", got)
			}
			for _, bad := range []string{"", "-1", "high"} {
				if _, err := bulk.Preview(ctx, e.owner, req(BulkPriority, bad, a.ID)); !IsBulkInvalid(err) {
					t.Errorf("priority %q: %v", bad, err)
				}
			}
		})

		t.Run("items of another kind are not found", func(t *testing.T) {
			parent := e.requirement(t)
			task := e.child(t, parent.ID, model.RequirementKindTask)
			got := outcomes(bulk.Preview(ctx, e.owner, req(BulkMilestone, "", task.ID)))
			if !errors.Is(got[task.ID], ErrNotFound) {
				t.Errorf("task in a requirements request: %v", got)
			}
		})

		t.Run("invalid requests", func(t *testing.T) {
			many := make([]string, BulkMaxItems+1)
			if _, err := bulk.Preview(ctx, e.owner, req(BulkMilestone, "", many...)); !errors.Is(err, ErrBulkTooMany) {
				t.Errorf("too many: %v", err)
			}
			d := BulkRequest{ProductID: e.product.ID, Kind: "defect", Action: BulkCriticality, Value: "apocalyptic"}
			if _, err := bulk.Preview(ctx, e.owner, d); !IsBulkInvalid(err) {
				t.Errorf("unknown criticality: %v", err)
			}
			if _, err := bulk.Preview(ctx, e.owner, req(BulkCriticality, model.DefectCriticalityHigh)); !IsBulkInvalid(err) {
				t.Errorf("criticality of a requirement: %v", err)
			}
		})

		t.Run("defects", func(t *testing.T) {
			d, err := e.svc.Backlog.CreateDefect(ctx, e.owner, CreateDefectInput{ProductID: e.product.ID, Title: "Crash", Criticality: model.DefectCriticalityLow})
			if err != nil {
				t.Fatal(err)
			}
			r := BulkRequest{ProductID: e.product.ID, Kind: "defect", Action: BulkCriticality, Value: model.DefectCriticalityCritical, IDs: []string{d.ID}}
			if got := outcomes(bulk.Apply(ctx, e.owner, r)); got[d.ID] != nil {
				t.Fatal(got)
			}
			if got, _ := e.svc.Backlog.GetDefect(ctx, e.owner, e.product.ID, d.ID); got.Criticality != model.DefectCriticalityCritical {
				t.Errorf("criticality %q", got.Criticality)
			}
		})
	})
}
