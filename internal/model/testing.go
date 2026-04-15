package model

// TestCase represents a test case within a product.
type TestCase struct {
	ID             string
	ProductID      string
	ComponentID    string // empty if not linked
	CapabilityID   string // empty if not linked
	Title          string
	Description    string // Markdown
	ExecutionMode  string // "manual", "automatic"
	TestIdentifier string // code test function name for automatic tests
	PyramidLevelID string // empty if not assigned
	CreatedAt      string // RFC 3339
	UpdatedAt      string // RFC 3339
}

// Execution mode constants.
const (
	ExecutionModeManual    = "manual"
	ExecutionModeAutomatic = "automatic"
)

// RuntimeEnvironment represents a freely definable label
// for runtime environments within a product.
type RuntimeEnvironment struct {
	ID        string
	ProductID string
	Name      string
	CreatedAt string // RFC 3339
}

// TestingCampaign represents a test execution campaign.
type TestingCampaign struct {
	ID        string
	ProductID string
	Name      string
	CreatedAt string // RFC 3339
	UpdatedAt string // RFC 3339
}

// CampaignResult represents the status of a test case
// within a campaign, optionally per runtime environment.
type CampaignResult struct {
	ID            string
	CampaignID    string
	TestCaseID    string
	EnvironmentID string // empty if no environment matrix
	Status        string // "pending", "passed", "failed"
	UpdatedAt     string // RFC 3339
}

// Campaign result status constants.
const (
	CampaignResultStatusPending = "pending"
	CampaignResultStatusPassed  = "passed"
	CampaignResultStatusFailed  = "failed"
)

// PyramidLevel represents a test pyramid categorization level.
type PyramidLevel struct {
	ID        string
	ProductID string
	ParentID  string // empty if top-level; max depth 2
	Name      string
	Position  int
	CreatedAt string // RFC 3339
}
