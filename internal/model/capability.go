package model

// Capability represents a testable feature area of a product,
// optionally linked to a component.
type Capability struct {
	ID          string
	ProductID   string
	ComponentID string // empty if top-level
	Name        string
	Description string
	MilestoneID string // empty if not linked
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}

// RequirementCapability represents a many-to-many link
// between a requirement and a capability.
type RequirementCapability struct {
	ID            string
	RequirementID string
	CapabilityID  string
	CreatedAt     string // RFC 3339
}
