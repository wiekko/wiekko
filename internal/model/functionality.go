package model

// Functionality represents a functional area of a product,
// optionally linked to a component.
type Functionality struct {
	ID          string
	ProductID   string
	ComponentID string // empty if top-level
	Name        string
	Description string
	MilestoneID string // empty if not linked
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}
