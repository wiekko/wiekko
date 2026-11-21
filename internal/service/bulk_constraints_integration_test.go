package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
)

// Bulk operations on constraints (FR-170): the owners' and admins' to apply,
// as single constraint changes are.
func TestBulkConstraints(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		bulk := e.svc.Bulk
		a, b := e.constraint(t, "Runs offline"), e.constraint(t, "Encrypts at rest")
		req := func(action BulkAction, value string, ids ...string) BulkRequest {
			return BulkRequest{ProductID: e.product.ID, Kind: BulkKindConstraint, Action: action, Value: value, IDs: ids}
		}
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
		reload := func(id string) *model.Constraint {
			t.Helper()
			c, err := e.svc.Constraint.GetConstraint(ctx, e.owner, e.product.ID, id)
			if err != nil {
				t.Fatal(err)
			}
			return c
		}

		t.Run("compliance, category and priority", func(t *testing.T) {
			got := outcomes(bulk.Apply(ctx, e.owner, req(BulkCompliance, model.ComplianceViolated, a.ID, b.ID)))
			if got[a.ID] != nil || got[b.ID] != nil {
				t.Fatal(got)
			}
			if reload(a.ID).ComplianceState != model.ComplianceViolated || reload(b.ID).ComplianceState != model.ComplianceViolated {
				t.Error("compliance not set")
			}
			cats, _ := e.svc.Category.ListCategories(ctx, e.owner, e.product.ID, "constraint")
			last := cats[len(cats)-1]
			if got = outcomes(bulk.Apply(ctx, e.owner, req(BulkCategory, last.ID, a.ID))); got[a.ID] != nil || reload(a.ID).CategoryID != last.ID {
				t.Errorf("category: %v", got)
			}
			if got = outcomes(bulk.Apply(ctx, e.owner, req(BulkPriority, "3", b.ID))); got[b.ID] != nil || reload(b.ID).Priority != 3 {
				t.Errorf("priority: %v", got)
			}
		})

		t.Run("developers are refused", func(t *testing.T) {
			got := outcomes(bulk.Preview(ctx, e.dev, req(BulkCompliance, model.ComplianceMet, a.ID)))
			if !errors.Is(got[a.ID], ErrForbidden) {
				t.Errorf("developer compliance: %v", got)
			}
			got = outcomes(bulk.Preview(ctx, e.dev, req(BulkDelete, "", a.ID)))
			if !errors.Is(got[a.ID], ErrForbidden) {
				t.Errorf("developer delete: %v", got)
			}
		})

		t.Run("delete previews without deleting", func(t *testing.T) {
			c := e.constraint(t, "Goes away")
			if got := outcomes(bulk.Preview(ctx, e.owner, req(BulkDelete, "", c.ID))); got[c.ID] != nil {
				t.Fatal(got)
			}
			reload(c.ID)
			if got := outcomes(bulk.Apply(ctx, e.owner, req(BulkDelete, "", c.ID))); got[c.ID] != nil {
				t.Fatal(got)
			}
			if _, err := e.svc.Constraint.GetConstraint(ctx, e.owner, e.product.ID, c.ID); !errors.Is(err, ErrNotFound) {
				t.Errorf("deleted constraint: %v", err)
			}
		})

		t.Run("invalid requests", func(t *testing.T) {
			for _, r := range []BulkRequest{
				req(BulkCompliance, "maybe", a.ID),
				req(BulkPriority, "-2", a.ID),
				req(BulkStatus, "x", a.ID),
				req(BulkCategory, "", a.ID),
			} {
				if _, err := bulk.Preview(ctx, e.owner, r); !IsBulkInvalid(err) {
					t.Errorf("%s %q: %v", r.Action, r.Value, err)
				}
			}
			r := e.requirement(t)
			got := outcomes(bulk.Preview(ctx, e.owner, req(BulkPriority, "1", r.ID)))
			if !errors.Is(got[r.ID], ErrNotFound) {
				t.Errorf("a requirement in a constraint request: %v", got)
			}
		})
	})
}
