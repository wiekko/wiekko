// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestGetBySequence(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		now := Now()
		exec := func(query string, args ...any) {
			t.Helper()
			if _, err := db.Exec(query, args...); err != nil {
				t.Fatalf("seed (%s): %v", query, err)
			}
		}

		// Two products, each with an item numbered 1, so a lookup that ignores
		// the product would be caught.
		products := []string{uuid.NewString(), uuid.NewString()}
		reqIDs := map[string]string{}
		conIDs := map[string]string{}
		for i, p := range products {
			exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
			      VALUES (?, ?, '', 'kanban', 'none', ?, ?)`, p, "P"+string(rune('A'+i)), now, now)
			fr, ac := uuid.NewString(), uuid.NewString()
			exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
			      VALUES (?, ?, 'requirement', 'Functionality', 'FR', 0, ?)`, fr, p, now)
			exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
			      VALUES (?, ?, 'constraint', 'Architecture', 'AC', 0, ?)`, ac, p, now)
			reqIDs[p] = uuid.NewString()
			exec(`INSERT INTO requirements (id, product_id, kind, category_id, title, sequence_number, created_at, updated_at)
			      VALUES (?, ?, 'requirement', ?, 'R', 1, ?, ?)`, reqIDs[p], p, fr, now, now)
			conIDs[p] = uuid.NewString()
			exec(`INSERT INTO constraints (id, product_id, category_id, sequence_number, title, created_at, updated_at)
			      VALUES (?, ?, ?, 1, 'C', ?, ?)`, conIDs[p], p, ac, now, now)
		}

		reqs := &requirementStore{db: db}
		cons := &constraintStore{db: db}
		for _, p := range products {
			r, err := reqs.GetBySequence(ctx, p, 1)
			if err != nil || r.ID != reqIDs[p] || r.SequenceNumber != 1 {
				t.Fatalf("requirement 1 of %s = %+v, %v", p, r, err)
			}
			c, err := cons.GetBySequence(ctx, p, 1)
			if err != nil || c.ID != conIDs[p] {
				t.Fatalf("constraint 1 of %s = %+v, %v", p, c, err)
			}
		}
		if _, err := reqs.GetBySequence(ctx, products[0], 2); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("missing requirement: err = %v, want sql.ErrNoRows", err)
		}
		if _, err := cons.GetBySequence(ctx, products[0], 2); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("missing constraint: err = %v, want sql.ErrNoRows", err)
		}
	})
}
