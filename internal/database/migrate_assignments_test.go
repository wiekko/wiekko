package database_test

import (
	"database/sql"
	"testing"

	"nexmap.net/internal/database"
)

// TestMigrationEndsAssignmentsWithMembership checks migration 6 (UR-171):
// assignments of users who are no member of the item's product are removed,
// those of members kept.
func TestMigrationEndsAssignmentsWithMembership(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		upTo5, err := database.MigrationsUpTo(driver, 5)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.MigrateFS(db, driver, upTo5, nil); err != nil {
			t.Fatalf("migrate to 5: %v", err)
		}
		const (
			now      = "2026-01-01T00:00:00Z"
			memberID = "00000000-0000-4000-8000-000000000001"
			formerID = "00000000-0000-4000-8000-000000000002"
		)
		for _, q := range []string{
			`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES ('00000000-0000-4000-8000-000000000001', 'member', 'x', '` + now + `', '` + now + `')`,
			`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES ('00000000-0000-4000-8000-000000000002', 'former', 'x', '` + now + `', '` + now + `')`,
			`INSERT INTO products (id, name, created_at, updated_at) VALUES ('00000000-0000-4000-8000-000000000003', 'Aviary', '` + now + `', '` + now + `')`,
			`INSERT INTO product_roles (id, user_id, product_id, role, created_at) VALUES ('00000000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000003', 'developer', '` + now + `')`,
			`INSERT INTO categories (id, product_id, entity_type, name, prefix, created_at) VALUES ('00000000-0000-4000-8000-000000000005', '00000000-0000-4000-8000-000000000003', 'requirement', 'Functional', 'FR', '` + now + `')`,
			`INSERT INTO requirements (id, product_id, category_id, title, created_at, updated_at) VALUES ('00000000-0000-4000-8000-000000000006', '00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000005', 'Record a sighting', '` + now + `', '` + now + `')`,
			`INSERT INTO defects (id, product_id, title, created_at, updated_at) VALUES ('00000000-0000-4000-8000-000000000007', '00000000-0000-4000-8000-000000000003', 'Crash', '` + now + `', '` + now + `')`,
			`INSERT INTO requirement_assignments (id, requirement_id, user_id, created_at) VALUES ('00000000-0000-4000-8000-000000000011', '00000000-0000-4000-8000-000000000006', '00000000-0000-4000-8000-000000000001', '` + now + `')`,
			`INSERT INTO requirement_assignments (id, requirement_id, user_id, created_at) VALUES ('00000000-0000-4000-8000-000000000012', '00000000-0000-4000-8000-000000000006', '00000000-0000-4000-8000-000000000002', '` + now + `')`,
			`INSERT INTO defect_assignments (id, defect_id, user_id, created_at) VALUES ('00000000-0000-4000-8000-000000000013', '00000000-0000-4000-8000-000000000007', '00000000-0000-4000-8000-000000000001', '` + now + `')`,
			`INSERT INTO defect_assignments (id, defect_id, user_id, created_at) VALUES ('00000000-0000-4000-8000-000000000014', '00000000-0000-4000-8000-000000000007', '00000000-0000-4000-8000-000000000002', '` + now + `')`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}

		if err := database.Migrate(db, driver, nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		for _, tc := range []struct {
			table, user string
			want        int
		}{
			{"requirement_assignments", memberID, 1},
			{"requirement_assignments", formerID, 0},
			{"defect_assignments", memberID, 1},
			{"defect_assignments", formerID, 0},
		} {
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tc.table + ` WHERE user_id = '` + tc.user + `'`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != tc.want {
				t.Errorf("%s of %s: %d, want %d", tc.table, tc.user, n, tc.want)
			}
		}
	})
}
