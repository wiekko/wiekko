package model

// Comment represents a user comment on any entity.
type Comment struct {
	ID        string
	OwnerType string // "requirement", "test_case", "component", "defect"
	OwnerID   string
	UserID    string
	Body      string // Markdown
	CreatedAt string // RFC 3339
	UpdatedAt string // RFC 3339
}

// Attachment represents a file attached to any entity.
type Attachment struct {
	ID          string
	OwnerType   string // "requirement", "test_case", "component", "defect"
	OwnerID     string
	UserID      string
	Filename    string
	ContentType string
	Size        int64
	StoragePath string
	CreatedAt   string // RFC 3339
}

// AuditEntry represents a single field-level change in the audit trail.
type AuditEntry struct {
	ID         string
	EntityType string // "product", "component", "requirement", etc.
	EntityID   string
	UserID     string
	Action     string // "create", "update", "delete"
	FieldName  string // empty for create/delete
	OldValue   string // empty for create
	NewValue   string // empty for delete
	CreatedAt  string // RFC 3339
}

// DefectLink represents a polymorphic link from a defect
// to a component, functionality, or test case.
type DefectLink struct {
	ID         string
	DefectID   string
	TargetType string // "component", "functionality", "test_case"
	TargetID   string
	CreatedAt  string // RFC 3339
}

// Assignment represents a user assigned to a requirement or defect.
type Assignment struct {
	ID        string
	TargetID  string // requirement_id or defect_id
	UserID    string
	CreatedAt string // RFC 3339
}

// TestCaseRequirement represents a link between a test case and a requirement.
type TestCaseRequirement struct {
	ID            string
	TestCaseID    string
	RequirementID string
	LinkType      string // "requirement" or "definition_of_done"
	CreatedAt     string // RFC 3339
}
