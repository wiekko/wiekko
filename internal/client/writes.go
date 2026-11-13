package client

import (
	"context"
	"net/http"
)

// Write calls. POST requests are never retried (see isIdempotent), so a
// create is sent at most once even when the connection drops.

func send[T any](ctx context.Context, c *Client, method, path string, body any) (*T, error) {
	var out T
	if err := c.Do(ctx, method, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RequirementCreate is the payload of CreateRequirement. IDs, not names:
// resolving names is the caller's job.
type RequirementCreate struct {
	Kind             string `json:"kind,omitempty"`
	ParentID         string `json:"parent_id,omitempty"`
	CategoryID       string `json:"category_id,omitempty"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	DefinitionOfDone string `json:"definition_of_done,omitempty"`
	StatusID         string `json:"status_id,omitempty"`
}

// CreateRequirement creates a requirement or task (owner role).
func (c *Client) CreateRequirement(ctx context.Context, productID string, in RequirementCreate) (*Requirement, error) {
	return send[Requirement](ctx, c, http.MethodPost, productPath(productID, "requirements"), in)
}

// RequirementPatch is a partial update; nil fields are left unchanged.
type RequirementPatch struct {
	StatusID *string `json:"status_id,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

// UpdateRequirement patches a requirement or task by identifier or ID.
// Developers may change status and activation only (FR-407).
func (c *Client) UpdateRequirement(ctx context.Context, productID, ref string, p RequirementPatch) (*Requirement, error) {
	return send[Requirement](ctx, c, http.MethodPatch, productPath(productID, "requirements", ref), p)
}

// DefectCreate is the payload of CreateDefect.
type DefectCreate struct {
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Criticality   string `json:"criticality,omitempty"`
	StatusID      string `json:"status_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
}

// CreateDefect creates a defect (tester role).
func (c *Client) CreateDefect(ctx context.Context, productID string, in DefectCreate) (*Defect, error) {
	return send[Defect](ctx, c, http.MethodPost, productPath(productID, "defects"), in)
}

// DefectPatch is a partial update; nil fields are left unchanged.
type DefectPatch struct {
	StatusID *string `json:"status_id,omitempty"`
}

// UpdateDefect patches a defect. Developers may change its status.
func (c *Client) UpdateDefect(ctx context.Context, productID, id string, p DefectPatch) (*Defect, error) {
	return send[Defect](ctx, c, http.MethodPatch, productPath(productID, "defects", id), p)
}

// AddDefectLink links a defect to a requirement, task, test case, component
// or capability (targetType: requirement, test_case, component, capability).
func (c *Client) AddDefectLink(ctx context.Context, productID, defectID, targetType, targetID string) error {
	body := map[string]string{"target_type": targetType, "target_id": targetID}
	return c.Do(ctx, http.MethodPost, productPath(productID, "defects", defectID, "links"), body, nil)
}

// CreateComment adds a Markdown comment to an entity. ownerType is one of
// requirement, task, constraint, test_case, component, capability, milestone,
// defect.
func (c *Client) CreateComment(ctx context.Context, productID, ownerType, ownerID, body string) (*Comment, error) {
	in := map[string]string{"owner_type": ownerType, "owner_id": ownerID, "body": body}
	return send[Comment](ctx, c, http.MethodPost, productPath(productID, "comments"), in)
}

// TestRun is an automated test run upload (POST /products/{p}/test-runs).
type TestRun struct {
	Source      string          `json:"source"`
	CampaignID  string          `json:"campaign_id,omitempty"`
	Environment string          `json:"environment,omitempty"`
	Revision    string          `json:"revision,omitempty"`
	Branch      string          `json:"branch,omitempty"`
	DryRun      bool            `json:"dry_run,omitempty"`
	Results     []TestRunResult `json:"results"`
}

// TestRunResult is one reported test.
type TestRunResult struct {
	TestIdentifier string `json:"test_identifier"`
	Status         string `json:"status"`
	Output         string `json:"output,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
}

// TestRunReport is the server's account of an uploaded run.
type TestRunReport struct {
	Recorded    bool             `json:"recorded"`
	DryRun      bool             `json:"dry_run"`
	Campaign    *NamedRef        `json:"campaign"`
	Environment string           `json:"environment"`
	Summary     map[string]int   `json:"summary"`
	Results     []TestRunOutcome `json:"results"`
}

// NamedRef is an entity's ID and name.
type NamedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TestRunOutcome is what happened to one test case of a run.
type TestRunOutcome struct {
	TestIdentifier string `json:"test_identifier"`
	Reported       int    `json:"reported"`
	Status         string `json:"status"`
	Outcome        string `json:"outcome"`
	TestCaseID     string `json:"test_case_id,omitempty"`
	ResultID       string `json:"result_id,omitempty"`
	PreviousStatus string `json:"previous_status,omitempty"`
	DefectID       string `json:"defect_id,omitempty"`
	// DefectIdentifier is the defect's identifier, e.g. DF-17.
	DefectIdentifier string `json:"defect_identifier,omitempty"`
	Defect           string `json:"defect,omitempty"`
}

// IngestTestRun uploads a test run (tester role). With no active campaign
// the server records nothing and says so in Recorded.
func (c *Client) IngestTestRun(ctx context.Context, productID string, run TestRun) (*TestRunReport, error) {
	return send[TestRunReport](ctx, c, http.MethodPost, productPath(productID, "test-runs"), run)
}

// TestCaseCreate is the payload of CreateTestCase.
type TestCaseCreate struct {
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	ExecutionMode  string `json:"execution_mode"`
	TestIdentifier string `json:"test_identifier,omitempty"`
	CapabilityID   string `json:"capability_id,omitempty"`
	ComponentID    string `json:"component_id,omitempty"`
}

// CreateTestCase creates a test case (developer role).
func (c *Client) CreateTestCase(ctx context.Context, productID string, in TestCaseCreate) (*TestCase, error) {
	return send[TestCase](ctx, c, http.MethodPost, productPath(productID, "test-cases"), in)
}
