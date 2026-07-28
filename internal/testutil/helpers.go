package testutil

import (
	"time"

	"nexmap.net/internal/model"
)

// MakeUser returns an active user with sensible defaults. Overrides can customize fields.
func MakeUser(overrides ...func(*model.User)) *model.User {
	now := time.Now().UTC().Format(time.RFC3339)
	u := &model.User{
		ID:           "user-001",
		Username:     "testuser",
		Email:        "test@example.com",
		DisplayName:  "Test User",
		PasswordHash: "hashed",
		IsAdmin:      false,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	for _, fn := range overrides {
		fn(u)
	}
	return u
}

// MakeSession returns a valid session expiring 24h in the future.
func MakeSession(overrides ...func(*model.Session)) *model.Session {
	now := time.Now().UTC()
	s := &model.Session{
		ID:        "session-001",
		UserID:    "user-001",
		CSRFToken: "csrf-token-001",
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339),
		IPAddress: "127.0.0.1",
		UserAgent: "TestAgent/1.0",
	}
	for _, fn := range overrides {
		fn(s)
	}
	return s
}

// MakePAT returns a valid, non-revoked, non-expired PAT.
func MakePAT(overrides ...func(*model.PAT)) *model.PAT {
	now := time.Now().UTC().Format(time.RFC3339)
	p := &model.PAT{
		ID:          "pat-001",
		UserID:      "user-001",
		Name:        "test-token",
		TokenHash:   "somehash",
		TokenPrefix: "nxm_abcd",
		CreatedAt:   now,
	}
	for _, fn := range overrides {
		fn(p)
	}
	return p
}

// MakeInvitation returns a valid, non-accepted invitation.
func MakeInvitation(overrides ...func(*model.Invitation)) *model.Invitation {
	now := time.Now().UTC()
	inv := &model.Invitation{
		ID:        "inv-001",
		Email:     "invite@example.com",
		TokenHash: "invhash",
		InvitedBy: "user-001",
		ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		CreatedAt: now.Format(time.RFC3339),
	}
	for _, fn := range overrides {
		fn(inv)
	}
	return inv
}

// FutureRFC3339 returns a time d in the future formatted as RFC3339.
func FutureRFC3339(d time.Duration) string {
	return time.Now().UTC().Add(d).Format(time.RFC3339)
}

// PastRFC3339 returns a time d in the past formatted as RFC3339.
func PastRFC3339(d time.Duration) string {
	return time.Now().UTC().Add(-d).Format(time.RFC3339)
}

