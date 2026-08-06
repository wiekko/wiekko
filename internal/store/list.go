package store

import "strings"

// Sort directions accepted by ListParams.
const (
	SortAsc  = "asc"
	SortDesc = "desc"
)

// ListParams carries sort, search and paging state from the HTTP layer down to
// a List query. Sort is a canonical key (e.g. "criticality"), never a column
// name: each store resolves it against its own allowlist, so a caller can never
// steer the ORDER BY clause.
type ListParams struct {
	Sort   string // "" selects the entity's default order
	Dir    string // SortAsc or SortDesc; anything else reads as ascending
	Search string // global substring match; "" matches everything
	Limit  int    // 0 means no limit (and makes Offset meaningless)
	Offset int
}

// orderClause resolves p.Sort against allow and returns a complete ORDER BY
// clause. Unknown keys fall back to fallback rather than erroring: a stale
// bookmark should render the default order, not a 400.
//
// tiebreak is appended unconditionally and must be unique per row. Without it
// LIMIT/OFFSET paging over a non-unique sort key (priority, criticality) has no
// defined order between rows that compare equal, so the same row can appear on
// two pages while another is never shown at all.
func orderClause(p ListParams, allow map[string]string, fallback, tiebreak string) string {
	expr, ok := allow[p.Sort]
	if !ok {
		return ` ORDER BY ` + fallback + `, ` + tiebreak
	}
	dir := ` ASC`
	if p.Dir == SortDesc {
		dir = ` DESC`
	}
	return ` ORDER BY ` + expr + dir + `, ` + tiebreak
}

// limitClause renders LIMIT/OFFSET and its arguments. A zero limit disables
// paging entirely so callers that want the full list need no special case.
func limitClause(p ListParams) (string, []any) {
	if p.Limit <= 0 {
		return "", nil
	}
	if p.Offset <= 0 {
		return ` LIMIT ?`, []any{p.Limit}
	}
	return ` LIMIT ? OFFSET ?`, []any{p.Limit, p.Offset}
}

// LikeEscape is the escape character used by every LIKE predicate built here.
// Queries must spell it out with `ESCAPE '\'` — the default differs between
// engines, and without it a search for "50%" would match everything.
const LikeEscape = `\`

// searchPattern turns a user's search string into a LIKE pattern. Callers match
// it against LOWER(column) so the comparison is case-insensitive on both SQLite
// and Postgres without resorting to ILIKE.
//
// The wildcards a user can type (% and _) are escaped so they match literally:
// searching for "100%" should find "100% CPU", not every row in the table.
func searchPattern(search string) string {
	r := strings.NewReplacer(
		LikeEscape, LikeEscape+LikeEscape,
		"%", LikeEscape+"%",
		"_", LikeEscape+"_",
	)
	return "%" + r.Replace(strings.ToLower(strings.TrimSpace(search))) + "%"
}
