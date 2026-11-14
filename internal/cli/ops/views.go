package ops

// Views are the results of operations. Their JSON form is the stable,
// machine-readable output of the CLI (-o json) and of the MCP tools, so field
// names only ever get added, never renamed or removed.

// Ref points at a related entity.
type Ref struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier,omitempty"` // FR-12 / AC-42 where the entity has one
	Title      string `json:"title"`
}

// RequirementSummary is one row of a requirement or task list.
type RequirementSummary struct {
	ID           string `json:"id"`
	Identifier   string `json:"identifier"`
	Kind         string `json:"kind"` // requirement or task
	Title        string `json:"title"`
	Category     string `json:"category"`
	Status       string `json:"status"`
	StatusClosed bool   `json:"status_closed"`
	Priority     int    `json:"priority"`
	Active       bool   `json:"active"`
	Coarse       bool   `json:"coarse"`
	Sprint       string `json:"sprint,omitempty"`
	Estimation   string `json:"estimation,omitempty"`
}

// RequirementDetail is everything an agent needs to work on one item.
type RequirementDetail struct {
	RequirementSummary
	Description      string               `json:"description"`
	DefinitionOfDone string               `json:"definition_of_done,omitempty"`
	Milestone        string               `json:"milestone,omitempty"`
	Parent           *Ref                 `json:"parent,omitempty"`
	Children         []RequirementSummary `json:"children"`
	Assignees        []string             `json:"assignees"`
	Capabilities     []Ref                `json:"capabilities"`
	TestCases        []TestCaseSummary    `json:"test_cases"`
	Defects          []DefectSummary      `json:"defects"`
	// Documents are the documents that define the item (FR-909); without
	// the documents scope, their identifiers only.
	Documents []Ref         `json:"documents"`
	Comments  []CommentView `json:"comments"`
}

// ConstraintSummary is one row of a constraint list.
type ConstraintSummary struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	Category   string `json:"category"`
	Compliance string `json:"compliance"` // not_assessed, met, violated
	Priority   int    `json:"priority"`
}

// ConstraintDetail adds the description, links and discussion.
type ConstraintDetail struct {
	ConstraintSummary
	Description  string        `json:"description"`
	Milestone    string        `json:"milestone,omitempty"`
	Capabilities []Ref         `json:"capabilities"`
	Documents    []Ref         `json:"documents"`
	Comments     []CommentView `json:"comments"`
}

// DefectSummary is one row of a defect list.
type DefectSummary struct {
	ID           string `json:"id"`
	Identifier   string `json:"identifier"` // e.g. DF-17
	Title        string `json:"title"`
	Criticality  string `json:"criticality"`
	Status       string `json:"status"`
	StatusClosed bool   `json:"status_closed"`
	Priority     int    `json:"priority"`
	Active       bool   `json:"active"`
	Environment  string `json:"environment,omitempty"`
}

// DefectDetail adds description, links, assignees and discussion.
type DefectDetail struct {
	DefectSummary
	Description string        `json:"description"`
	Milestone   string        `json:"milestone,omitempty"`
	Sprint      string        `json:"sprint,omitempty"`
	Links       []LinkView    `json:"links"`
	Assignees   []string      `json:"assignees"`
	Comments    []CommentView `json:"comments"`
}

// LinkView is a defect link with a readable label.
type LinkView struct {
	Type  string `json:"type"` // requirement, test_case, component, capability
	ID    string `json:"id"`
	Label string `json:"label"`
}

// TestCaseSummary is one row of a test case list.
type TestCaseSummary struct {
	ID             string `json:"id"`
	ShortID        string `json:"short_id"`
	Title          string `json:"title"`
	Mode           string `json:"mode"` // manual or automatic
	TestIdentifier string `json:"test_identifier,omitempty"`
	Component      string `json:"component,omitempty"`
	Capability     string `json:"capability,omitempty"`
	PyramidLevel   string `json:"pyramid_level,omitempty"`
}