// MakeProduct returns a product with sensible defaults.
func MakeProduct(overrides ...func(*model.Product)) *model.Product {
	now := time.Now().UTC().Format(time.RFC3339)
	p := &model.Product{
		ID:             "product-001",
		Name:           "Test Product",
		Description:    "A test product",
		EstimationMode: model.EstimationModeDisabled,
		WorkflowMode:   model.WorkflowModeKanban,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	for _, fn := range overrides {
		fn(p)
	}
	return p
}

// MakeProductRole returns a product role with sensible defaults.
func MakeProductRole(overrides ...func(*model.ProductRole)) *model.ProductRole {
	now := time.Now().UTC().Format(time.RFC3339)
	pr := &model.ProductRole{
		ID:        "role-001",
		UserID:    "user-001",
		ProductID: "product-001",
		Role:      model.RoleOwner,
		CreatedAt: now,
	}
	for _, fn := range overrides {
		fn(pr)
	}
	return pr
}

// MakeComponent returns a component with sensible defaults.
func MakeComponent(overrides ...func(*model.Component)) *model.Component {
	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Component{
		ID:          "component-001",
		ProductID:   "product-001",
		Name:        "Test Component",
		Description: "A test component",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for _, fn := range overrides {
		fn(c)
	}
	return c
}

// MakeCapability returns a capability with sensible defaults.
func MakeCapability(overrides ...func(*model.Capability)) *model.Capability {
	now := time.Now().UTC().Format(time.RFC3339)
	f := &model.Capability{
		ID:          "capability-001",
		ProductID:   "product-001",
		ComponentID: "component-001",
		Name:        "Test Capability",
		Description: "A test capability",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for _, fn := range overrides {
		fn(f)
	}
	return f
}

// MakeRequirementCapability returns a requirement-capability link with sensible defaults.
func MakeRequirementCapability(overrides ...func(*model.RequirementCapability)) *model.RequirementCapability {
	now := time.Now().UTC().Format(time.RFC3339)
	rc := &model.RequirementCapability{
		ID:            "reqcap-001",
		RequirementID: "req-001",
		CapabilityID:  "capability-001",
		CreatedAt:     now,
	}
	for _, fn := range overrides {
		fn(rc)
	}
	return rc
}

// MakeMilestone returns a milestone with sensible defaults.
func MakeMilestone(overrides ...func(*model.Milestone)) *model.Milestone {
	now := time.Now().UTC().Format(time.RFC3339)
	m := &model.Milestone{
		ID:          "milestone-001",
		ProductID:   "product-001",
		Name:        "Test Milestone",
		Description: "A test milestone",
		Type:        model.MilestoneTypePlanning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for _, fn := range overrides {
		fn(m)
	}
	return m
}

// MakeSprint returns a sprint with sensible defaults.
func MakeSprint(overrides ...func(*model.Sprint)) *model.Sprint {
	now := time.Now().UTC().Format(time.RFC3339)
	s := &model.Sprint{
		ID:        "sprint-001",
		ProductID: "product-001",
		Name:      "Test Sprint",
		StartDate: now,
		EndDate:   FutureRFC3339(14 * 24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, fn := range overrides {
		fn(s)
	}
	return s
}

// MakeStatus returns a status with sensible defaults.
func MakeStatus(overrides ...func(*model.Status)) *model.Status {
	now := time.Now().UTC().Format(time.RFC3339)
	s := &model.Status{
		ID:         "status-001",
		ProductID:  "product-001",
		EntityType: "requirement",
		Name:       "Open",
		Position:   0,
		IsDefault:  true,
		CreatedAt:  now,
	}
	for _, fn := range overrides {
		fn(s)
	}
	return s
}

// MakeCategory returns a category with sensible defaults.
func MakeCategory(overrides ...func(*model.Category)) *model.Category {
	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Category{
		ID:         "cat-001",
		ProductID:  "product-001",
		EntityType: model.RequirementKindRequirement,
		Name:       "Functionality",
		Prefix:     "FR",
		Position:   1,
		CreatedAt:  now,
	}
	for _, fn := range overrides {
		fn(c)
	}
	return c
}

// MakeRequirement returns a requirement with sensible defaults.
func MakeRequirement(overrides ...func(*model.Requirement)) *model.Requirement {
	now := time.Now().UTC().Format(time.RFC3339)
	r := &model.Requirement{
		ID:         "req-001",
		ProductID:  "product-001",
		Kind:       model.RequirementKindRequirement,
		CategoryID: "cat-001",
		Title:      "Test Requirement",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	for _, fn := range overrides {
		fn(r)
	}
	return r
}

// MakeTask returns a task (a backlog item of kind "task") with sensible defaults.
func MakeTask(overrides ...func(*model.Requirement)) *model.Requirement {
	t := MakeRequirement(func(r *model.Requirement) {
		r.ID = "task-001"
		r.Kind = model.RequirementKindTask
		r.CategoryID = "taskcat-001"
		r.Title = "Test Task"
	})
	for _, fn := range overrides {
		fn(t)
	}
	return t
}

// MakeConstraint returns a constraint with sensible defaults.
func MakeConstraint(overrides ...func(*model.Constraint)) *model.Constraint {
	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Constraint{
		ID:              "constraint-001",
		ProductID:       "product-001",
		CategoryID:      "cat-001",
		SequenceNumber:  1,
		Title:           "Test Constraint",
		Description:     "A test constraint",
		ComplianceState: model.ComplianceNotAssessed,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	for _, fn := range overrides {
		fn(c)
	}
	return c
}

// MakeConstraintCapability returns a constraint-capability link with sensible defaults.
func MakeConstraintCapability(overrides ...func(*model.ConstraintCapability)) *model.ConstraintCapability {
	now := time.Now().UTC().Format(time.RFC3339)
	cc := &model.ConstraintCapability{
		ID:           "concap-001",
		ConstraintID: "constraint-001",
		CapabilityID: "capability-001",
		CreatedAt:    now,
	}
	for _, fn := range overrides {
		fn(cc)
	}
	return cc
}

// MakeDefect returns a defect with sensible defaults.
func MakeDefect(overrides ...func(*model.Defect)) *model.Defect {
	now := time.Now().UTC().Format(time.RFC3339)
	d := &model.Defect{
		ID:          "defect-001",
		ProductID:   "product-001",
		Title:       "Test Defect",
		Criticality: model.DefectCriticalityMedium,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for _, fn := range overrides {
		fn(d)
	}
	return d
}

// MakeTestCase returns a test case with sensible defaults.
func MakeTestCase(overrides ...func(*model.TestCase)) *model.TestCase {
	now := time.Now().UTC().Format(time.RFC3339)
	tc := &model.TestCase{
		ID:            "tc-001",
		ProductID:     "product-001",
		Title:         "Test Case",
		ExecutionMode: model.ExecutionModeManual,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	for _, fn := range overrides {
		fn(tc)
	}
	return tc
}

// MakeTestingCampaign returns a testing campaign with sensible defaults.
func MakeTestingCampaign(overrides ...func(*model.TestingCampaign)) *model.TestingCampaign {
	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.TestingCampaign{
		ID:        "campaign-001",
		ProductID: "product-001",
		Name:      "Test Campaign",
		Status:    model.CampaignStatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, fn := range overrides {
		fn(c)
	}
	return c
}

// MakeCampaignResult returns a pending campaign result with sensible defaults.
func MakeCampaignResult(overrides ...func(*model.CampaignResult)) *model.CampaignResult {
	now := time.Now().UTC().Format(time.RFC3339)
	cr := &model.CampaignResult{
		ID:         "cr-001",
		CampaignID: "campaign-001",
		TestCaseID: "tc-001",
		Status:     model.CampaignResultStatusPending,
		UpdatedAt:  now,
	}
	for _, fn := range overrides {
		fn(cr)
	}
	return cr
}

// MakeRuntimeEnvironment returns a runtime environment with sensible defaults.
func MakeRuntimeEnvironment(overrides ...func(*model.RuntimeEnvironment)) *model.RuntimeEnvironment {
	now := time.Now().UTC().Format(time.RFC3339)
	e := &model.RuntimeEnvironment{
		ID:        "env-001",
		ProductID: "product-001",
		Name:      "linux",
		CreatedAt: now,
	}
	for _, fn := range overrides {
		fn(e)
	}
	return e
}

// MakeComment returns a comment with sensible defaults.
func MakeComment(overrides ...func(*model.Comment)) *model.Comment {
	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Comment{
		ID:        "comment-001",
		ProductID: "product-001",
		OwnerType: model.OwnerTypeRequirement,
		OwnerID:   "req-001",
		UserID:    "user-001",
		Body:      "Test comment",
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, fn := range overrides {
		fn(c)
	}
	return c
}

// MakeAttachment returns an attachment with sensible defaults.
func MakeAttachment(overrides ...func(*model.Attachment)) *model.Attachment {
	now := time.Now().UTC().Format(time.RFC3339)
	a := &model.Attachment{
		ID:          "attachment-001",
		ProductID:   "product-001",
		OwnerType:   model.OwnerTypeRequirement,
		OwnerID:     "req-001",
		UserID:      "user-001",
		Filename:    "test.pdf",
		ContentType: "application/pdf",
		Size:        1024,
		StoragePath: "product-001/attachment-001",
		CreatedAt:   now,
	}
	for _, fn := range overrides {
		fn(a)
	}
	return a
}

// MakeAuditEntry returns an audit entry with sensible defaults.
func MakeAuditEntry(overrides ...func(*model.AuditEntry)) *model.AuditEntry {
	now := time.Now().UTC().Format(time.RFC3339)
	e := &model.AuditEntry{
		ID:         "audit-001",
		EntityType: "product",
		EntityID:   "product-001",
		UserID:     "user-001",
		Action:     "create",
		CreatedAt:  now,
	}
	for _, fn := range overrides {
		fn(e)
	}
	return e
}
