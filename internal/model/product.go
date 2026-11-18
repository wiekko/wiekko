package model

// Product represents a top-level product entity.
type Product struct {
	ID             string
	Name           string
	Description    string
	EstimationMode string // "disabled", "story_points", "tshirt_sizes", "hours"
	WorkflowMode   string // "kanban", "scrum"
	StatusSync     bool   // true = statuses synced between requirements and defects
	DefectPrefix   string // prefix of defect identifiers, e.g. "DF" in DF-17 (FR-49)
	// DocumentPrefix is the prefix of document identifiers, e.g. "DOC" in DOC-3 (FR-56).
	DocumentPrefix string
	// TwoFactorRequired keeps members without 2FA out of the product (SC-34).
	TwoFactorRequired bool
	CreatedAt         string // RFC 3339
	UpdatedAt         string // RFC 3339
}

// DefaultDefectPrefix is the defect identifier prefix of new products.
const DefaultDefectPrefix = "DF"

// Estimation mode constants.
const (
	EstimationModeDisabled    = "disabled"
	EstimationModeStoryPoints = "story_points"
	EstimationModeTShirtSizes = "tshirt_sizes"
	EstimationModeHours       = "hours"
)

// Workflow mode constants.
const (
	WorkflowModeKanban = "kanban"
	WorkflowModeScrum  = "scrum"
)
