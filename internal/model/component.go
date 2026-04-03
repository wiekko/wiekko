package model

// Component represents a structural part of a product.
type Component struct {
	ID          string
	ProductID   string
	Name        string
	Description string
	MilestoneID string // empty if not linked
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}
