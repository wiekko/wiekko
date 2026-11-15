package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// These tests run the multi-step writes on both engines and check that a
// failure part way leaves nothing behind, and that concurrent requests cannot
// break the invariants the reads before a write rely on.

// failingAudit fails every audit write for one action, standing in for the
// last step of a unit of work going wrong.
type failingAudit struct {
	store.AuditStore
	action string
}

func (f failingAudit) Create(ctx context.Context, e *model.AuditEntry) error {
	if e.Action == f.action {
		return errors.New("audit trail unavailable")
	}
	return f.AuditStore.Create(ctx, e)
}

func countRows(t *testing.T, conn *sql.DB, driver, table, column, value string) int {
	t.Helper()
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = ?`, table, column)
	if driver == dbtest.Postgres {
		q = fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = $1`, table, column)
	}
	var n int
	if err := conn.QueryRow(q, value).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeleteRollsBackAsAWhole(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		d, err := e.svc.Document.CreateDocument(ctx, e.owner, CreateDocumentInput{ProductID: e.product.ID, Title: "Spec", Body: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Discussion.CreateComment(ctx, e.owner, e.product.ID, model.OwnerTypeDocument, d.ID, "keep me"); err != nil {
			t.Fatal(err)
		}

		e.svc.Document.audit = failingAudit{AuditStore: e.svc.Document.audit, action: "delete"}
		if err := e.svc.Document.DeleteDocument(ctx, e.owner, e.product.ID, d.ID); err == nil {
			t.Fatal("delete succeeded although its audit entry failed")
		}
		if n := countRows(t, conn, driver, "documents", "id", d.ID); n != 1 {
			t.Fatalf("document rows = %d, want 1 (rolled back)", n)
		}
		if n := countRows(t, conn, driver, "comments", "owner_id", d.ID); n != 1 {
			t.Fatalf("comment rows = %d, want 1 (rolled back with the delete)", n)
		}
	})
}

func TestDeleteRemovesDefectLinksToTarget(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		r := e.requirement(t)
		d, err := e.svc.Backlog.CreateDefect(ctx, e.owner, CreateDefectInput{ProductID: e.product.ID, Title: "Broken"})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Backlog.AddDefectLink(ctx, e.owner, e.product.ID, d.ID, model.DefectLinkTargetRequirement, r.ID); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Backlog.DeleteRequirement(ctx, e.owner, e.product.ID, r.ID); err != nil {
			t.Fatal(err)
		}
		if n := countRows(t, conn, driver, "defect_links", "target_id", r.ID); n != 0 {
			t.Fatalf("defect links to the deleted requirement = %d, want 0", n)
		}
	})
}

func TestTransferOwnershipKeepsAnOwner(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		// No such user: the promotion fails on the foreign key.
		if err := e.svc.Product.TransferOwnership(ctx, e.owner, e.product.ID, uuid.NewString()); err == nil {
			t.Fatal("transfer to a missing user succeeded")
		}
		members, err := e.svc.Product.ListMembers(ctx, e.owner, e.product.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range members {
			if m.UserID == e.owner.ID && m.Role != model.RoleOwner {
				t.Fatalf("owner demoted to %s by a failed transfer", m.Role)
			}
		}

		if err := e.svc.Product.TransferOwnership(ctx, e.owner, e.product.ID, e.dev.ID); err != nil {
			t.Fatal(err)
		}
		roles := map[string]string{}
		members, _ = e.svc.Product.ListMembers(ctx, e.owner, e.product.ID)
		for _, m := range members {
			roles[m.UserID] = m.Role
		}
		if roles[e.dev.ID] != model.RoleOwner || roles[e.owner.ID] != model.RoleDeveloper {
			t.Fatalf("roles after transfer = %v", roles)
		}
	})
}

