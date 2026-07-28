package testutil

import (
	"context"
	"fmt"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// MockUserStore implements store.UserStore with configurable function fields.
type MockUserStore struct {
	CreateFunc         func(ctx context.Context, user *model.User) error
	GetByIDFunc        func(ctx context.Context, id string) (*model.User, error)
	GetByUsernameFunc  func(ctx context.Context, username string) (*model.User, error)
	GetByEmailFunc     func(ctx context.Context, email string) (*model.User, error)
	UpdateFunc         func(ctx context.Context, user *model.User) error
	UpdateStatusFunc   func(ctx context.Context, id, status string) error
	UpdatePasswordFunc func(ctx context.Context, id, hash string) error
	DeleteFunc         func(ctx context.Context, id string) error
	ListFunc           func(ctx context.Context, filter store.UserFilter) ([]*model.User, error)
	CountFunc          func(ctx context.Context) (int64, error)
}

func (m *MockUserStore) Create(ctx context.Context, user *model.User) error {
	return m.CreateFunc(ctx, user)
}
func (m *MockUserStore) GetByID(ctx context.Context, id string) (*model.User, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockUserStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	return m.GetByUsernameFunc(ctx, username)
}
func (m *MockUserStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return m.GetByEmailFunc(ctx, email)
}
func (m *MockUserStore) Update(ctx context.Context, user *model.User) error {
	return m.UpdateFunc(ctx, user)
}
func (m *MockUserStore) UpdateStatus(ctx context.Context, id, status string) error {
	return m.UpdateStatusFunc(ctx, id, status)
}
func (m *MockUserStore) UpdatePassword(ctx context.Context, id, hash string) error {
	return m.UpdatePasswordFunc(ctx, id, hash)
}
func (m *MockUserStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockUserStore) List(ctx context.Context, filter store.UserFilter) ([]*model.User, error) {
	return m.ListFunc(ctx, filter)
}
func (m *MockUserStore) Count(ctx context.Context) (int64, error) {
	return m.CountFunc(ctx)
}

// MockSessionStore implements store.SessionStore with configurable function fields.
type MockSessionStore struct {
	CreateFunc         func(ctx context.Context, session *model.Session) error
	GetByIDFunc        func(ctx context.Context, id string) (*model.Session, error)
	DeleteFunc         func(ctx context.Context, id string) error
	DeleteByUserIDFunc func(ctx context.Context, userID string) error
	DeleteExpiredFunc  func(ctx context.Context) error
}

func (m *MockSessionStore) Create(ctx context.Context, session *model.Session) error {
	return m.CreateFunc(ctx, session)
}
func (m *MockSessionStore) GetByID(ctx context.Context, id string) (*model.Session, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockSessionStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockSessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	return m.DeleteByUserIDFunc(ctx, userID)
}
func (m *MockSessionStore) DeleteExpired(ctx context.Context) error {
	return m.DeleteExpiredFunc(ctx)
}

// MockPATStore implements store.PATStore with configurable function fields.
type MockPATStore struct {
	CreateFunc         func(ctx context.Context, pat *model.PAT) error
	GetByTokenHashFunc func(ctx context.Context, hash string) (*model.PAT, error)
	ListByUserIDFunc   func(ctx context.Context, userID string) ([]*model.PAT, error)
	RevokeFunc         func(ctx context.Context, id, revokedAt string) error
	UpdateLastUsedFunc func(ctx context.Context, id, at string) error
}

func (m *MockPATStore) Create(ctx context.Context, pat *model.PAT) error {
	return m.CreateFunc(ctx, pat)
}
func (m *MockPATStore) GetByTokenHash(ctx context.Context, hash string) (*model.PAT, error) {
	return m.GetByTokenHashFunc(ctx, hash)
}
func (m *MockPATStore) ListByUserID(ctx context.Context, userID string) ([]*model.PAT, error) {
	return m.ListByUserIDFunc(ctx, userID)
}
func (m *MockPATStore) Revoke(ctx context.Context, id, revokedAt string) error {
	return m.RevokeFunc(ctx, id, revokedAt)
}
func (m *MockPATStore) UpdateLastUsed(ctx context.Context, id, at string) error {
	return m.UpdateLastUsedFunc(ctx, id, at)
}

// MockSettingStore implements store.SettingStore with configurable function fields.
type MockSettingStore struct {
	GetFunc    func(ctx context.Context, key string) (string, error)
	SetFunc    func(ctx context.Context, key, value string) error
	GetAllFunc func(ctx context.Context) (map[string]string, error)
}

func (m *MockSettingStore) Get(ctx context.Context, key string) (string, error) {
	return m.GetFunc(ctx, key)
}
func (m *MockSettingStore) Set(ctx context.Context, key, value string) error {
	return m.SetFunc(ctx, key, value)
}
func (m *MockSettingStore) GetAll(ctx context.Context) (map[string]string, error) {
	return m.GetAllFunc(ctx)
}

// MockInvitationStore implements store.InvitationStore with configurable function fields.
type MockInvitationStore struct {
	CreateFunc         func(ctx context.Context, inv *model.Invitation) error
	GetByTokenHashFunc func(ctx context.Context, hash string) (*model.Invitation, error)
	ListFunc           func(ctx context.Context) ([]*model.Invitation, error)
	DeleteFunc         func(ctx context.Context, id string) error
	MarkAcceptedFunc   func(ctx context.Context, id, acceptedAt string) error
}

func (m *MockInvitationStore) Create(ctx context.Context, inv *model.Invitation) error {
	return m.CreateFunc(ctx, inv)
}
func (m *MockInvitationStore) GetByTokenHash(ctx context.Context, hash string) (*model.Invitation, error) {
	return m.GetByTokenHashFunc(ctx, hash)
}
func (m *MockInvitationStore) List(ctx context.Context) ([]*model.Invitation, error) {
	return m.ListFunc(ctx)
}
func (m *MockInvitationStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockInvitationStore) MarkAccepted(ctx context.Context, id, acceptedAt string) error {
	return m.MarkAcceptedFunc(ctx, id, acceptedAt)
}

// MockAuthProvider implements auth.Provider with configurable function fields.
type MockAuthProvider struct {
	AuthenticateFunc           func(ctx context.Context, username, password string) (*auth.Identity, error)
	ValidatePasswordFunc       func(password string, policy auth.PasswordPolicy) error
	HashPasswordFunc           func(password string) (string, error)
	VerifyPasswordFunc         func(password, hash string) error
	SupportsPasswordChangeFunc func() bool
}

func (m *MockAuthProvider) Authenticate(ctx context.Context, username, password string) (*auth.Identity, error) {
	return m.AuthenticateFunc(ctx, username, password)
}
func (m *MockAuthProvider) ValidatePassword(password string, policy auth.PasswordPolicy) error {
	return m.ValidatePasswordFunc(password, policy)
}
func (m *MockAuthProvider) HashPassword(password string) (string, error) {
	return m.HashPasswordFunc(password)
}
func (m *MockAuthProvider) VerifyPassword(password, hash string) error {
	return m.VerifyPasswordFunc(password, hash)
}
func (m *MockAuthProvider) SupportsPasswordChange() bool {
	return m.SupportsPasswordChangeFunc()
}

// MockProductRoleStore implements store.ProductRoleStore with configurable function fields.
type MockProductRoleStore struct {
	CreateFunc              func(ctx context.Context, role *model.ProductRole) error
	GetByUserAndProductFunc func(ctx context.Context, userID, productID string) (*model.ProductRole, error)
	UpdateFunc              func(ctx context.Context, role *model.ProductRole) error
	DeleteFunc              func(ctx context.Context, userID, productID string) error
	ListByProductIDFunc     func(ctx context.Context, productID string) ([]*model.ProductRole, error)
	ListByUserIDFunc        func(ctx context.Context, userID string) ([]*model.ProductRole, error)
}

func (m *MockProductRoleStore) Create(ctx context.Context, role *model.ProductRole) error {
	return m.CreateFunc(ctx, role)
}
func (m *MockProductRoleStore) GetByUserAndProduct(ctx context.Context, userID, productID string) (*model.ProductRole, error) {
	return m.GetByUserAndProductFunc(ctx, userID, productID)
}
func (m *MockProductRoleStore) Update(ctx context.Context, role *model.ProductRole) error {
	return m.UpdateFunc(ctx, role)
}
func (m *MockProductRoleStore) Delete(ctx context.Context, userID, productID string) error {
	return m.DeleteFunc(ctx, userID, productID)
}
func (m *MockProductRoleStore) ListByProductID(ctx context.Context, productID string) ([]*model.ProductRole, error) {
	return m.ListByProductIDFunc(ctx, productID)
}
func (m *MockProductRoleStore) ListByUserID(ctx context.Context, userID string) ([]*model.ProductRole, error) {
	return m.ListByUserIDFunc(ctx, userID)
}

// MockProductStore implements store.ProductStore with configurable function fields.
type MockProductStore struct {
	CreateFunc  func(ctx context.Context, product *model.Product) error
	GetByIDFunc func(ctx context.Context, id string) (*model.Product, error)
	UpdateFunc  func(ctx context.Context, product *model.Product) error
	DeleteFunc  func(ctx context.Context, id string) error
	ListFunc    func(ctx context.Context) ([]*model.Product, error)
}

func (m *MockProductStore) Create(ctx context.Context, product *model.Product) error {
	return m.CreateFunc(ctx, product)
}
func (m *MockProductStore) GetByID(ctx context.Context, id string) (*model.Product, error) {
	if m == nil || m.GetByIDFunc == nil {
		return nil, fmt.Errorf("product not found")
	}
	return m.GetByIDFunc(ctx, id)
}
func (m *MockProductStore) Update(ctx context.Context, product *model.Product) error {
	return m.UpdateFunc(ctx, product)
}
func (m *MockProductStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockProductStore) List(ctx context.Context) ([]*model.Product, error) {
	return m.ListFunc(ctx)
}

// MockAuditStore implements store.AuditStore with configurable function fields.
type MockAuditStore struct {
	CreateFunc       func(ctx context.Context, entry *model.AuditEntry) error
	ListByEntityFunc func(ctx context.Context, entityType, entityID string) ([]*model.AuditEntry, error)
}

func (m *MockAuditStore) Create(ctx context.Context, entry *model.AuditEntry) error {
	return m.CreateFunc(ctx, entry)
}
func (m *MockAuditStore) ListByEntity(ctx context.Context, entityType, entityID string) ([]*model.AuditEntry, error) {
	return m.ListByEntityFunc(ctx, entityType, entityID)
}

// MockComponentStore implements store.ComponentStore with configurable function fields.
type MockComponentStore struct {
	CreateFunc            func(ctx context.Context, c *model.Component) error
	GetByIDFunc           func(ctx context.Context, id string) (*model.Component, error)
	UpdateFunc            func(ctx context.Context, c *model.Component) error
	DeleteFunc            func(ctx context.Context, id string) error
	ListByProductIDFunc   func(ctx context.Context, productID string) ([]*model.Component, error)
	ListByMilestoneIDFunc func(ctx context.Context, milestoneID string) ([]*model.Component, error)
}

func (m *MockComponentStore) Create(ctx context.Context, c *model.Component) error {
	return m.CreateFunc(ctx, c)
}
func (m *MockComponentStore) GetByID(ctx context.Context, id string) (*model.Component, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockComponentStore) Update(ctx context.Context, c *model.Component) error {
	return m.UpdateFunc(ctx, c)
}
func (m *MockComponentStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockComponentStore) ListByProductID(ctx context.Context, productID string) ([]*model.Component, error) {
	return m.ListByProductIDFunc(ctx, productID)
}
func (m *MockComponentStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Component, error) {
	if m.ListByMilestoneIDFunc == nil {
		return nil, nil
	}
	return m.ListByMilestoneIDFunc(ctx, milestoneID)
}

// MockConstraintStore implements store.ConstraintStore with configurable function fields.
type MockConstraintStore struct {
	CreateFunc                       func(ctx context.Context, c *model.Constraint) error
	GetByIDFunc                      func(ctx context.Context, id string) (*model.Constraint, error)
	UpdateFunc                       func(ctx context.Context, c *model.Constraint) error
	DeleteFunc                       func(ctx context.Context, id string) error
	ListByProductIDFunc              func(ctx context.Context, productID string, filter store.ConstraintFilter) ([]*model.Constraint, error)
	ListByMilestoneIDFunc            func(ctx context.Context, milestoneID string) ([]*model.Constraint, error)
	NextSequenceNumberFunc           func(ctx context.Context, productID string) (int, error)
	LinkCapabilityFunc               func(ctx context.Context, link *model.ConstraintCapability) error
	UnlinkCapabilityFunc             func(ctx context.Context, constraintID, capabilityID string) error
	ListCapabilitiesByConstraintFunc func(ctx context.Context, constraintID string) ([]*model.ConstraintCapability, error)
	ListConstraintsByCapabilityFunc  func(ctx context.Context, capabilityID string) ([]*model.ConstraintCapability, error)
}

func (m *MockConstraintStore) Create(ctx context.Context, c *model.Constraint) error {
	return m.CreateFunc(ctx, c)
}
func (m *MockConstraintStore) GetByID(ctx context.Context, id string) (*model.Constraint, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockConstraintStore) Update(ctx context.Context, c *model.Constraint) error {
	return m.UpdateFunc(ctx, c)
}
func (m *MockConstraintStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockConstraintStore) ListByProductID(ctx context.Context, productID string, filter store.ConstraintFilter) ([]*model.Constraint, error) {
	return m.ListByProductIDFunc(ctx, productID, filter)
}
func (m *MockConstraintStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Constraint, error) {
	if m.ListByMilestoneIDFunc == nil {
		return nil, nil
	}
	return m.ListByMilestoneIDFunc(ctx, milestoneID)
}
func (m *MockConstraintStore) NextSequenceNumber(ctx context.Context, productID string) (int, error) {
	if m.NextSequenceNumberFunc == nil {
		return 1, nil
	}
	return m.NextSequenceNumberFunc(ctx, productID)
}
func (m *MockConstraintStore) LinkCapability(ctx context.Context, link *model.ConstraintCapability) error {
	return m.LinkCapabilityFunc(ctx, link)
}
func (m *MockConstraintStore) UnlinkCapability(ctx context.Context, constraintID, capabilityID string) error {
	return m.UnlinkCapabilityFunc(ctx, constraintID, capabilityID)
}
func (m *MockConstraintStore) ListCapabilitiesByConstraint(ctx context.Context, constraintID string) ([]*model.ConstraintCapability, error) {
	return m.ListCapabilitiesByConstraintFunc(ctx, constraintID)
}
func (m *MockConstraintStore) ListConstraintsByCapability(ctx context.Context, capabilityID string) ([]*model.ConstraintCapability, error) {
	return m.ListConstraintsByCapabilityFunc(ctx, capabilityID)
}

// MockCapabilityStore implements store.CapabilityStore with configurable function fields.
type MockCapabilityStore struct {
	CreateFunc                        func(ctx context.Context, c *model.Capability) error
	GetByIDFunc                       func(ctx context.Context, id string) (*model.Capability, error)
	UpdateFunc                        func(ctx context.Context, c *model.Capability) error
	DeleteFunc                        func(ctx context.Context, id string) error
	ListByProductIDFunc               func(ctx context.Context, productID string) ([]*model.Capability, error)
	ListByComponentIDFunc             func(ctx context.Context, componentID string) ([]*model.Capability, error)
	ListByMilestoneIDFunc             func(ctx context.Context, milestoneID string) ([]*model.Capability, error)
	LinkRequirementFunc               func(ctx context.Context, link *model.RequirementCapability) error
	UnlinkRequirementFunc             func(ctx context.Context, capabilityID, requirementID string) error
	ListRequirementsByCapabilityFunc  func(ctx context.Context, capabilityID string) ([]*model.RequirementCapability, error)
	ListCapabilitiesByRequirementFunc func(ctx context.Context, requirementID string) ([]*model.RequirementCapability, error)
}

func (m *MockCapabilityStore) Create(ctx context.Context, c *model.Capability) error {
	return m.CreateFunc(ctx, c)
}
func (m *MockCapabilityStore) GetByID(ctx context.Context, id string) (*model.Capability, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockCapabilityStore) Update(ctx context.Context, c *model.Capability) error {
	return m.UpdateFunc(ctx, c)
}
func (m *MockCapabilityStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockCapabilityStore) ListByProductID(ctx context.Context, productID string) ([]*model.Capability, error) {
	return m.ListByProductIDFunc(ctx, productID)
}
func (m *MockCapabilityStore) ListByComponentID(ctx context.Context, componentID string) ([]*model.Capability, error) {
	return m.ListByComponentIDFunc(ctx, componentID)
}
func (m *MockCapabilityStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Capability, error) {
	if m.ListByMilestoneIDFunc == nil {
		return nil, nil
	}
	return m.ListByMilestoneIDFunc(ctx, milestoneID)
}
func (m *MockCapabilityStore) LinkRequirement(ctx context.Context, link *model.RequirementCapability) error {
	return m.LinkRequirementFunc(ctx, link)
}
func (m *MockCapabilityStore) UnlinkRequirement(ctx context.Context, capabilityID, requirementID string) error {
	return m.UnlinkRequirementFunc(ctx, capabilityID, requirementID)
}
func (m *MockCapabilityStore) ListRequirementsByCapability(ctx context.Context, capabilityID string) ([]*model.RequirementCapability, error) {
	return m.ListRequirementsByCapabilityFunc(ctx, capabilityID)
}
func (m *MockCapabilityStore) ListCapabilitiesByRequirement(ctx context.Context, requirementID string) ([]*model.RequirementCapability, error) {
	return m.ListCapabilitiesByRequirementFunc(ctx, requirementID)
}

// MockMilestoneStore implements store.MilestoneStore with configurable function fields.
type MockMilestoneStore struct {
	CreateFunc          func(ctx context.Context, m *model.Milestone) error
	GetByIDFunc         func(ctx context.Context, id string) (*model.Milestone, error)
	UpdateFunc          func(ctx context.Context, m *model.Milestone) error
	DeleteFunc          func(ctx context.Context, id string) error
	ListByProductIDFunc func(ctx context.Context, productID string) ([]*model.Milestone, error)
}

func (m *MockMilestoneStore) Create(ctx context.Context, ms *model.Milestone) error {
	return m.CreateFunc(ctx, ms)
}
func (m *MockMilestoneStore) GetByID(ctx context.Context, id string) (*model.Milestone, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockMilestoneStore) Update(ctx context.Context, ms *model.Milestone) error {
	return m.UpdateFunc(ctx, ms)
}
func (m *MockMilestoneStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockMilestoneStore) ListByProductID(ctx context.Context, productID string) ([]*model.Milestone, error) {
	return m.ListByProductIDFunc(ctx, productID)
}

// MockSprintStore implements store.SprintStore with configurable function fields.
type MockSprintStore struct {
	CreateFunc            func(ctx context.Context, sprint *model.Sprint) error
	GetByIDFunc           func(ctx context.Context, id string) (*model.Sprint, error)
	UpdateFunc            func(ctx context.Context, sprint *model.Sprint) error
	DeleteFunc            func(ctx context.Context, id string) error
	ListByProductIDFunc   func(ctx context.Context, productID string) ([]*model.Sprint, error)
	ListByMilestoneIDFunc func(ctx context.Context, milestoneID string) ([]*model.Sprint, error)
}

func (m *MockSprintStore) Create(ctx context.Context, sprint *model.Sprint) error {
	return m.CreateFunc(ctx, sprint)
}
func (m *MockSprintStore) GetByID(ctx context.Context, id string) (*model.Sprint, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockSprintStore) Update(ctx context.Context, sprint *model.Sprint) error {
	return m.UpdateFunc(ctx, sprint)
}
func (m *MockSprintStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockSprintStore) ListByProductID(ctx context.Context, productID string) ([]*model.Sprint, error) {
	return m.ListByProductIDFunc(ctx, productID)
}
func (m *MockSprintStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Sprint, error) {
	if m.ListByMilestoneIDFunc == nil {
		return nil, nil
	}
	return m.ListByMilestoneIDFunc(ctx, milestoneID)
}

// MockStatusStore implements store.StatusStore with configurable function fields.
type MockStatusStore struct {
	CreateFunc              func(ctx context.Context, status *model.Status) error
	GetByIDFunc             func(ctx context.Context, id string) (*model.Status, error)
	UpdateFunc              func(ctx context.Context, status *model.Status) error
	DeleteFunc              func(ctx context.Context, id string) error
	ListByProductIDFunc     func(ctx context.Context, productID, entityType string) ([]*model.Status, error)
	GetDefaultFunc          func(ctx context.Context, productID, entityType string) (*model.Status, error)
	GetByProductAndNameFunc func(ctx context.Context, productID, entityType, name string) (*model.Status, error)
}

func (m *MockStatusStore) Create(ctx context.Context, status *model.Status) error {
	return m.CreateFunc(ctx, status)
}
func (m *MockStatusStore) GetByID(ctx context.Context, id string) (*model.Status, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockStatusStore) Update(ctx context.Context, status *model.Status) error {
	return m.UpdateFunc(ctx, status)
}
func (m *MockStatusStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockStatusStore) ListByProductID(ctx context.Context, productID, entityType string) ([]*model.Status, error) {
	return m.ListByProductIDFunc(ctx, productID, entityType)
}
func (m *MockStatusStore) GetDefault(ctx context.Context, productID, entityType string) (*model.Status, error) {
	return m.GetDefaultFunc(ctx, productID, entityType)
}
func (m *MockStatusStore) GetByProductAndName(ctx context.Context, productID, entityType, name string) (*model.Status, error) {
	if m.GetByProductAndNameFunc != nil {
		return m.GetByProductAndNameFunc(ctx, productID, entityType, name)
	}
	return nil, fmt.Errorf("status not found")
}

// MockPyramidLevelStore implements store.PyramidLevelStore with configurable function fields.
type MockPyramidLevelStore struct {
	CreateFunc          func(ctx context.Context, pl *model.PyramidLevel) error
	GetByIDFunc         func(ctx context.Context, id string) (*model.PyramidLevel, error)
	UpdateFunc          func(ctx context.Context, pl *model.PyramidLevel) error
	DeleteFunc          func(ctx context.Context, id string) error
	ListByProductIDFunc func(ctx context.Context, productID string) ([]*model.PyramidLevel, error)
}

func (m *MockPyramidLevelStore) Create(ctx context.Context, pl *model.PyramidLevel) error {
	return m.CreateFunc(ctx, pl)
}
func (m *MockPyramidLevelStore) GetByID(ctx context.Context, id string) (*model.PyramidLevel, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockPyramidLevelStore) Update(ctx context.Context, pl *model.PyramidLevel) error {
	return m.UpdateFunc(ctx, pl)
}
func (m *MockPyramidLevelStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockPyramidLevelStore) ListByProductID(ctx context.Context, productID string) ([]*model.PyramidLevel, error) {
	return m.ListByProductIDFunc(ctx, productID)
}

// MockRuntimeEnvironmentStore implements store.RuntimeEnvironmentStore with configurable function fields.
type MockRuntimeEnvironmentStore struct {
	CreateFunc          func(ctx context.Context, env *model.RuntimeEnvironment) error
	GetByIDFunc         func(ctx context.Context, id string) (*model.RuntimeEnvironment, error)
	DeleteFunc          func(ctx context.Context, id string) error
	ListByProductIDFunc func(ctx context.Context, productID string) ([]*model.RuntimeEnvironment, error)
}

func (m *MockRuntimeEnvironmentStore) Create(ctx context.Context, env *model.RuntimeEnvironment) error {
	return m.CreateFunc(ctx, env)
}
func (m *MockRuntimeEnvironmentStore) GetByID(ctx context.Context, id string) (*model.RuntimeEnvironment, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockRuntimeEnvironmentStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockRuntimeEnvironmentStore) ListByProductID(ctx context.Context, productID string) ([]*model.RuntimeEnvironment, error) {
	return m.ListByProductIDFunc(ctx, productID)
}

// MockRequirementStore implements store.RequirementStore with configurable function fields.
type MockRequirementStore struct {
	CreateFunc             func(ctx context.Context, r *model.Requirement) error
	GetByIDFunc            func(ctx context.Context, id string) (*model.Requirement, error)
	UpdateFunc             func(ctx context.Context, r *model.Requirement) error
	DeleteFunc             func(ctx context.Context, id string) error
	ListByProductIDFunc    func(ctx context.Context, productID string, filter store.RequirementFilter) ([]*model.Requirement, error)
	ListByParentIDFunc     func(ctx context.Context, parentID string) ([]*model.Requirement, error)
	ListBySprintIDFunc     func(ctx context.Context, sprintID string) ([]*model.Requirement, error)
	ListByMilestoneIDFunc  func(ctx context.Context, milestoneID string) ([]*model.Requirement, error)
	NextSequenceNumberFunc func(ctx context.Context, productID string) (int, error)
	AssignFunc             func(ctx context.Context, a *model.Assignment) error
	UnassignFunc           func(ctx context.Context, requirementID, userID string) error
	ListAssigneesFunc      func(ctx context.Context, requirementID string) ([]*model.Assignment, error)
}

func (m *MockRequirementStore) Create(ctx context.Context, r *model.Requirement) error {
	return m.CreateFunc(ctx, r)
}
func (m *MockRequirementStore) GetByID(ctx context.Context, id string) (*model.Requirement, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockRequirementStore) Update(ctx context.Context, r *model.Requirement) error {
	return m.UpdateFunc(ctx, r)
}
func (m *MockRequirementStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockRequirementStore) ListByProductID(ctx context.Context, productID string, filter store.RequirementFilter) ([]*model.Requirement, error) {
	return m.ListByProductIDFunc(ctx, productID, filter)
}
func (m *MockRequirementStore) ListByParentID(ctx context.Context, parentID string) ([]*model.Requirement, error) {
	return m.ListByParentIDFunc(ctx, parentID)
}
func (m *MockRequirementStore) ListBySprintID(ctx context.Context, sprintID string) ([]*model.Requirement, error) {
	return m.ListBySprintIDFunc(ctx, sprintID)
}
func (m *MockRequirementStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Requirement, error) {
	if m.ListByMilestoneIDFunc == nil {
		return nil, nil
	}
	return m.ListByMilestoneIDFunc(ctx, milestoneID)
}
func (m *MockRequirementStore) NextSequenceNumber(ctx context.Context, productID string) (int, error) {
	if m.NextSequenceNumberFunc != nil {
		return m.NextSequenceNumberFunc(ctx, productID)
	}
	return 1, nil
}
func (m *MockRequirementStore) Assign(ctx context.Context, a *model.Assignment) error {
	return m.AssignFunc(ctx, a)
}
func (m *MockRequirementStore) Unassign(ctx context.Context, requirementID, userID string) error {
	return m.UnassignFunc(ctx, requirementID, userID)
}
func (m *MockRequirementStore) ListAssignees(ctx context.Context, requirementID string) ([]*model.Assignment, error) {
	return m.ListAssigneesFunc(ctx, requirementID)
}

// MockDefectStore implements store.DefectStore with configurable function fields.
type MockDefectStore struct {
	CreateFunc            func(ctx context.Context, d *model.Defect) error
	GetByIDFunc           func(ctx context.Context, id string) (*model.Defect, error)
	UpdateFunc            func(ctx context.Context, d *model.Defect) error
	DeleteFunc            func(ctx context.Context, id string) error
	ListByProductIDFunc   func(ctx context.Context, productID string, filter store.DefectFilter) ([]*model.Defect, error)
	ListBySprintIDFunc    func(ctx context.Context, sprintID string) ([]*model.Defect, error)
	ListByMilestoneIDFunc func(ctx context.Context, milestoneID string) ([]*model.Defect, error)
	AssignFunc            func(ctx context.Context, a *model.Assignment) error
	UnassignFunc          func(ctx context.Context, defectID, userID string) error
	ListAssigneesFunc     func(ctx context.Context, defectID string) ([]*model.Assignment, error)
	AddLinkFunc           func(ctx context.Context, link *model.DefectLink) error
	RemoveLinkFunc        func(ctx context.Context, id string) error
	ListLinksFunc         func(ctx context.Context, defectID string) ([]*model.DefectLink, error)
	ListLinksByTargetFunc func(ctx context.Context, targetType, targetID string) ([]*model.DefectLink, error)
}

func (m *MockDefectStore) Create(ctx context.Context, d *model.Defect) error {
	return m.CreateFunc(ctx, d)
}
func (m *MockDefectStore) GetByID(ctx context.Context, id string) (*model.Defect, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockDefectStore) Update(ctx context.Context, d *model.Defect) error {
	return m.UpdateFunc(ctx, d)
}
func (m *MockDefectStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockDefectStore) ListByProductID(ctx context.Context, productID string, filter store.DefectFilter) ([]*model.Defect, error) {
	return m.ListByProductIDFunc(ctx, productID, filter)
}
func (m *MockDefectStore) ListBySprintID(ctx context.Context, sprintID string) ([]*model.Defect, error) {
	return m.ListBySprintIDFunc(ctx, sprintID)
}
func (m *MockDefectStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Defect, error) {
	if m.ListByMilestoneIDFunc == nil {
		return nil, nil
	}
	return m.ListByMilestoneIDFunc(ctx, milestoneID)
}
func (m *MockDefectStore) Assign(ctx context.Context, a *model.Assignment) error {
	return m.AssignFunc(ctx, a)
}
func (m *MockDefectStore) Unassign(ctx context.Context, defectID, userID string) error {
	return m.UnassignFunc(ctx, defectID, userID)
}
func (m *MockDefectStore) ListAssignees(ctx context.Context, defectID string) ([]*model.Assignment, error) {
	return m.ListAssigneesFunc(ctx, defectID)
}
func (m *MockDefectStore) AddLink(ctx context.Context, link *model.DefectLink) error {
	return m.AddLinkFunc(ctx, link)
}
func (m *MockDefectStore) RemoveLink(ctx context.Context, id string) error {
	return m.RemoveLinkFunc(ctx, id)
}
func (m *MockDefectStore) ListLinks(ctx context.Context, defectID string) ([]*model.DefectLink, error) {
	return m.ListLinksFunc(ctx, defectID)
}
func (m *MockDefectStore) ListLinksByTarget(ctx context.Context, targetType, targetID string) ([]*model.DefectLink, error) {
	if m.ListLinksByTargetFunc == nil {
		return nil, nil
	}
	return m.ListLinksByTargetFunc(ctx, targetType, targetID)
}

// MockCommentStore implements store.CommentStore with configurable function fields.
type MockCommentStore struct {
	CreateFunc        func(ctx context.Context, c *model.Comment) error
	GetByIDFunc       func(ctx context.Context, id string) (*model.Comment, error)
	UpdateFunc        func(ctx context.Context, c *model.Comment) error
	DeleteFunc        func(ctx context.Context, id string) error
	DeleteByOwnerFunc func(ctx context.Context, productID, ownerType, ownerID string) error
	ListByOwnerFunc   func(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Comment, error)
}

func (m *MockCommentStore) Create(ctx context.Context, c *model.Comment) error {
	return m.CreateFunc(ctx, c)
}
func (m *MockCommentStore) GetByID(ctx context.Context, id string) (*model.Comment, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockCommentStore) Update(ctx context.Context, c *model.Comment) error {
	return m.UpdateFunc(ctx, c)
}
func (m *MockCommentStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockCommentStore) DeleteByOwner(ctx context.Context, productID, ownerType, ownerID string) error {
	return m.DeleteByOwnerFunc(ctx, productID, ownerType, ownerID)
}
func (m *MockCommentStore) ListByOwner(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Comment, error) {
	return m.ListByOwnerFunc(ctx, productID, ownerType, ownerID)
}

// MockAttachmentStore implements store.AttachmentStore with configurable function fields.
type MockAttachmentStore struct {
	CreateFunc        func(ctx context.Context, a *model.Attachment) error
	GetByIDFunc       func(ctx context.Context, id string) (*model.Attachment, error)
	DeleteFunc        func(ctx context.Context, id string) error
	DeleteByOwnerFunc func(ctx context.Context, productID, ownerType, ownerID string) error
	ListByOwnerFunc   func(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Attachment, error)
}

func (m *MockAttachmentStore) Create(ctx context.Context, a *model.Attachment) error {
	return m.CreateFunc(ctx, a)
}
func (m *MockAttachmentStore) GetByID(ctx context.Context, id string) (*model.Attachment, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockAttachmentStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockAttachmentStore) DeleteByOwner(ctx context.Context, productID, ownerType, ownerID string) error {
	return m.DeleteByOwnerFunc(ctx, productID, ownerType, ownerID)
}
func (m *MockAttachmentStore) ListByOwner(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Attachment, error) {
	return m.ListByOwnerFunc(ctx, productID, ownerType, ownerID)
}

// MockTestCaseStore implements store.TestCaseStore with configurable function fields.
type MockTestCaseStore struct {
	CreateFunc                         func(ctx context.Context, tc *model.TestCase) error
	GetByIDFunc                        func(ctx context.Context, id string) (*model.TestCase, error)
	UpdateFunc                         func(ctx context.Context, tc *model.TestCase) error
	DeleteFunc                         func(ctx context.Context, id string) error
	ListByProductIDFunc                func(ctx context.Context, productID string) ([]*model.TestCase, error)
	ListByCapabilityIDFunc             func(ctx context.Context, capabilityID string) ([]*model.TestCase, error)
	ListByComponentIDFunc              func(ctx context.Context, componentID string) ([]*model.TestCase, error)
	LinkRequirementFunc                func(ctx context.Context, link *model.TestCaseRequirement) error
	UnlinkRequirementFunc              func(ctx context.Context, testCaseID, requirementID string) error
	ListRequirementLinksFunc           func(ctx context.Context, testCaseID string) ([]*model.TestCaseRequirement, error)
	ListTestCaseLinksByRequirementFunc func(ctx context.Context, requirementID string) ([]*model.TestCaseRequirement, error)
	AddEnvironmentFunc                 func(ctx context.Context, id, testCaseID, environmentID string) error
	RemoveEnvironmentFunc              func(ctx context.Context, testCaseID, environmentID string) error
	ListEnvironmentsFunc               func(ctx context.Context, testCaseID string) ([]string, error)
}

func (m *MockTestCaseStore) Create(ctx context.Context, tc *model.TestCase) error {
	return m.CreateFunc(ctx, tc)
}
func (m *MockTestCaseStore) GetByID(ctx context.Context, id string) (*model.TestCase, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockTestCaseStore) Update(ctx context.Context, tc *model.TestCase) error {
	return m.UpdateFunc(ctx, tc)
}
func (m *MockTestCaseStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockTestCaseStore) ListByProductID(ctx context.Context, productID string) ([]*model.TestCase, error) {
	return m.ListByProductIDFunc(ctx, productID)
}
func (m *MockTestCaseStore) ListByCapabilityID(ctx context.Context, capabilityID string) ([]*model.TestCase, error) {
	if m.ListByCapabilityIDFunc == nil {
		return nil, nil
	}
	return m.ListByCapabilityIDFunc(ctx, capabilityID)
}
func (m *MockTestCaseStore) ListByComponentID(ctx context.Context, componentID string) ([]*model.TestCase, error) {
	if m.ListByComponentIDFunc == nil {
		return nil, nil
	}
	return m.ListByComponentIDFunc(ctx, componentID)
}
func (m *MockTestCaseStore) LinkRequirement(ctx context.Context, link *model.TestCaseRequirement) error {
	return m.LinkRequirementFunc(ctx, link)
}
func (m *MockTestCaseStore) UnlinkRequirement(ctx context.Context, testCaseID, requirementID string) error {
	return m.UnlinkRequirementFunc(ctx, testCaseID, requirementID)
}
func (m *MockTestCaseStore) ListRequirementLinks(ctx context.Context, testCaseID string) ([]*model.TestCaseRequirement, error) {
	return m.ListRequirementLinksFunc(ctx, testCaseID)
}
func (m *MockTestCaseStore) ListTestCaseLinksByRequirement(ctx context.Context, requirementID string) ([]*model.TestCaseRequirement, error) {
	if m.ListTestCaseLinksByRequirementFunc == nil {
		return nil, nil
	}
	return m.ListTestCaseLinksByRequirementFunc(ctx, requirementID)
}
func (m *MockTestCaseStore) AddEnvironment(ctx context.Context, id, testCaseID, environmentID string) error {
	return m.AddEnvironmentFunc(ctx, id, testCaseID, environmentID)
}
func (m *MockTestCaseStore) RemoveEnvironment(ctx context.Context, testCaseID, environmentID string) error {
	return m.RemoveEnvironmentFunc(ctx, testCaseID, environmentID)
}
func (m *MockTestCaseStore) ListEnvironments(ctx context.Context, testCaseID string) ([]string, error) {
	return m.ListEnvironmentsFunc(ctx, testCaseID)
}

// MockTestingCampaignStore implements store.TestingCampaignStore with configurable function fields.
type MockTestingCampaignStore struct {
	CreateFunc          func(ctx context.Context, tc *model.TestingCampaign) error
	GetByIDFunc         func(ctx context.Context, id string) (*model.TestingCampaign, error)
	UpdateFunc          func(ctx context.Context, tc *model.TestingCampaign) error
	DeleteFunc          func(ctx context.Context, id string) error
	ListByProductIDFunc func(ctx context.Context, productID string) ([]*model.TestingCampaign, error)
	// Added later; nil-guarded so existing tests need no update.
	GetActiveByProductIDFunc func(ctx context.Context, productID string) (*model.TestingCampaign, error)
}

func (m *MockTestingCampaignStore) Create(ctx context.Context, tc *model.TestingCampaign) error {
	return m.CreateFunc(ctx, tc)
}
func (m *MockTestingCampaignStore) GetByID(ctx context.Context, id string) (*model.TestingCampaign, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockTestingCampaignStore) Update(ctx context.Context, tc *model.TestingCampaign) error {
	return m.UpdateFunc(ctx, tc)
}
func (m *MockTestingCampaignStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockTestingCampaignStore) ListByProductID(ctx context.Context, productID string) ([]*model.TestingCampaign, error) {
	return m.ListByProductIDFunc(ctx, productID)
}
func (m *MockTestingCampaignStore) GetActiveByProductID(ctx context.Context, productID string) (*model.TestingCampaign, error) {
	if m.GetActiveByProductIDFunc == nil {
		return nil, nil
	}
	return m.GetActiveByProductIDFunc(ctx, productID)
}

// MockCampaignResultStore implements store.CampaignResultStore with configurable function fields.
type MockCampaignResultStore struct {
	CreateFunc                   func(ctx context.Context, cr *model.CampaignResult) error
	UpdateFunc                   func(ctx context.Context, cr *model.CampaignResult) error
	GetByCampaignAndTestCaseFunc func(ctx context.Context, campaignID, testCaseID, environmentID string) (*model.CampaignResult, error)
	ListByCampaignIDFunc         func(ctx context.Context, campaignID string) ([]*model.CampaignResult, error)
	// Added later; nil-guarded so existing tests need no update.
	CreateBatchFunc                 func(ctx context.Context, results []*model.CampaignResult) error
	GetByIDFunc                     func(ctx context.Context, id string) (*model.CampaignResult, error)
	ListByTestCaseIDFunc            func(ctx context.Context, testCaseID string) ([]*model.CampaignResult, error)
	ListByDefectIDFunc              func(ctx context.Context, defectID string) ([]*model.CampaignResult, error)
	DeleteByCampaignAndTestCaseFunc func(ctx context.Context, campaignID, testCaseID string) error
	CountsByProductIDFunc           func(ctx context.Context, productID string) (map[string]map[string]int, error)
}

func (m *MockCampaignResultStore) Create(ctx context.Context, cr *model.CampaignResult) error {
	return m.CreateFunc(ctx, cr)
}
func (m *MockCampaignResultStore) Update(ctx context.Context, cr *model.CampaignResult) error {
	return m.UpdateFunc(ctx, cr)
}
func (m *MockCampaignResultStore) GetByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID, environmentID string) (*model.CampaignResult, error) {
	return m.GetByCampaignAndTestCaseFunc(ctx, campaignID, testCaseID, environmentID)
}
func (m *MockCampaignResultStore) ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResult, error) {
	return m.ListByCampaignIDFunc(ctx, campaignID)
}
func (m *MockCampaignResultStore) CreateBatch(ctx context.Context, results []*model.CampaignResult) error {
	if m.CreateBatchFunc == nil {
		return nil
	}
	return m.CreateBatchFunc(ctx, results)
}
func (m *MockCampaignResultStore) GetByID(ctx context.Context, id string) (*model.CampaignResult, error) {
	if m.GetByIDFunc == nil {
		return nil, nil
	}
	return m.GetByIDFunc(ctx, id)
}
func (m *MockCampaignResultStore) ListByTestCaseID(ctx context.Context, testCaseID string) ([]*model.CampaignResult, error) {
	if m.ListByTestCaseIDFunc == nil {
		return nil, nil
	}
	return m.ListByTestCaseIDFunc(ctx, testCaseID)
}
func (m *MockCampaignResultStore) ListByDefectID(ctx context.Context, defectID string) ([]*model.CampaignResult, error) {
	if m.ListByDefectIDFunc == nil {
		return nil, nil
	}
	return m.ListByDefectIDFunc(ctx, defectID)
}
func (m *MockCampaignResultStore) DeleteByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID string) error {
	if m.DeleteByCampaignAndTestCaseFunc == nil {
		return nil
	}
	return m.DeleteByCampaignAndTestCaseFunc(ctx, campaignID, testCaseID)
}
func (m *MockCampaignResultStore) CountsByProductID(ctx context.Context, productID string) (map[string]map[string]int, error) {
	if m.CountsByProductIDFunc == nil {
		return nil, nil
	}
	return m.CountsByProductIDFunc(ctx, productID)
}

