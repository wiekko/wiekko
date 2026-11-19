package client

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// The DTOs below mirror the JSON the /api/v1 product endpoints return. Empty
// strings stand for "not set", as on the server.

// Requirement is a requirement or task (Kind distinguishes them).
type Requirement struct {
	ID               string `json:"id"`
	Identifier       string `json:"identifier"` // e.g. "FR-12"; empty if the category is unknown
	SequenceNumber   int    `json:"sequence_number"`
	ProductID        string `json:"product_id"`
	Kind             string `json:"kind"`
	ParentID         string `json:"parent_id"`
	CategoryID       string `json:"category_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	DefinitionOfDone string `json:"definition_of_done"`
	StatusID         string `json:"status_id"`
	MilestoneID      string `json:"milestone_id"`
	SprintID         string `json:"sprint_id"`
	Estimation       string `json:"estimation"`
	Priority         int    `json:"priority"`
	IsCoarse         bool   `json:"is_coarse"`
	IsActive         bool   `json:"is_active"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	// Documents names the linked documents (FR-141); only in single-item
	// responses, and identifiers only without the documents scope.
	Documents []DocumentRef `json:"documents,omitempty"`
}

// RequirementFilter narrows ListRequirements; empty fields do not filter.
type RequirementFilter struct {
	Kind       string
	CategoryID string
	StatusID   string
	SprintID   string
}

// Constraint is an ongoing rule that is met or violated.
type Constraint struct {
	ID              string `json:"id"`
	Identifier      string `json:"identifier"` // e.g. "AC-42"
	ProductID       string `json:"product_id"`
	CategoryID      string `json:"category_id"`
	SequenceNumber  int    `json:"sequence_number"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	ComplianceState string `json:"compliance_state"`
	MilestoneID     string `json:"milestone_id"`
	Priority        int    `json:"priority"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	// Documents names the linked documents (FR-141); only in single-item
	// responses, and identifiers only without the documents scope.
	Documents []DocumentRef `json:"documents,omitempty"`
}

// Category groups requirements, tasks or constraints and supplies their prefix.
type Category struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Position   int    `json:"position"`
}

// Status is a workflow column for requirements/tasks or defects.
type Status struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	Name       string `json:"name"`
	Position   int    `json:"position"`
	IsDefault  bool   `json:"is_default"`
	IsClosed   bool   `json:"is_closed"`
	IsActive   bool   `json:"is_active"`
}

// Component is a structural part of the product.
type Component struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MilestoneID string `json:"milestone_id"`
}

// Capability is a testable feature area, optionally under a component.
type Capability struct {
	ID          string `json:"id"`
	ComponentID string `json:"component_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MilestoneID string `json:"milestone_id"`
}

// Defect is a bug or issue.
type Defect struct {
	ID             string `json:"id"`
	ProductID      string `json:"product_id"`
	Identifier     string `json:"identifier"` // e.g. DF-17
	SequenceNumber int    `json:"sequence_number"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Criticality    string `json:"criticality"`
	StatusID       string `json:"status_id"`
	MilestoneID    string `json:"milestone_id"`
	SprintID       string `json:"sprint_id"`
	EnvironmentID  string `json:"environment_id"`
	Estimation     string `json:"estimation"`
	Priority       int    `json:"priority"`
	IsActive       bool   `json:"is_active"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// DefectLink links a defect to a component, capability, test case or requirement.
type DefectLink struct {
	ID         string `json:"id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
}

