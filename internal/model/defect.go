package model

// Defect represents a bug or issue within a product.
type Defect struct {
	ID          string
	ProductID   string
	Title       string
	Description string // Markdown
	Criticality string // "critical", "high", "medium", "low"
	StatusID    string // empty if no status assigned
	MilestoneID string // empty if not linked
	SprintID    string // empty if not in a sprint
	Estimation  string // interpreted per product's estimation mode
	Priority    int
	IsActive    bool   // activated for work
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}

// Defect criticality constants.
const (
	DefectCriticalityCritical = "critical"
	DefectCriticalityHigh     = "high"
	DefectCriticalityMedium   = "medium"
	DefectCriticalityLow      = "low"
)