func TestInvitationAcceptedOnce(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		inv, err := e.svc.Invitation.Create(ctx, e.owner, "guest@example.com", e.product.ID, model.RoleViewer)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 4)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = e.svc.User.AcceptInvitation(ctx, inv.RawToken, RegisterInput{
					Username: fmt.Sprintf("guest%d", i), Password: "Correct-Horse-9",
				})
			}(i)
		}
		wg.Wait()
		ok := 0
		for _, err := range errs {
			if err == nil {
				ok++
			}
		}
		if ok != 1 {
			t.Fatalf("%d accepts succeeded (errors %v), want exactly 1", ok, errs)
		}
		if n := countRows(t, conn, driver, "product_roles", "product_id", e.product.ID); n != 3 {
			t.Fatalf("product members = %d, want 3 (owner, developer, one guest)", n)
		}
	})
}

func TestConcurrentCreatesDrawDistinctNumbers(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		const n = 8
		var wg sync.WaitGroup
		var mu sync.Mutex
		seen := map[string]bool{}
		for i := 0; i < n; i++ {
			wg.Add(2)
			go func(i int) {
				defer wg.Done()
				d, err := e.svc.Backlog.CreateDefect(ctx, e.owner, CreateDefectInput{ProductID: e.product.ID, Title: fmt.Sprint("D", i)})
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					t.Errorf("defect %d: %v", i, err)
					return
				}
				seen[fmt.Sprint("defect ", d.SequenceNumber)] = true
			}(i)
			go func(i int) {
				defer wg.Done()
				r, err := e.svc.Backlog.CreateRequirement(ctx, e.owner, CreateRequirementInput{ProductID: e.product.ID, Title: fmt.Sprint("R", i)})
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					t.Errorf("requirement %d: %v", i, err)
					return
				}
				seen[fmt.Sprint("requirement ", r.SequenceNumber)] = true
			}(i)
		}
		wg.Wait()
		if len(seen) != 2*n {
			t.Fatalf("distinct numbers = %d, want %d: %v", len(seen), 2*n, seen)
		}
	})
}

func TestDeletingAnAuthorKeepsTheirComments(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		r := e.requirement(t)
		if _, err := e.svc.Discussion.CreateComment(ctx, e.dev, e.product.ID, model.OwnerTypeRequirement, r.ID, "from dev"); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.User.AdminDeleteUser(ctx, e.dev.ID); err != nil {
			t.Fatalf("delete an author: %v", err)
		}
		comments, err := e.svc.Discussion.ListComments(ctx, e.owner, e.product.ID, model.OwnerTypeRequirement, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(comments) != 1 || comments[0].UserID != "" || comments[0].Body != "from dev" {
			t.Fatalf("comments = %+v, want the comment kept without an author", comments)
		}
	})
}

func TestOneActiveSprint(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		var ids []string
		for i := 0; i < 2; i++ {
			sp, err := e.svc.Milestone.CreateSprint(ctx, e.owner, CreateSprintInput{
				ProductID: e.product.ID, Name: fmt.Sprint("S", i), StartDate: "2026-01-01", EndDate: "2026-01-14",
			})
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, sp.ID)
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, id := range ids {
			wg.Add(1)
			go func(i int, id string) {
				defer wg.Done()
				_, errs[i] = e.svc.Milestone.SetSprintStatus(ctx, e.owner, e.product.ID, id, model.SprintStatusActive)
			}(i, id)
		}
		wg.Wait()
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatalf("errors = %v, want exactly one sprint started", errs)
		}
	})
}

func TestStaleUpdateIsAConflict(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		r := e.requirement(t)
		title := "Edited"
		if _, err := e.svc.Backlog.UpdateRequirement(ctx, e.owner, e.product.ID, r.ID, UpdateRequirementInput{
			Title: &title, IfUpdatedAt: "2000-01-01T00:00:00Z",
		}); !errors.Is(err, ErrConflict) {
			t.Fatalf("stale update: err = %v, want ErrConflict", err)
		}
		if _, err := e.svc.Backlog.UpdateRequirement(ctx, e.owner, e.product.ID, r.ID, UpdateRequirementInput{
			Title: &title, IfUpdatedAt: r.UpdatedAt,
		}); err != nil {
			t.Fatalf("current update: %v", err)
		}
	})
}
