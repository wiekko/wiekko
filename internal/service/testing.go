package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// TestingService handles test cases, campaigns, and campaign results.
type TestingService struct {
	testCases       store.TestCaseStore
	campaigns       store.TestingCampaignStore
	campaignResults store.CampaignResultStore
	campaignEvents  store.CampaignResultEventStore
	comments        ownerPurger
	audit           store.AuditStore
	authz           *Authorizer
}

// --- Test Cases ---

// CreateTestCaseInput contains the fields required to create a test case.
type CreateTestCaseInput struct {
	ProductID      string
	ComponentID    string
	CapabilityID   string
	Title          string
	Description    string
	ExecutionMode  string
	TestIdentifier string
	PyramidLevelID string
}

// UpdateTestCaseInput contains the fields that can be updated on a test case.
type UpdateTestCaseInput struct {
	ComponentID    *string
	CapabilityID   *string
	Title          *string
	Description    *string
	ExecutionMode  *string
	TestIdentifier *string
	PyramidLevelID *string
}

// CreateTestCase creates a new test case. Requires developer role.
func (s *TestingService) CreateTestCase(ctx context.Context, user *model.User, input CreateTestCaseInput) (*model.TestCase, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleDeveloper); err != nil {
		return nil, err
	}

	if input.Title == "" {
		return nil, errors.New("test case title is required")
	}
	if input.ExecutionMode == "" {
		input.ExecutionMode = model.ExecutionModeManual
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tc := &model.TestCase{
		ID:             uuid.New().String(),
		ProductID:      input.ProductID,
		ComponentID:    input.ComponentID,
		CapabilityID:   input.CapabilityID,
		Title:          input.Title,
		Description:    input.Description,
		ExecutionMode:  input.ExecutionMode,
		TestIdentifier: input.TestIdentifier,
		PyramidLevelID: input.PyramidLevelID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.testCases.Create(ctx, tc); err != nil {
		return nil, fmt.Errorf("create test case: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "test_case", EntityID: tc.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return tc, nil
}

// GetTestCase returns a test case by ID. Requires viewer role.
func (s *TestingService) GetTestCase(ctx context.Context, user *model.User, productID, id string) (*model.TestCase, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.testCases.GetByID(ctx, id)
}

// UpdateTestCase modifies a test case. Requires developer role.
func (s *TestingService) UpdateTestCase(ctx context.Context, user *model.User, productID, id string, input UpdateTestCaseInput) (*model.TestCase, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return nil, err
	}

	tc, err := s.testCases.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if input.ComponentID != nil {
		tc.ComponentID = *input.ComponentID
	}
	if input.CapabilityID != nil {
		tc.CapabilityID = *input.CapabilityID
	}
	if input.Title != nil {
		tc.Title = *input.Title
	}
	if input.Description != nil {
		tc.Description = *input.Description
	}
	if input.ExecutionMode != nil {
		tc.ExecutionMode = *input.ExecutionMode
	}
	if input.TestIdentifier != nil {
		tc.TestIdentifier = *input.TestIdentifier
	}
	if input.PyramidLevelID != nil {
		tc.PyramidLevelID = *input.PyramidLevelID
	}
	tc.UpdatedAt = now

	if err := s.testCases.Update(ctx, tc); err != nil {
		return nil, fmt.Errorf("update test case: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "test_case", EntityID: tc.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return tc, nil
}

// DeleteTestCase removes a test case. Requires developer role.
func (s *TestingService) DeleteTestCase(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.testCases.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete test case: %w", err)
	}

	// Comments and attachments hang off polymorphic owner columns, so no
	// foreign key cascades them away with the row above.
	if err := s.comments.PurgeOwner(ctx, productID, model.OwnerTypeTestCase, id); err != nil {
		return err
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "test_case", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListTestCases returns all test cases for a product. Requires viewer role.
func (s *TestingService) ListTestCases(ctx context.Context, user *model.User, productID string) ([]*model.TestCase, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.testCases.ListByProductID(ctx, productID)
}

// LinkRequirement links a test case to a requirement. Requires developer role.
func (s *TestingService) LinkRequirement(ctx context.Context, user *model.User, productID, tcID, reqID, linkType string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return s.testCases.LinkRequirement(ctx, &model.TestCaseRequirement{
		ID:            uuid.New().String(),
		TestCaseID:    tcID,
		RequirementID: reqID,
		LinkType:      linkType,
		CreatedAt:     now,
	})
}

// UnlinkRequirement removes a link between a test case and a requirement. Requires developer role.
func (s *TestingService) UnlinkRequirement(ctx context.Context, user *model.User, productID, tcID, reqID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}
	return s.testCases.UnlinkRequirement(ctx, tcID, reqID)
}

// ListTestCaseRequirements returns requirement links for a test case. Requires viewer role.
func (s *TestingService) ListTestCaseRequirements(ctx context.Context, user *model.User, productID, tcID string) ([]*model.TestCaseRequirement, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.testCases.ListRequirementLinks(ctx, tcID)
}

// ListTestCasesByCapability returns test cases whose CapabilityID FK
// references the given capability. Requires viewer role.
func (s *TestingService) ListTestCasesByCapability(ctx context.Context, user *model.User, productID, capabilityID string) ([]*model.TestCase, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.testCases.ListByCapabilityID(ctx, capabilityID)
}

// ListTestCasesByComponent returns test cases whose ComponentID FK
// references the given component. Requires viewer role.
func (s *TestingService) ListTestCasesByComponent(ctx context.Context, user *model.User, productID, componentID string) ([]*model.TestCase, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.testCases.ListByComponentID(ctx, componentID)
}

// ListLinkedTestCasesForRequirement returns the full TestCase records linked
// to a requirement. Requires viewer role.
func (s *TestingService) ListLinkedTestCasesForRequirement(ctx context.Context, user *model.User, productID, requirementID string) ([]*model.TestCase, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	links, err := s.testCases.ListTestCaseLinksByRequirement(ctx, requirementID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.TestCase, 0, len(links))
	for _, link := range links {
		tc, err := s.testCases.GetByID(ctx, link.TestCaseID)
		if err != nil {
			continue
		}
		if tc.ProductID != productID {
			continue
		}
		out = append(out, tc)
	}
	return out, nil
}

// AddTestCaseEnvironment links a test case to a runtime environment. Requires developer role.
func (s *TestingService) AddTestCaseEnvironment(ctx context.Context, user *model.User, productID, tcID, envID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}
	return s.testCases.AddEnvironment(ctx, uuid.New().String(), tcID, envID)
}

// RemoveTestCaseEnvironment removes a test case environment link. Requires developer role.
func (s *TestingService) RemoveTestCaseEnvironment(ctx context.Context, user *model.User, productID, tcID, envID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}
	return s.testCases.RemoveEnvironment(ctx, tcID, envID)
}

// ListTestCaseEnvironments returns environment IDs for a test case. Requires viewer role.
func (s *TestingService) ListTestCaseEnvironments(ctx context.Context, user *model.User, productID, tcID string) ([]string, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.testCases.ListEnvironments(ctx, tcID)
}

// --- Campaigns ---

// CreateCampaignInput contains the fields required to create a campaign.
type CreateCampaignInput struct {
	ProductID   string
	Name        string
	Description string
	StartsAt    string
	EndsAt      string
	// TestCaseIDs is the campaign scope, selected explicitly by the product
	// owner and snapshotted at creation (FR-321).
	TestCaseIDs []string
}

// UpdateCampaignInput contains the fields that can be updated on a campaign.
type UpdateCampaignInput struct {
	Name        *string
	Description *string
	StartsAt    *string
	EndsAt      *string
}

// IncompleteResultsError reports that a campaign still has results awaiting
// testing. Closing is allowed anyway, but only with an explicit confirmation
// from the product owner (FR-333).
type IncompleteResultsError struct {
	Incomplete int
}

func (e *IncompleteResultsError) Error() string {
	return fmt.Sprintf("campaign has %d result(s) still awaiting testing", e.Incomplete)
}

// CreateCampaign creates a new testing campaign and seeds its result matrix
// from the selected test cases. Requires owner role.
func (s *TestingService) CreateCampaign(ctx context.Context, user *model.User, input CreateCampaignInput) (*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("campaign name is required")
	}
	if len(input.TestCaseIDs) == 0 {
		return nil, errors.New("a campaign must contain at least one test case")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.TestingCampaign{
		ID:          uuid.New().String(),
		ProductID:   input.ProductID,
		Name:        input.Name,
		Description: input.Description,
		Status:      model.CampaignStatusDraft,
		StartsAt:    input.StartsAt,
		EndsAt:      input.EndsAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	results, err := s.seedResults(ctx, c.ID, input.ProductID, input.TestCaseIDs, now)
	if err != nil {
		return nil, err
	}

	if err := s.campaigns.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create campaign: %w", err)
	}
	if err := s.campaignResults.CreateBatch(ctx, results); err != nil {
		return nil, fmt.Errorf("seed campaign results: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "testing_campaign", EntityID: c.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return c, nil
}

// seedResults builds the pending result rows for a set of test cases: one row
// per linked runtime environment, or a single environment-less row when the
// test case defines no matrix (FR-021).
func (s *TestingService) seedResults(ctx context.Context, campaignID, productID string, testCaseIDs []string, now string) ([]*model.CampaignResult, error) {
	var results []*model.CampaignResult
	for _, tcID := range testCaseIDs {
		tc, err := s.testCases.GetByID(ctx, tcID)
		if err != nil {
			return nil, fmt.Errorf("test case %s: %w", tcID, err)
		}
		if tc.ProductID != productID {
			return nil, fmt.Errorf("%w: test case %s belongs to another product", ErrForbidden, tcID)
		}

		envIDs, err := s.testCases.ListEnvironments(ctx, tcID)
		if err != nil {
			return nil, fmt.Errorf("list environments for test case %s: %w", tcID, err)
		}
		if len(envIDs) == 0 {
			envIDs = []string{""}
		}
		for _, envID := range envIDs {
			results = append(results, &model.CampaignResult{
				ID:            uuid.New().String(),
				CampaignID:    campaignID,
				TestCaseID:    tcID,
				EnvironmentID: envID,
				Status:        model.CampaignResultStatusPending,
				UpdatedAt:     now,
			})
		}
	}
	return results, nil
}

// AddCampaignTestCases extends a draft campaign's scope. Requires owner role.
func (s *TestingService) AddCampaignTestCases(ctx context.Context, user *model.User, productID, campaignID string, testCaseIDs []string) error {
	c, err := s.requireDraftCampaign(ctx, user, productID, campaignID)
	if err != nil {
		return err
	}

	existing, err := s.campaignResults.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return err
	}
	inScope := make(map[string]bool, len(existing))
	for _, r := range existing {
		inScope[r.TestCaseID] = true
	}

	var toAdd []string
	for _, id := range testCaseIDs {
		if !inScope[id] {
			toAdd = append(toAdd, id)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	results, err := s.seedResults(ctx, c.ID, productID, toAdd, now)
	if err != nil {
		return err
	}
	return s.campaignResults.CreateBatch(ctx, results)
}

// RemoveCampaignTestCase drops a test case from a draft campaign's scope.
// Requires owner role.
func (s *TestingService) RemoveCampaignTestCase(ctx context.Context, user *model.User, productID, campaignID, testCaseID string) error {
	if _, err := s.requireDraftCampaign(ctx, user, productID, campaignID); err != nil {
		return err
	}
	return s.campaignResults.DeleteByCampaignAndTestCase(ctx, campaignID, testCaseID)
}

// requireDraftCampaign loads a campaign and rejects scope changes once it has
// left draft state (FR-321).
func (s *TestingService) requireDraftCampaign(ctx context.Context, user *model.User, productID, campaignID string) (*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if c.ProductID != productID {
		return nil, ErrNotFound
	}
	if c.Status != model.CampaignStatusDraft {
		return nil, errors.New("campaign scope can only be changed while it is a draft")
	}
	return c, nil
}

// ActivateCampaign moves a draft campaign to active. A product may have at most
// one active campaign (FR-322). Requires owner role.
func (s *TestingService) ActivateCampaign(ctx context.Context, user *model.User, productID, id string) (*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	c, err := s.campaigns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.ProductID != productID {
		return nil, ErrNotFound
	}
	if c.Status != model.CampaignStatusDraft {
		return nil, fmt.Errorf("only draft campaigns can be activated, this one is %s", c.Status)
	}

	active, err := s.campaigns.GetActiveByProductID(ctx, productID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, fmt.Errorf("campaign %q is already active; close it first", active.Name)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c.Status = model.CampaignStatusActive
	c.UpdatedAt = now
	if err := s.campaigns.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("activate campaign: %w", err)
	}

	s.auditCampaignStatus(ctx, user, c.ID, model.CampaignStatusDraft, model.CampaignStatusActive, now)
	return c, nil
}

// CloseCampaign closes an active campaign, freezing its results (FR-332).
// Without force, a campaign that still has incomplete results is refused with an
// *IncompleteResultsError so the caller can confirm (FR-333). Requires owner role.
func (s *TestingService) CloseCampaign(ctx context.Context, user *model.User, productID, id string, force bool) (*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	c, err := s.campaigns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.ProductID != productID {
		return nil, ErrNotFound
	}
	if c.Status != model.CampaignStatusActive {
		return nil, fmt.Errorf("only active campaigns can be closed, this one is %s", c.Status)
	}

	if !force {
		results, err := s.campaignResults.ListByCampaignID(ctx, id)
		if err != nil {
			return nil, err
		}
		incomplete := 0
		for _, r := range results {
			if model.IsIncompleteResultStatus(r.Status) {
				incomplete++
			}
		}
		if incomplete > 0 {
			return nil, &IncompleteResultsError{Incomplete: incomplete}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c.Status = model.CampaignStatusClosed
	c.ClosedAt = now
	c.ClosedBy = user.ID
	c.UpdatedAt = now
	if err := s.campaigns.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("close campaign: %w", err)
	}

	s.auditCampaignStatus(ctx, user, c.ID, model.CampaignStatusActive, model.CampaignStatusClosed, now)
	return c, nil
}

// auditCampaignStatus records a campaign status transition with field-level
// detail, so the history tab names the old and new state (FR-332).
func (s *TestingService) auditCampaignStatus(ctx context.Context, user *model.User, campaignID, from, to, now string) {
	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "testing_campaign", EntityID: campaignID,
		UserID: user.ID, Action: "update", FieldName: "status",
		OldValue: from, NewValue: to, CreatedAt: now,
	})
}

// GetCampaign returns a campaign by ID. Requires viewer role.
func (s *TestingService) GetCampaign(ctx context.Context, user *model.User, productID, id string) (*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.campaigns.GetByID(ctx, id)
}

// UpdateCampaign modifies a campaign. Requires owner role.
func (s *TestingService) UpdateCampaign(ctx context.Context, user *model.User, productID, id string, input UpdateCampaignInput) (*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	c, err := s.campaigns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status == model.CampaignStatusClosed {
		return nil, errors.New("a closed campaign cannot be modified")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if input.Name != nil {
		c.Name = *input.Name
	}
	if input.Description != nil {
		c.Description = *input.Description
	}
	if input.StartsAt != nil {
		c.StartsAt = *input.StartsAt
	}
	if input.EndsAt != nil {
		c.EndsAt = *input.EndsAt
	}
	c.UpdatedAt = now

	if err := s.campaigns.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("update campaign: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "testing_campaign", EntityID: c.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return c, nil
}

// DeleteCampaign removes a campaign. Requires owner role.
func (s *TestingService) DeleteCampaign(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.campaigns.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete campaign: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "testing_campaign", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListCampaigns returns all campaigns for a product. Requires viewer role.
func (s *TestingService) ListCampaigns(ctx context.Context, user *model.User, productID string) ([]*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.campaigns.ListByProductID(ctx, productID)
}

// ListCampaignResults returns results for a campaign. Requires viewer role.
func (s *TestingService) ListCampaignResults(ctx context.Context, user *model.User, productID, campaignID string) ([]*model.CampaignResult, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.campaignResults.ListByCampaignID(ctx, campaignID)
}

// ListResultsForTestCase returns a test case's results across all campaigns.
// Requires viewer role.
func (s *TestingService) ListResultsForTestCase(ctx context.Context, user *model.User, productID, testCaseID string) ([]*model.CampaignResult, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.campaignResults.ListByTestCaseID(ctx, testCaseID)
}

// ListResultEvents returns the status change log for a campaign, ordered
// oldest first. Requires viewer role (FR-042, UC-207).
func (s *TestingService) ListResultEvents(ctx context.Context, user *model.User, productID, campaignID string) ([]*model.CampaignResultEvent, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.campaignEvents.ListByCampaignID(ctx, campaignID)
}

// ListEventsForResult returns the status change log for a single result, oldest
// first, so a tester can see who set the current verdict before changing it.
// Requires viewer role (FR-042).
func (s *TestingService) ListEventsForResult(ctx context.Context, user *model.User, productID, campaignID, resultID string) ([]*model.CampaignResultEvent, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}

	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if c.ProductID != productID {
		return nil, ErrNotFound
	}

	result, err := s.campaignResults.GetByID(ctx, resultID)
	if err != nil {
		return nil, err
	}
	if result.CampaignID != campaignID {
		return nil, ErrNotFound
	}

	return s.campaignEvents.ListByResultID(ctx, resultID)
}

// ResultChurn returns how many times each result in the campaign has been set,
// keyed by result id. A count above one means the verdict was changed after it
// was first recorded. Requires viewer role.
func (s *TestingService) ResultChurn(ctx context.Context, user *model.User, productID, campaignID string) (map[string]int, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.campaignEvents.CountsByCampaignID(ctx, campaignID)
}

// ListVerificationEvents returns the full result history behind a defect: every
// change to every result the defect ever failed, including the later pass that
// verifies the fix. Requires viewer role, matching every other defect read.
//
// The defect itself belongs to the backlog, which this service cannot read, so
// the product scope is enforced on the way out: events whose campaign is not
// part of this product are dropped rather than trusted.
func (s *TestingService) ListVerificationEvents(ctx context.Context, user *model.User, productID, defectID string) ([]*model.CampaignResultEvent, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}

	events, err := s.campaignEvents.ListByDefectTrail(ctx, defectID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}

	campaigns, err := s.campaigns.ListByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list campaigns for verification scope: %w", err)
	}
	inProduct := make(map[string]bool, len(campaigns))
	for _, c := range campaigns {
		inProduct[c.ID] = true
	}

	scoped := make([]*model.CampaignResultEvent, 0, len(events))
	for _, ev := range events {
		if inProduct[ev.CampaignID] {
			scoped = append(scoped, ev)
		}
	}
	return scoped, nil
}

// CampaignProgress returns result counts keyed by status for one campaign.
// Requires viewer role.
func (s *TestingService) CampaignProgress(ctx context.Context, user *model.User, productID, campaignID string) (map[string]int, error) {
	results, err := s.ListCampaignResults(ctx, user, productID, campaignID)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(results))
	for _, r := range results {
		counts[r.Status]++
	}
	return counts, nil
}

// ListCampaignProgress returns result counts per campaign for a whole product
// in one query, for the campaign list page. Requires viewer role.
func (s *TestingService) ListCampaignProgress(ctx context.Context, user *model.User, productID string) (map[string]map[string]int, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.campaignResults.CountsByProductID(ctx, productID)
}

// SetResultStatus records a tester's verdict on one campaign result. Results are
// writable only while their campaign is active (FR-505), and marking one failed
// requires a defect that explains the failure (FR-502). Requires tester role.
func (s *TestingService) SetResultStatus(ctx context.Context, user *model.User, productID, campaignID, resultID, status, defectID string) (*model.CampaignResult, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleTester); err != nil {
		return nil, err
	}

	if !model.ValidCampaignResultStatus(status) {
		return nil, fmt.Errorf("invalid campaign result status: %q", status)
	}
	if status == model.CampaignResultStatusFailed && defectID == "" {
		return nil, errors.New("failing a test case requires a linked defect")
	}

	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if c.ProductID != productID {
		return nil, ErrNotFound
	}
	if c.Status != model.CampaignStatusActive {
		return nil, fmt.Errorf("results can only be changed while the campaign is active, this one is %s", c.Status)
	}

	result, err := s.campaignResults.GetByID(ctx, resultID)
	if err != nil {
		return nil, err
	}
	if result.CampaignID != campaignID {
		return nil, ErrNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)
	from := result.Status
	result.Status = status
	// A passing or skipped result carries no defect; any other verdict keeps
	// the defect it was given, or the one already on record.
	switch status {
	case model.CampaignResultStatusPassed, model.CampaignResultStatusSkipped:
		result.DefectID = ""
	default:
		if defectID != "" {
			result.DefectID = defectID
		}
	}
	result.ExecutedBy = user.ID
	result.ExecutedAt = now
	result.UpdatedAt = now

	if err := s.campaignResults.Update(ctx, result); err != nil {
		return nil, fmt.Errorf("update campaign result: %w", err)
	}
	s.recordResultEvent(ctx, result, user.ID, from, now)

	return result, nil
}

// MarkResultsForRetest moves every failed result blamed on the given defect to
// re-test. Called by the backlog service once a defect is closed, which has
// already authorized the caller, so this performs no role check of its own.
// A closed defect never marks a result as passed — only a tester can (FR-504).
//
// Results in a campaign that is no longer active are left alone: a closed
// campaign records what was true when it was closed, and fixing a defect
// afterwards must not rewrite that verdict.
func (s *TestingService) MarkResultsForRetest(ctx context.Context, defectID, userID string) error {
	results, err := s.campaignResults.ListByDefectID(ctx, defectID)
	if err != nil {
		return fmt.Errorf("list results for defect: %w", err)
	}

	active := make(map[string]bool)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, result := range results {
		if result.Status != model.CampaignResultStatusFailed {
			continue
		}
		ok, cached := active[result.CampaignID]
		if !cached {
			campaign, err := s.campaigns.GetByID(ctx, result.CampaignID)
			if err != nil {
				return fmt.Errorf("look up campaign for result: %w", err)
			}
			ok = campaign.Status == model.CampaignStatusActive
			active[result.CampaignID] = ok
		}
		if !ok {
			continue
		}
		from := result.Status
		result.Status = model.CampaignResultStatusRetest
		result.UpdatedAt = now
		if err := s.campaignResults.Update(ctx, result); err != nil {
			return fmt.Errorf("mark result for retest: %w", err)
		}
		s.recordResultEvent(ctx, result, userID, from, now)
	}
	return nil
}

// recordResultEvent appends to the immutable result change log (FR-042). Like
// audit entries, a failure here must not undo the change it describes.
func (s *TestingService) recordResultEvent(ctx context.Context, result *model.CampaignResult, userID, from, now string) {
	_ = s.campaignEvents.Create(ctx, &model.CampaignResultEvent{
		ID:         uuid.New().String(),
		ResultID:   result.ID,
		CampaignID: result.CampaignID,
		UserID:     userID,
		FromStatus: from,
		ToStatus:   result.Status,
		DefectID:   result.DefectID,
		CreatedAt:  now,
	})
}
