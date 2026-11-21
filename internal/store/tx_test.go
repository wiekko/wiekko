// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// seedInTx inserts a product with the transaction ctx carries.
func seedInTx(ctx context.Context, db *DB, id, name string) error {
	now := Now()
	_, err := db.ExecContext(ctx, `INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
	      VALUES (?, ?, '', 'scrum', 'none', ?, ?)`, id, name, now, now)
	return err
}

func TestInTxOutcome(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		boom := errors.New("boom")

		// Committed.
		if err := db.InTx(ctx, func(ctx context.Context) error {
			return seedInTx(ctx, db, "10000000-0000-0000-0000-000000000001", "kept")
		}); err != nil {
			t.Fatal(err)
		}
		// Rolled back on error, including a nested InTx and a store method
		// that opens its own transaction (sprintStore.Delete).
		sprints := &sprintStore{db: db}
		err := db.InTx(ctx, func(ctx context.Context) error {
			if err := seedInTx(ctx, db, "10000000-0000-0000-0000-000000000002", "dropped"); err != nil {
				return err
			}
			if err := db.InTx(ctx, func(ctx context.Context) error {
				return seedInTx(ctx, db, "10000000-0000-0000-0000-000000000003", "nested")
			}); err != nil {
				return err
			}
			sp := makeStoredSprint("10000000-0000-0000-0000-000000000002", "S")
			if err := sprints.Create(ctx, sp); err != nil {
				return err
			}
			if err := sprints.Delete(ctx, sp.ID); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want boom", err)
		}
		// Rolled back on panic.
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("panic not propagated")
				}
			}()
			_ = db.InTx(ctx, func(ctx context.Context) error {
				if err := seedInTx(ctx, db, "10000000-0000-0000-0000-000000000004", "panicked"); err != nil {
					return err
				}
				panic("boom")
			})
		}()

		rows, err := db.QueryContext(ctx, `SELECT name FROM products WHERE CAST(id AS TEXT) LIKE '10000000-%' ORDER BY name`)
		if err != nil {
			t.Fatal(err)
		}
		defer closeRows(rows)
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			names = append(names, n)
		}
		if len(names) != 1 || names[0] != "kept" {
			t.Fatalf("products = %v, want [kept]", names)
		}
	})
}

func TestLockOutsideTx(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		if err := db.Lock(context.Background(), "k"); err == nil {
			t.Fatal("Lock outside a transaction succeeded")
		}
	})
}

// TestLockSerializes: a second transaction taking the same key waits until the
// first ends. On SQLite the single connection gives the same guarantee.
func TestLockSerializes(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		if db.Dialect() != Postgres {
			t.Skip("SQLite serializes transactions on its single connection")
		}
		ctx := context.Background()
		held := make(chan struct{})
		var mu sync.Mutex
		var order []string
		mark := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = db.InTx(ctx, func(ctx context.Context) error {
				if err := db.Lock(ctx, "k"); err != nil {
					t.Error(err)
				}
				close(held)
				time.Sleep(200 * time.Millisecond)
				mark("first done")
				return nil
			})
		}()
		<-held
		if err := db.InTx(ctx, func(ctx context.Context) error {
			if err := db.Lock(ctx, "k"); err != nil {
				return err
			}
			mark("second locked")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		if len(order) != 2 || order[0] != "first done" {
			t.Fatalf("order = %v, want first done before second locked", order)
		}
	})
}
