// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"wiekko.net/internal/model"
)

// TestCapabilityLinksAreIdempotent covers DF-3: linking a requirement or a
// constraint to a capability it is already linked to succeeds and keeps the
// one existing link, instead of failing on the unique index.
func TestCapabilityLinksAreIdempotent(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		now := Now()
		exec := func(query string, args ...any) {
			t.Helper()
			if _, err := db.Exec(query, args...); err != nil {
				t.Fatalf("seed (%s): %v", query, err)
			}
		}

		p, fr, ac := uuid.NewString(), uuid.NewString(), uuid.NewString()
		capID, reqID, conID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
		      VALUES (?, 'Links', '', 'kanban', 'none', ?, ?)`, p, now, now)
		exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
		      VALUES (?, ?, 'requirement', 'Functionality', 'FR', 0, ?)`, fr, p, now)
		exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
		      VALUES (?, ?, 'constraint', 'Architecture', 'AC', 0, ?)`, ac, p, now)
		exec(`INSERT INTO requirements (id, product_id, kind, category_id, title, sequence_number, created_at, updated_at)
		      VALUES (?, ?, 'requirement', ?, 'R', 1, ?, ?)`, reqID, p, fr, now, now)
		exec(`INSERT INTO constraints (id, product_id, category_id, sequence_number, title, created_at, updated_at)
		      VALUES (?, ?, ?, 1, 'C', ?, ?)`, conID, p, ac, now, now)
		exec(`INSERT INTO capabilities (id, product_id, name, created_at, updated_at)
		      VALUES (?, ?, 'Search', ?, ?)`, capID, p, now, now)

		caps := &capabilityStore{db: db}
		cons := &constraintStore{db: db}
		for i := range 2 {
			if err := caps.LinkRequirement(ctx, &model.RequirementCapability{
				ID: uuid.NewString(), RequirementID: reqID, CapabilityID: capID, CreatedAt: now,
			}); err != nil {
				t.Fatalf("LinkRequirement #%d: %v", i+1, err)
			}
			if err := cons.LinkCapability(ctx, &model.ConstraintCapability{
				ID: uuid.NewString(), ConstraintID: conID, CapabilityID: capID, CreatedAt: now,
			}); err != nil {
				t.Fatalf("LinkCapability #%d: %v", i+1, err)
			}
		}

		reqLinks, err := caps.ListRequirementsByCapability(ctx, capID)
		if err != nil || len(reqLinks) != 1 || reqLinks[0].RequirementID != reqID {
			t.Errorf("requirement links = %+v, %v; want exactly one", reqLinks, err)
		}
		conLinks, err := cons.ListConstraintsByCapability(ctx, capID)
		if err != nil || len(conLinks) != 1 || conLinks[0].ConstraintID != conID {
			t.Errorf("constraint links = %+v, %v; want exactly one", conLinks, err)
		}
	})
}