// MockCampaignResultEventStore implements store.CampaignResultEventStore with configurable function fields.
type MockCampaignResultEventStore struct {
	CreateFunc           func(ctx context.Context, ev *model.CampaignResultEvent) error
	ListByResultIDFunc   func(ctx context.Context, resultID string) ([]*model.CampaignResultEvent, error)
	ListByCampaignIDFunc func(ctx context.Context, campaignID string) ([]*model.CampaignResultEvent, error)
}

func (m *MockCampaignResultEventStore) Create(ctx context.Context, ev *model.CampaignResultEvent) error {
	if m.CreateFunc == nil {
		return nil
	}
	return m.CreateFunc(ctx, ev)
}
func (m *MockCampaignResultEventStore) ListByResultID(ctx context.Context, resultID string) ([]*model.CampaignResultEvent, error) {
	if m.ListByResultIDFunc == nil {
		return nil, nil
	}
	return m.ListByResultIDFunc(ctx, resultID)
}
func (m *MockCampaignResultEventStore) ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResultEvent, error) {
	if m.ListByCampaignIDFunc == nil {
		return nil, nil
	}
	return m.ListByCampaignIDFunc(ctx, campaignID)
}

// MockCategoryStore implements store.CategoryStore with configurable function fields.
type MockCategoryStore struct {
	CreateFunc                func(ctx context.Context, cat *model.Category) error
	GetByIDFunc               func(ctx context.Context, id string) (*model.Category, error)
	UpdateFunc                func(ctx context.Context, cat *model.Category) error
	DeleteFunc                func(ctx context.Context, id string) error
	ListByProductIDFunc       func(ctx context.Context, productID, entityType string) ([]*model.Category, error)
	GetByProductAndNameFunc   func(ctx context.Context, productID, entityType, name string) (*model.Category, error)
	GetByProductAndPrefixFunc func(ctx context.Context, productID, entityType, prefix string) (*model.Category, error)
}

func (m *MockCategoryStore) Create(ctx context.Context, cat *model.Category) error {
	return m.CreateFunc(ctx, cat)
}
func (m *MockCategoryStore) GetByID(ctx context.Context, id string) (*model.Category, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *MockCategoryStore) Update(ctx context.Context, cat *model.Category) error {
	return m.UpdateFunc(ctx, cat)
}
func (m *MockCategoryStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
func (m *MockCategoryStore) ListByProductID(ctx context.Context, productID, entityType string) ([]*model.Category, error) {
	if m.ListByProductIDFunc != nil {
		return m.ListByProductIDFunc(ctx, productID, entityType)
	}
	return nil, nil
}
func (m *MockCategoryStore) GetByProductAndName(ctx context.Context, productID, entityType, name string) (*model.Category, error) {
	return m.GetByProductAndNameFunc(ctx, productID, entityType, name)
}
func (m *MockCategoryStore) GetByProductAndPrefix(ctx context.Context, productID, entityType, prefix string) (*model.Category, error) {
	return m.GetByProductAndPrefixFunc(ctx, productID, entityType, prefix)
}
