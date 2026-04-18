package model

// Constraint represents a non-functional requirement (architecture, security, UX rule)
// within a product. Constraints are ongoing rules that are met or violated,
// rather than implementable work items.
type Constraint struct {
	ID              string
	ProductID       string
	CategoryID      string
	SequenceNumber  int
	Title           string
	Description     string // Markdown
	ComplianceState string // "not_assessed", "met", "violated"
	MilestoneID     string // empty if not linked
	Priority        int
	CreatedAt       string // RFC 3339
	UpdatedAt       string // RFC 3339
}

// Compliance state constants.
const (
	ComplianceNotAssessed = "not_assessed"
	ComplianceMet         = "met"
	ComplianceViolated    = "violated"
)

// ConstraintCapability represents a many-to-many link
// between a constraint and a capability.
type ConstraintCapability struct {
	ID           string
	ConstraintID string
	CapabilityID string
	CreatedAt    string // RFC 3339
}
