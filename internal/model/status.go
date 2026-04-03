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
	CreatedAt  string // RFC 3339
}
