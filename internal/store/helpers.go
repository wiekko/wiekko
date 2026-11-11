package store

import "log/slog"

// scanner is satisfied by both *Row and *Rows.
type scanner interface {
	Scan(dest ...any) error
}

// closeRows closes rows and logs any error.
func closeRows(rows *Rows) {
	if err := rows.Close(); err != nil {
		slog.Error("failed to close rows", "err", err)
	}
}

// nullableText returns nil for empty strings, enabling SQL NULL storage.
func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// activeColumn is the select expression for a backlog item's is_active, for
// the requirements or defects table (or its alias) named by table.
//
// In a kanban product, activation is a property of the column (FR-318): an
// item is active while its status is marked is_active, and the stored flag is
// not consulted. In a scrum product the stored flag holds, set when an item is
// planned into a sprint. Computing it here, rather than copying the column's
// flag onto every item, means toggling a column or switching the workflow
// mode takes effect without rewriting items.
func activeColumn(table string) string {
	return `CASE WHEN (SELECT ap.workflow_mode FROM products ap WHERE ap.id = ` + table + `.product_id) = 'kanban'
		THEN COALESCE((SELECT ast.is_active FROM statuses ast WHERE ast.id = ` + table + `.status_id), FALSE)
		ELSE ` + table + `.is_active END`
}
