package store

// Priorities rank with the lowest number first: 1 is the highest priority.
// 0, the default, means no priority is set; such items sort after every
// prioritised one, as do negative values (DF-2).

// priorityRank turns a priority column into an ascending sort key.
func priorityRank(col string) string {
	return `CASE WHEN ` + col + ` > 0 THEN ` + col + ` ELSE 2147483647 END`
}

// itemOrder orders requirements, tasks and constraints: by priority, then by
// creation. created_at has one-second resolution, so items created in the same
// second fall back to their identifier and finally to their ID, giving every
// list a stable order (DF-2).
var itemOrder = priorityRank("priority") + `, created_at, sequence_number, id`
