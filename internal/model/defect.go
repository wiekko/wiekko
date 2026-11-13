package model

// Defect represents a bug or issue within a product.
type Defect struct {
	ID        string
	ProductID string
	// SequenceNumber numbers the product's defects (FR-043); with the product's
	// defect prefix it forms the identifier, e.g. DF-17.
	SequenceNumber int
	// Identifier is the human-readable identifier, filled in when read from
	// the store with the product's current prefix.
	Identifier  string
	Title       string
	Description string // Markdown
	Criticality string // "critical", "high", "medium", "low"
	StatusID    string // empty if no status assigned
	MilestoneID string // empty if not linked
	SprintID    string // empty if not in a sprint
	// EnvironmentID names the runtime environment the defect was observed in
	// ("login fails in Firefox"); empty if it is not environment specific.
	EnvironmentID string
	Estimation    string // interpreted per product's estimation mode
	Priority      int
	IsActive      bool   // activated for work
	CreatedAt     string // RFC 3339
	UpdatedAt     string // RFC 3339
}

// Defect criticality constants.
const (
	DefectCriticalityCritical = "critical"
	DefectCriticalityHigh     = "high"
	DefectCriticalityMedium   = "medium"
	DefectCriticalityLow      = "low"
)