// TestCaseDetail adds description, coverage links, environments and results.
type TestCaseDetail struct {
	TestCaseSummary
	Description  string          `json:"description"`
	Requirements []CoverageRef   `json:"requirements"`
	Environments []string        `json:"environments"`
	Results      []ResultView    `json:"results"`
	Defects      []DefectSummary `json:"defects"`
	Documents    []Ref           `json:"documents"`
	Comments     []CommentView   `json:"comments"`
}

// CoverageRef is a requirement a test case covers, and how.
type CoverageRef struct {
	Ref
	LinkType string `json:"link_type"` // requirement or definition_of_done
}

// ResultView is one campaign result.
type ResultView struct {
	Campaign       string `json:"campaign"`
	CampaignStatus string `json:"campaign_status"`
	TestCase       *Ref   `json:"test_case,omitempty"`
	TestIdentifier string `json:"test_identifier,omitempty"`
	Environment    string `json:"environment,omitempty"`
	Status         string `json:"status"`
	DefectID       string `json:"defect_id,omitempty"`
	Defect         string `json:"defect,omitempty"` // the defect's identifier
	ExecutedAt     string `json:"executed_at,omitempty"`
}

// CommentView is a comment with its author's username.
type CommentView struct {
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	Body      string `json:"body"`
}

// CapabilityView is a capability in the structure tree.
type CapabilityView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ComponentView is a component and its capabilities.
type ComponentView struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	Capabilities []CapabilityView `json:"capabilities"`
}

// Structure is the product's component → capability tree.
type Structure struct {
	Components []ComponentView `json:"components"`
	// ProductLevel lists capabilities that belong to no component.
	ProductLevel []CapabilityView `json:"product_level_capabilities"`
}

// CapabilityDetail is a capability with everything traced to it.
type CapabilityDetail struct {
	CapabilityView
	Component    string               `json:"component,omitempty"`
	Requirements []RequirementSummary `json:"requirements"`
	TestCases    []TestCaseSummary    `json:"test_cases"`
	Defects      []DefectSummary      `json:"defects"`
	Comments     []CommentView        `json:"comments"`
}

// CampaignView is a testing campaign's status.
type CampaignView struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Status   string         `json:"status"` // draft, active, closed
	StartsAt string         `json:"starts_at,omitempty"`
	EndsAt   string         `json:"ends_at,omitempty"`
	Verdict  string         `json:"verdict"` // incomplete, passed, failed
	Total    int            `json:"total"`
	Counts   map[string]int `json:"counts"`
	// Attention lists failed, blocked and re-test results (filled by
	// CampaignStatus, not by Context).
	Attention []ResultView `json:"attention,omitempty"`
}

// CampaignStatusView wraps the campaign, which is nil when none is active.
type CampaignStatusView struct {
	Campaign *CampaignView `json:"campaign"`
	Message  string        `json:"message,omitempty"`
}

// ContextView is the one-shot product briefing for agents.
type ContextView struct {
	Product struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		WorkflowMode   string `json:"workflow_mode"`
		EstimationMode string `json:"estimation_mode"`
	} `json:"product"`
	// Workflow lists the requirement/task status names in board order; these
	// are the values `req status` accepts.
	Workflow       []string             `json:"workflow"`
	Structure      Structure            `json:"structure"`
	Constraints    []ConstraintSummary  `json:"constraints"`
	ActiveSprint   *SprintView          `json:"active_sprint,omitempty"`
	InProgress     []RequirementSummary `json:"in_progress"`
	Backlog        BacklogCounts        `json:"backlog"`
	OpenDefects    []DefectSummary      `json:"open_defects"`
	ActiveCampaign *CampaignView        `json:"active_campaign,omitempty"`
}

// SprintView is a sprint.
type SprintView struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// BacklogCounts summarizes open backlog items.
type BacklogCounts struct {
	OpenRequirements int `json:"open_requirements"`
	OpenTasks        int `json:"open_tasks"`
	Coarse           int `json:"coarse"` // open items that still need refinement
}
