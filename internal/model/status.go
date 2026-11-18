package model

// Status represents a configurable workflow status column
// for requirements or defects within a product.
type Status struct {
	ID         string
	ProductID  string
	EntityType string // "requirement" or "defect"
	Name       string
	Position   int
	IsDefault  bool
	IsClosed   bool
	// IsActive marks a kanban column as activated for work (FR-91, FR-116):
	// developers may move items out of it. Only owners move items into an
	// active column from an inactive one. Ignored in scrum mode, where
	// planning an item into a sprint activates it.
	IsActive  bool
	CreatedAt string // RFC 3339
}
