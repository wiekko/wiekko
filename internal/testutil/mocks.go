package testutil

import (
	"context"

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
