package model

// Requirement represents a backlog item within a product.
type Requirement struct {
	ID               string
	ProductID        string
	ParentID         string // empty if top-level
	Category         string // "architecture", "security", "functionality", "ux"
	Title            string
	Description      string // Markdown
	DefinitionOfDone string // Markdown
	StatusID         string // empty if no status assigned
	MilestoneID      string // empty if not linked
	SprintID         string // empty if not in a sprint
	Estimation       string // interpreted per product's estimation mode
	Priority         int
	IsCoarse         bool   // needs refinement
	IsActive         bool   // activated for work
	CreatedAt        string // RFC 3339
	UpdatedAt        string // RFC 3339
}

// Requirement category constants.
const (
	RequirementCategoryArchitecture  = "architecture"
	RequirementCategorySecurity      = "security"
	RequirementCategoryFunctionality = "functionality"
	RequirementCategoryUX            = "ux"
)
