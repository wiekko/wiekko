// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package model

// Milestone represents a point on a product's roadmap.
type Milestone struct {
	ID          string
	ProductID   string
	Name        string
	Description string
	Type        string // "planning", "release", "internal"
	DueDate     string // empty if no due date; RFC 3339
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}

// Milestone type constants.
const (
	MilestoneTypePlanning = "planning"
	MilestoneTypeRelease  = "release"
	MilestoneTypeInternal = "internal"
)
