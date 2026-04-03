package model

// Sprint represents a time-boxed iteration within a product.
type Sprint struct {
	ID          string
	ProductID   string
	Name        string
	MilestoneID string // empty if not linked
	StartDate   string // RFC 3339
	EndDate     string // RFC 3339
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}
