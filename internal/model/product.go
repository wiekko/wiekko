package model

// Product represents a top-level product entity.
type Product struct {
	ID             string
	Name           string
	Description    string
	EstimationMode string // "disabled", "story_points", "tshirt_sizes", "hours"
	WorkflowMode   string // "kanban", "scrum"
	CreatedAt      string // RFC 3339
	UpdatedAt      string // RFC 3339
}

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
