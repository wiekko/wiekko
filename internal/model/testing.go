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
	ID          string
	ProductID   string
	Name        string
	Description string // Markdown
	Status      string // one of the CampaignStatus* constants
	StartsAt    string // empty if unplanned; RFC 3339
	EndsAt      string // empty if unplanned; RFC 3339
	ClosedAt    string // empty until closed; RFC 3339
	ClosedBy    string // user who closed the campaign; empty until closed
	CreatedAt   string // RFC 3339
	UpdatedAt   string // RFC 3339
}

// Campaign status constants (FR-320).
const (
	CampaignStatusDraft  = "draft"
	CampaignStatusActive = "active"
	CampaignStatusClosed = "closed"
)

// ValidCampaignStatus reports whether status is a supported campaign status.
func ValidCampaignStatus(status string) bool {
	switch status {
	case CampaignStatusDraft, CampaignStatusActive, CampaignStatusClosed:
		return true
	default:
		return false
	}
}

// CampaignResult represents the status of a test case
// within a campaign, optionally per runtime environment.
type CampaignResult struct {
	ID            string
	CampaignID    string
	TestCaseID    string
	EnvironmentID string // empty if no environment matrix
	Status        string // one of the CampaignResultStatus* constants
	DefectID      string // defect explaining a failure (FR-502); empty otherwise
	ExecutedBy    string // user who last set the status; empty while pending
	ExecutedAt    string // empty while pending; RFC 3339
	UpdatedAt     string // RFC 3339
}

// Campaign result status constants (FR-021).
const (
	CampaignResultStatusPending = "pending"
	CampaignResultStatusPassed  = "passed"
	CampaignResultStatusFailed  = "failed"
	CampaignResultStatusBlocked = "blocked"
	CampaignResultStatusSkipped = "skipped"
	CampaignResultStatusRetest  = "retest"
)

// ValidCampaignResultStatus reports whether status is a supported result status.
func ValidCampaignResultStatus(status string) bool {
	switch status {
	case CampaignResultStatusPending, CampaignResultStatusPassed, CampaignResultStatusFailed,
		CampaignResultStatusBlocked, CampaignResultStatusSkipped, CampaignResultStatusRetest:
		return true
	default:
		return false
	}
}

// IsIncompleteResultStatus reports whether a result still awaits testing and
// therefore leaves its campaign incomplete (FR-041).
func IsIncompleteResultStatus(status string) bool {
	switch status {
	case CampaignResultStatusPending, CampaignResultStatusBlocked, CampaignResultStatusRetest:
		return true
	default:
		return false
	}
}

// Campaign verdict constants, derived from result counts and never stored (FR-041).
const (
	CampaignVerdictIncomplete = "incomplete"
	CampaignVerdictPassed     = "passed"
	CampaignVerdictFailed     = "failed"
)

// CampaignVerdict derives the outcome of a campaign from its result counts,
// keyed by result status. Skipped results are ignored: a campaign passes when
// every non-skipped result passed, and fails as soon as one failed (FR-041).
func CampaignVerdict(counts map[string]int) string {
	for status, n := range counts {
		if n > 0 && IsIncompleteResultStatus(status) {
			return CampaignVerdictIncomplete
		}
	}
	if counts[CampaignResultStatusFailed] > 0 {
		return CampaignVerdictFailed
	}
	return CampaignVerdictPassed
}

// CampaignResultEvent records one status change of a campaign result. Events are
// immutable and drive campaign progress over time (FR-042, UC-207).
type CampaignResultEvent struct {
	ID         string
	ResultID   string
	CampaignID string
	UserID     string // empty if the acting user was deleted
	FromStatus string // empty for the seeding event
	ToStatus   string
	DefectID   string // defect linked at the time of the change; empty otherwise
	// Source, Revision and Branch describe the automated run that made the
	// change (FR-409); all empty for changes made by a person.
	Source    string // e.g. "go-test", "junit"
	Revision  string // VCS revision the run tested
	Branch    string // VCS branch the run tested
	CreatedAt string // RFC 3339
}

// PyramidLevel represents a test pyramid categorization level.
type PyramidLevel struct {
	ID        string
	ProductID string
	ParentID  string // empty if top-level; max depth 2
	Name      string
	Position  int
	CreatedAt string // RFC 3339
}
