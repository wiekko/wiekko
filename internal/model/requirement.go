// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package model

import "fmt"

// Requirement kinds. Requirements and tasks share one table and one status
// set; the kind distinguishes a statement about the product (requirement)
// from work that carries no product statement of its own (task).
const (
	RequirementKindRequirement = "requirement"
	RequirementKindTask        = "task"
)

// ValidRequirementKind reports whether kind is a supported backlog item kind.
func ValidRequirementKind(kind string) bool {
	return kind == RequirementKindRequirement || kind == RequirementKindTask
}

// Requirement represents a backlog item within a product.
type Requirement struct {
	ID               string
	ProductID        string
	Kind             string // "requirement" or "task"
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
	IsCoarse         bool // needs refinement
	IsActive         bool // activated for work
	// ArchivedAt is when the item was archived (FR-155), RFC 3339; empty if
	// it is not. An archived item is always in a closed status.
	ArchivedAt string
	CreatedAt  string // RFC 3339
	UpdatedAt  string // RFC 3339
}

// Archived reports whether the item has been archived (FR-155).
func (r *Requirement) Archived() bool { return r.ArchivedAt != "" }

// EntityIdentifier returns the display identifier (e.g. "FR-1", "AC-42")
// for a requirement or constraint, given its category prefix.
func EntityIdentifier(prefix string, seqNum int) string {
	return fmt.Sprintf("%s-%d", prefix, seqNum)
}
