package model

// Comment represents a user comment on any entity.
type Comment struct {
	ID        string
	ProductID string // owning product; scopes reads and writes
	OwnerType string // one of the OwnerType* constants
	OwnerID   string
	UserID    string // author; "" once the author's account is deleted
	Body      string // Markdown
	CreatedAt string // RFC 3339
	UpdatedAt string // RFC 3339
}

// Attachment represents a file attached to any entity.
type Attachment struct {
	ID          string
	ProductID   string // owning product; scopes reads and writes
	OwnerType   string // one of the OwnerType* constants
	OwnerID     string
	UserID      string // uploader; "" once the uploader's account is deleted
	Filename    string
	ContentType string
	Size        int64
	StoragePath string // server-generated; never accepted from a client
	CreatedAt   string // RFC 3339
}

// Owner types for comments and attachments (FR-026, FR-027, FR-401).
const (
	OwnerTypeRequirement = "requirement"
	OwnerTypeTask        = "task"
	OwnerTypeConstraint  = "constraint"
	OwnerTypeTestCase    = "test_case"
	OwnerTypeComponent   = "component"
	OwnerTypeCapability  = "capability"
	OwnerTypeMilestone   = "milestone"
	OwnerTypeDefect      = "defect"
	OwnerTypeDocument    = "document"
)

// ValidOwnerType reports whether ownerType can carry comments and attachments.
func ValidOwnerType(ownerType string) bool {
	switch ownerType {
	case OwnerTypeRequirement, OwnerTypeTask, OwnerTypeConstraint, OwnerTypeTestCase,
		OwnerTypeComponent, OwnerTypeCapability, OwnerTypeMilestone, OwnerTypeDefect, OwnerTypeDocument:
		return true
	default:
		return false
	}
}

// AuditEntry represents a single field-level change in the audit trail.
type AuditEntry struct {
	ID         string
	ProductID  string // product the entity belongs to; kept after the product is deleted
	EntityType string // "product", "component", "requirement", "constraint", etc.
	EntityID   string
	UserID     string
	Action     string // "create", "update", "delete"
	FieldName  string // empty for create/delete
	OldValue   string // empty for create
	NewValue   string // empty for delete
	CreatedAt  string // RFC 3339
}

// DeletedProduct is a product the audit trail recorded as deleted, kept for
// administrators until its history is purged.
type DeletedProduct struct {
	ID        string
	Name      string // the name it had when deleted
	DeletedAt string // RFC 3339
	Entries   int    // audit entries still kept for it
}

// DefectLink represents a polymorphic link from a defect
// to a component, capability, test case, or requirement.
type DefectLink struct {
	ID         string
	DefectID   string
	TargetType string // one of the DefectLinkTarget* constants
	TargetID   string
	CreatedAt  string // RFC 3339
}

// Defect link target types (FR-023, FR-501).
const (
	DefectLinkTargetComponent   = "component"
	DefectLinkTargetCapability  = "capability"
	DefectLinkTargetTestCase    = "test_case"
	DefectLinkTargetRequirement = "requirement"
)

// ValidDefectLinkTarget reports whether targetType is a supported defect link target.
func ValidDefectLinkTarget(targetType string) bool {
	switch targetType {
	case DefectLinkTargetComponent, DefectLinkTargetCapability,
		DefectLinkTargetTestCase, DefectLinkTargetRequirement:
		return true
	default:
		return false
	}
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
