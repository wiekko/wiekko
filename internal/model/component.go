// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package model

// Component represents a structural part of a product.
type Component struct {
	ID          string
	ProductID   string
	Name        string
	Description string
	MilestoneID string // empty if not linked
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}