// TestCase is a manual or automatic test.
type TestCase struct {
	ID             string `json:"id"`
	ComponentID    string `json:"component_id"`
	CapabilityID   string `json:"capability_id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	ExecutionMode  string `json:"execution_mode"`
	TestIdentifier string `json:"test_identifier"`
	PyramidLevelID string `json:"pyramid_level_id"`
	// Documents names the linked documents (FR-141); only in single-item
	// responses, and identifiers only without the documents scope.
	Documents []DocumentRef `json:"documents,omitempty"`
}

// TestCaseRequirement links a test case to a requirement or its definition of done.
type TestCaseRequirement struct {
	RequirementID string `json:"requirement_id"`
	LinkType      string `json:"link_type"`
}

// Campaign is a testing campaign.
type Campaign struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // draft, active, closed
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	ClosedAt    string `json:"closed_at"`
}

// CampaignProgress holds result counts by status and the derived verdict.
type CampaignProgress struct {
	Counts  map[string]int `json:"counts"`
	Verdict string         `json:"verdict"` // incomplete, passed, failed
}

// CampaignResult is one test case's status in a campaign (per environment).
type CampaignResult struct {
	ID         string `json:"id"`
	CampaignID string `json:"campaign_id"`
	// TestCaseID is empty once the test case has been deleted; TestTitle is
	// the title the campaign copied, and stays.
	TestCaseID      string `json:"test_case_id"`
	TestTitle       string `json:"test_title"`
	EnvironmentID   string `json:"environment_id"`
	EnvironmentName string `json:"environment_name"`
	Status          string `json:"status"`
	DefectID        string `json:"defect_id"`
	ExecutedAt      string `json:"executed_at"`
}

// Comment is a Markdown comment on any entity.
type Comment struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// CapabilityLink is one end of a requirement–capability or
// constraint–capability link.
type CapabilityLink struct {
	CapabilityID string `json:"capability_id"`
}

// RequirementLink is the requirement end of a capability–requirement link.
type RequirementLink struct {
	RequirementID string `json:"requirement_id"`
}

// Member is a user's role in a product.
type Member struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// Assignment is a user assigned to a requirement, task or defect.
type Assignment struct {
	UserID string `json:"user_id"`
}

// Sprint is a time box of backlog work.
type Sprint struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // planned, active, completed
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// Milestone is a roadmap checkpoint.
type Milestone struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	DueDate string `json:"due_date"`
}

// PyramidLevel is a test pyramid level.
type PyramidLevel struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
}

// RuntimeEnvironment is a label such as an OS or browser.
type RuntimeEnvironment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// productPath builds /products/{productID}/{parts...}, escaping each segment.
func productPath(productID string, parts ...string) string {
	var b strings.Builder
	b.WriteString("/products/")
	b.WriteString(url.PathEscape(productID))
	for _, p := range parts {
		b.WriteByte('/')
		b.WriteString(url.PathEscape(p))
	}
	return b.String()
}

func withQuery(path string, q url.Values) string {
	for k, v := range q {
		if len(v) == 0 || v[0] == "" {
			q.Del(k)
		}
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

func getList[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var out []T
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func getOne[T any](ctx context.Context, c *Client, path string) (*T, error) {
	var out T
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Backlog ---

// Requirements lists requirements and tasks.
func (c *Client) Requirements(ctx context.Context, productID string, f RequirementFilter) ([]Requirement, error) {
	q := url.Values{"kind": {f.Kind}, "category_id": {f.CategoryID}, "status_id": {f.StatusID}, "sprint_id": {f.SprintID}}
	return getList[Requirement](ctx, c, withQuery(productPath(productID, "requirements"), q))
}

// Requirement returns a requirement or task by ID or identifier ("FR-12").
func (c *Client) Requirement(ctx context.Context, productID, ref string) (*Requirement, error) {
	return getOne[Requirement](ctx, c, productPath(productID, "requirements", ref))
}

// RequirementChildren lists the direct children of a requirement or task.
func (c *Client) RequirementChildren(ctx context.Context, productID, ref string) ([]Requirement, error) {
	return getList[Requirement](ctx, c, productPath(productID, "requirements", ref, "children"))
}

// RequirementAssignees lists the users assigned to a requirement or task.
func (c *Client) RequirementAssignees(ctx context.Context, productID, ref string) ([]Assignment, error) {
	return getList[Assignment](ctx, c, productPath(productID, "requirements", ref, "assignees"))
}

// RequirementCapabilityLinks lists the capability links of a requirement.
func (c *Client) RequirementCapabilityLinks(ctx context.Context, productID, ref string) ([]CapabilityLink, error) {
	return getList[CapabilityLink](ctx, c, productPath(productID, "requirements", ref, "capabilities"))
}

// RequirementTestCases lists the test cases linked to a requirement.
func (c *Client) RequirementTestCases(ctx context.Context, productID, ref string) ([]TestCase, error) {
	return getList[TestCase](ctx, c, productPath(productID, "requirements", ref, "test-cases"))
}

// RequirementDefects lists the defects linked to a requirement.
func (c *Client) RequirementDefects(ctx context.Context, productID, ref string) ([]Defect, error) {
	return getList[Defect](ctx, c, productPath(productID, "requirements", ref, "defects"))
}

// Constraints lists constraints.
func (c *Client) Constraints(ctx context.Context, productID string) ([]Constraint, error) {
	return getList[Constraint](ctx, c, productPath(productID, "constraints"))
}

// Constraint returns a constraint by ID or identifier ("AC-42").
func (c *Client) Constraint(ctx context.Context, productID, ref string) (*Constraint, error) {
	return getOne[Constraint](ctx, c, productPath(productID, "constraints", ref))
}

// ConstraintCapabilityLinks lists the capability links of a constraint.
func (c *Client) ConstraintCapabilityLinks(ctx context.Context, productID, ref string) ([]CapabilityLink, error) {
	return getList[CapabilityLink](ctx, c, productPath(productID, "constraints", ref, "capabilities"))
}

// Defects lists defects.
func (c *Client) Defects(ctx context.Context, productID string) ([]Defect, error) {
	return getList[Defect](ctx, c, productPath(productID, "defects"))
}

// Defect returns a defect by ID.
func (c *Client) Defect(ctx context.Context, productID, id string) (*Defect, error) {
	return getOne[Defect](ctx, c, productPath(productID, "defects", id))
}

// DefectLinks lists what a defect is linked to.
func (c *Client) DefectLinks(ctx context.Context, productID, id string) ([]DefectLink, error) {
	return getList[DefectLink](ctx, c, productPath(productID, "defects", id, "links"))
}

// DefectAssignees lists the users assigned to a defect.
func (c *Client) DefectAssignees(ctx context.Context, productID, id string) ([]Assignment, error) {
	return getList[Assignment](ctx, c, productPath(productID, "defects", id, "assignees"))
}

// --- Product structure ---

// Components lists components.
func (c *Client) Components(ctx context.Context, productID string) ([]Component, error) {
	return getList[Component](ctx, c, productPath(productID, "components"))
}

// Capabilities lists capabilities.
func (c *Client) Capabilities(ctx context.Context, productID string) ([]Capability, error) {
	return getList[Capability](ctx, c, productPath(productID, "capabilities"))
}

// Capability returns a capability by ID.
func (c *Client) Capability(ctx context.Context, productID, id string) (*Capability, error) {
	return getOne[Capability](ctx, c, productPath(productID, "capabilities", id))
}

// CapabilityRequirementLinks lists the requirements linked to a capability.
func (c *Client) CapabilityRequirementLinks(ctx context.Context, productID, id string) ([]RequirementLink, error) {
	return getList[RequirementLink](ctx, c, productPath(productID, "capabilities", id, "requirements"))
}

// CapabilityTestCases lists the test cases of a capability.
func (c *Client) CapabilityTestCases(ctx context.Context, productID, id string) ([]TestCase, error) {
	return getList[TestCase](ctx, c, productPath(productID, "capabilities", id, "test-cases"))
}

// CapabilityDefects lists the defects linked to a capability.
func (c *Client) CapabilityDefects(ctx context.Context, productID, id string) ([]Defect, error) {
	return getList[Defect](ctx, c, productPath(productID, "capabilities", id, "defects"))
}

// CapabilityConstraints lists the constraints linked to a capability.
func (c *Client) CapabilityConstraints(ctx context.Context, productID, id string) ([]Constraint, error) {
	return getList[Constraint](ctx, c, productPath(productID, "capabilities", id, "constraints"))
}

// Categories lists the categories of one entity type (requirement, task, constraint).
func (c *Client) Categories(ctx context.Context, productID, entityType string) ([]Category, error) {
	return getList[Category](ctx, c, withQuery(productPath(productID, "categories"), url.Values{"entity_type": {entityType}}))
}

// Statuses lists the statuses of one entity type (requirement, defect).
func (c *Client) Statuses(ctx context.Context, productID, entityType string) ([]Status, error) {
	return getList[Status](ctx, c, withQuery(productPath(productID, "statuses"), url.Values{"entity_type": {entityType}}))
}

// Members lists the product's members.
func (c *Client) Members(ctx context.Context, productID string) ([]Member, error) {
	return getList[Member](ctx, c, productPath(productID, "members"))
}

// Sprints lists sprints.
func (c *Client) Sprints(ctx context.Context, productID string) ([]Sprint, error) {
	return getList[Sprint](ctx, c, productPath(productID, "sprints"))
}

// Milestones lists milestones.
func (c *Client) Milestones(ctx context.Context, productID string) ([]Milestone, error) {
	return getList[Milestone](ctx, c, productPath(productID, "milestones"))
}

// PyramidLevels lists test pyramid levels.
func (c *Client) PyramidLevels(ctx context.Context, productID string) ([]PyramidLevel, error) {
	return getList[PyramidLevel](ctx, c, productPath(productID, "pyramid-levels"))
}

// RuntimeEnvironments lists runtime environments.
func (c *Client) RuntimeEnvironments(ctx context.Context, productID string) ([]RuntimeEnvironment, error) {
	return getList[RuntimeEnvironment](ctx, c, productPath(productID, "runtime-environments"))
}

// Comments lists the comments on an entity (ownerType: requirement, task,
// constraint, test_case, component, capability, milestone, defect).
func (c *Client) Comments(ctx context.Context, productID, ownerType, ownerID string) ([]Comment, error) {
	return getList[Comment](ctx, c, withQuery(productPath(productID, "comments"), url.Values{"owner_type": {ownerType}, "owner_id": {ownerID}}))
}

// --- Testing ---

// TestCases lists test cases; testIdentifier, when set, matches exactly.
func (c *Client) TestCases(ctx context.Context, productID, testIdentifier string) ([]TestCase, error) {
	return getList[TestCase](ctx, c, withQuery(productPath(productID, "test-cases"), url.Values{"test_identifier": {testIdentifier}}))
}

// TestCase returns a test case by ID.
func (c *Client) TestCase(ctx context.Context, productID, id string) (*TestCase, error) {
	return getOne[TestCase](ctx, c, productPath(productID, "test-cases", id))
}

// TestCaseRequirements lists the requirement links of a test case.
func (c *Client) TestCaseRequirements(ctx context.Context, productID, id string) ([]TestCaseRequirement, error) {
	return getList[TestCaseRequirement](ctx, c, productPath(productID, "test-cases", id, "requirements"))
}

// TestCaseEnvironments lists the runtime environment IDs of a test case.
func (c *Client) TestCaseEnvironments(ctx context.Context, productID, id string) ([]string, error) {
	return getList[string](ctx, c, productPath(productID, "test-cases", id, "environments"))
}

// TestCaseResults lists a test case's results across campaigns.
func (c *Client) TestCaseResults(ctx context.Context, productID, id string) ([]CampaignResult, error) {
	return getList[CampaignResult](ctx, c, productPath(productID, "test-cases", id, "results"))
}

// TestCaseDefects lists the defects linked to a test case.
func (c *Client) TestCaseDefects(ctx context.Context, productID, id string) ([]Defect, error) {
	return getList[Defect](ctx, c, productPath(productID, "test-cases", id, "defects"))
}

// Campaigns lists testing campaigns.
func (c *Client) Campaigns(ctx context.Context, productID string) ([]Campaign, error) {
	return getList[Campaign](ctx, c, productPath(productID, "campaigns"))
}

// Campaign returns a campaign by ID.
func (c *Client) Campaign(ctx context.Context, productID, id string) (*Campaign, error) {
	return getOne[Campaign](ctx, c, productPath(productID, "campaigns", id))
}

// CampaignProgress returns a campaign's result counts and verdict.
func (c *Client) CampaignProgress(ctx context.Context, productID, id string) (*CampaignProgress, error) {
	return getOne[CampaignProgress](ctx, c, productPath(productID, "campaigns", id, "progress"))
}

// CampaignResults lists a campaign's results.
func (c *Client) CampaignResults(ctx context.Context, productID, id string) ([]CampaignResult, error) {
	return getList[CampaignResult](ctx, c, productPath(productID, "campaigns", id, "results"))
}

// Specification returns the product's specification as Markdown (FR-137),
// with tasks if asked for.
func (c *Client) Specification(ctx context.Context, productID string, tasks, drafts bool) (string, error) {
	q := url.Values{}
	if tasks {
		q.Set("tasks", "true")
	}
	if drafts {
		q.Set("drafts", "true")
	}
	path := withQuery(productPath(productID, "specification"), q)
	resp, err := c.Raw(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", parseAPIError(resp)
	}
	return string(resp.Body), nil
}
