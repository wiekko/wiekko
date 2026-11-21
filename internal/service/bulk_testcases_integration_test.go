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
)

// Bulk operations on test cases (FR-169): developers may apply them, as they
// change single test cases (FR-111); linking what is linked and unlinking
// what is not change nothing and are no errors.
func TestBulkTestCases(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		bulk := e.svc.Bulk
		tester := e.member(t, "tess", model.RoleTester)
		a, b := e.testCase(t, "Parses sightings", ""), e.testCase(t, "Syncs offline", "")
		req := func(action BulkAction, value string, ids ...string) BulkRequest {
			return BulkRequest{ProductID: e.product.ID, Kind: BulkKindTestCase, Action: action, Value: value, IDs: ids}
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
		reqLinks := func(id string) int {
			t.Helper()
			links, err := e.svc.Testing.ListTestCaseRequirements(ctx, e.owner, e.product.ID, id)
			if err != nil {
				t.Fatal(err)
			}
			return len(links)
		}

		t.Run("linking requirements, twice, and unlinking", func(t *testing.T) {
			r := e.requirement(t)
			if got := outcomes(bulk.Apply(ctx, e.dev, req(BulkLinkRequirement, r.ID, a.ID))); got[a.ID] != nil {
				t.Fatal(got)
			}
			got := outcomes(bulk.Apply(ctx, e.dev, req(BulkLinkRequirement, r.ID, a.ID, b.ID)))
			if got[a.ID] != nil || got[b.ID] != nil {
				t.Fatalf("linking with one already linked: %v", got)
			}
			if reqLinks(a.ID) != 1 || reqLinks(b.ID) != 1 {
				t.Errorf("links: %d, %d; want 1 each", reqLinks(a.ID), reqLinks(b.ID))
			}
			got = outcomes(bulk.Apply(ctx, e.dev, req(BulkUnlinkRequirement, r.ID, a.ID, b.ID)))
			if got[a.ID] != nil || got[b.ID] != nil || reqLinks(a.ID) != 0 {
				t.Fatalf("unlink: %v", got)
			}
			if got = outcomes(bulk.Apply(ctx, e.dev, req(BulkUnlinkRequirement, r.ID, a.ID))); got[a.ID] != nil {
				t.Errorf("unlinking what is not linked: %v", got)
			}
		})

		t.Run("constraints", func(t *testing.T) {
			c := e.constraint(t, "Runs offline")
			if got := outcomes(bulk.Apply(ctx, e.dev, req(BulkLinkConstraint, c.ID, a.ID, b.ID))); got[a.ID] != nil || got[b.ID] != nil {
				t.Fatal(got)
			}
			cons, _ := e.svc.Testing.ListTestCaseConstraints(ctx, e.owner, e.product.ID, b.ID)
			if len(cons) != 1 {
				t.Errorf("%d constraints linked, want 1", len(cons))
			}
		})

		t.Run("setting and clearing the component", func(t *testing.T) {
			comp, err := e.svc.Component.CreateComponent(ctx, e.owner, CreateComponentInput{ProductID: e.product.ID, Name: "Server"})
			if err != nil {
				t.Fatal(err)
			}
			if got := outcomes(bulk.Apply(ctx, e.dev, req(BulkComponent, comp.ID, a.ID))); got[a.ID] != nil {
				t.Fatal(got)
			}
			tc, _ := e.svc.Testing.GetTestCase(ctx, e.owner, e.product.ID, a.ID)
			if tc.ComponentID != comp.ID {
				t.Errorf("component %q", tc.ComponentID)
			}
			if got := outcomes(bulk.Apply(ctx, e.dev, req(BulkComponent, "", a.ID))); got[a.ID] != nil {
				t.Fatal(got)
			}
			if tc, _ = e.svc.Testing.GetTestCase(ctx, e.owner, e.product.ID, a.ID); tc.ComponentID != "" {
				t.Error("component not cleared")
			}
		})

		t.Run("testers are refused, delete previews without deleting", func(t *testing.T) {
			got := outcomes(bulk.Apply(ctx, tester, req(BulkComponent, "", a.ID)))
			if !errors.Is(got[a.ID], ErrForbidden) {
				t.Errorf("tester: %v", got)
			}
			got = outcomes(bulk.Preview(ctx, tester, req(BulkDelete, "", a.ID)))
			if !errors.Is(got[a.ID], ErrForbidden) {
				t.Errorf("tester delete preview: %v", got)
			}
			if got = outcomes(bulk.Preview(ctx, e.dev, req(BulkDelete, "", b.ID))); got[b.ID] != nil {
				t.Fatal(got)
			}
			if _, err := e.svc.Testing.GetTestCase(ctx, e.owner, e.product.ID, b.ID); err != nil {
				t.Fatal("the preview deleted the test case")
			}
			if got = outcomes(bulk.Apply(ctx, e.dev, req(BulkDelete, "", b.ID))); got[b.ID] != nil {
				t.Fatal(got)
			}
			if _, err := e.svc.Testing.GetTestCase(ctx, e.owner, e.product.ID, b.ID); !errors.Is(err, ErrNotFound) {
				t.Errorf("deleted test case: %v", err)
			}
		})

		t.Run("backlog operations do not apply, and vice versa", func(t *testing.T) {
			if _, err := bulk.Preview(ctx, e.owner, req(BulkStatus, "x", a.ID)); !IsBulkInvalid(err) {
				t.Errorf("status of a test case: %v", err)
			}
			if _, err := bulk.Preview(ctx, e.owner, req(BulkLinkRequirement, "", a.ID)); !IsBulkInvalid(err) {
				t.Errorf("link without a requirement: %v", err)
			}
			r := e.requirement(t)
			back := BulkRequest{ProductID: e.product.ID, Kind: model.RequirementKindRequirement, Action: BulkComponent, IDs: []string{r.ID}}
			if _, err := bulk.Preview(ctx, e.owner, back); !IsBulkInvalid(err) {
				t.Errorf("component of a requirement: %v", err)
			}
			got := outcomes(bulk.Preview(ctx, e.owner, req(BulkComponent, "", r.ID)))
			if !errors.Is(got[r.ID], ErrNotFound) {
				t.Errorf("a requirement in a test case request: %v", got)
			}
		})
	})
}
