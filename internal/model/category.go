package model

// Category represents a configurable category for requirements or constraints,
// scoped to a product.
type Category struct {
	ID         string
	ProductID  string
	EntityType string // "requirement", "task" or "constraint"
	Name       string
	Prefix     string // e.g. "FR", "AR", "AC"
	Position   int
	CreatedAt  string // RFC 3339
}
