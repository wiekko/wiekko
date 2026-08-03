package model

// Sprint represents a time-boxed iteration within a product.
type Sprint struct {
	ID          string
	ProductID   string
	Name        string
	MilestoneID string // empty if not linked
	Status      string // "planned", "active", "completed"
	StartDate   string // RFC 3339
	EndDate     string // RFC 3339
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}

// Sprint status constants. A sprint is planned until someone starts it and
// completed once it is closed; at most one sprint per product is active.
const (
	SprintStatusPlanned   = "planned"
	SprintStatusActive    = "active"
	SprintStatusCompleted = "completed"
)

// ValidSprintStatus reports whether s is a known sprint status.
func ValidSprintStatus(s string) bool {
	switch s {
	case SprintStatusPlanned, SprintStatusActive, SprintStatusCompleted:
		return true
	default:
		return false
	}
}
