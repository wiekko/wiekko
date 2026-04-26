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
	ProductID string
	Name      string
}

// UpdateCampaignInput contains the fields that can be updated on a campaign.
type UpdateCampaignInput struct {
	Name *string
}

// CreateCampaign creates a new testing campaign. Requires owner role.
func (s *TestingService) CreateCampaign(ctx context.Context, user *model.User, input CreateCampaignInput) (*model.TestingCampaign, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("campaign name is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.TestingCampaign{
		ID:        uuid.New().String(),
		ProductID: input.ProductID,
		Name:      input.Name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.campaigns.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create campaign: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "testing_campaign", EntityID: c.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return c, nil
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

	now := time.Now().UTC().Format(time.RFC3339)
	if input.Name != nil {
		c.Name = *input.Name
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

// UpdateCampaignResult updates a campaign result status. Requires tester role.
func (s *TestingService) UpdateCampaignResult(ctx context.Context, user *model.User, productID, campaignID, testCaseID, environmentID, status string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleTester); err != nil {
		return err
	}

	result, err := s.campaignResults.GetByCampaignAndTestCase(ctx, campaignID, testCaseID, environmentID)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result.Status = status
	result.UpdatedAt = now
	return s.campaignResults.Update(ctx, result)
}
