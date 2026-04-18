package model

import "fmt"

// Requirement represents a backlog item within a product.
type Requirement struct {
	ID               string
	ProductID        string
	ParentID         string // empty if top-level
	CategoryID       string
	SequenceNumber   int // auto-assigned per product, used for identifier
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

// EntityIdentifier returns the display identifier (e.g. "FR-1", "AC-42")
// for a requirement or constraint, given its category prefix.
func EntityIdentifier(prefix string, seqNum int) string {
	return fmt.Sprintf("%s-%d", prefix, seqNum)
}
