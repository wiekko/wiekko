// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"wiekko.net/internal/auth"
	"wiekko.net/internal/model"
	"wiekko.net/internal/store"
	"wiekko.net/internal/testutil"
)

func settingsGetFunc(settings map[string]string) func(context.Context, string) (string, error) {
	return func(_ context.Context, key string) (string, error) {
		if v, ok := settings[key]; ok {
			return v, nil
		}
		return "", errors.New("not found")
	}
}

func noopProvider() *testutil.MockAuthProvider {
	return &testutil.MockAuthProvider{
		ValidatePasswordFunc:       func(_ string, _ auth.PasswordPolicy) error { return nil },
		HashPasswordFunc:           func(_ string) (string, error) { return "hashed", nil },
		VerifyPasswordFunc:         func(_, _ string) error { return nil },
		SupportsPasswordChangeFunc: func() bool { return true },
	}
}

// notFoundUser mimics the user store's "no such user" error.
func notFoundUser(context.Context, string) (*model.User, error) {
	return nil, fmt.Errorf("user not found: %w", sql.ErrNoRows)
}

func newUserService(users *testutil.MockUserStore, sessions *testutil.MockSessionStore, settings *testutil.MockSettingStore, invitations *testutil.MockInvitationStore, provider *testutil.MockAuthProvider) *UserService {
	// New accounts are checked against existing ones; default to "none".
	if users != nil && users.GetByUsernameFunc == nil {
		users.GetByUsernameFunc = notFoundUser
	}
	if users != nil && users.GetByEmailFunc == nil {
		users.GetByEmailFunc = notFoundUser
	}
	// Locking and deleting look the account up first; default to an ordinary user.
	if users != nil && users.GetByIDFunc == nil {
		users.GetByIDFunc = func(_ context.Context, id string) (*model.User, error) {
			return testutil.MakeUser(func(u *model.User) { u.ID = id }), nil
		}
	}
	svc := &UserService{
		users:       users,
		sessions:    sessions,
		settings:    settings,
		invitations: invitations,
	}
	// A nil *MockAuthProvider in the interface would not compare equal to nil.
	if provider != nil {
		svc.provider = provider
	}
	return svc
}

func TestRegister(t *testing.T) {
	input := RegisterInput{
		Username:    "alice",
		Email:       "alice@example.com",
		DisplayName: "Alice",
		Password:    "Str0ngP@ss!",
	}

	t.Run("self-reg enabled, moderation off", func(t *testing.T) {
		var created *model.User
		svc := newUserService(
			&testutil.MockUserStore{
				CreateFunc: func(_ context.Context, u *model.User) error { created = u; return nil },
			}, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{"self_registration_enabled": "true"}),
			}, nil, noopProvider(),
		)
		user, err := svc.Register(context.Background(), input)
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		if user.Status != model.UserStatusActive {
			t.Fatalf("status = %q, want %q", user.Status, model.UserStatusActive)
		}
		if created == nil {
			t.Fatal("user was not created")
		}
	})

	t.Run("self-reg enabled, moderation on", func(t *testing.T) {
		svc := newUserService(
			&testutil.MockUserStore{
				CreateFunc: func(_ context.Context, _ *model.User) error { return nil },
			}, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{
					"self_registration_enabled": "true",
					"moderation_enabled":        "true",
				}),
			}, nil, noopProvider(),
		)
		user, err := svc.Register(context.Background(), input)
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		if user.Status != model.UserStatusPendingApproval {
			t.Fatalf("status = %q, want %q", user.Status, model.UserStatusPendingApproval)
		}
	})

	t.Run("self-reg disabled", func(t *testing.T) {
		svc := newUserService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			}, nil, noopProvider(),
		)
		_, err := svc.Register(context.Background(), input)
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "self-registration is disabled" {
			t.Fatalf("err = %v, want 'self-registration is disabled'", err)
		}
	})

	t.Run("password fails policy", func(t *testing.T) {
		provider := noopProvider()
		provider.ValidatePasswordFunc = func(_ string, _ auth.PasswordPolicy) error {
			return errors.New("password too short")
		}
		svc := newUserService(&testutil.MockUserStore{}, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{"self_registration_enabled": "true"}),
			}, nil, provider,
		)
		_, err := svc.Register(context.Background(), input)
		if err == nil || err.Error() != "password too short" {
			t.Fatalf("err = %v, want 'password too short'", err)
		}
	})
}

func TestAcceptInvitation(t *testing.T) {
	rawToken := "somerawtoken"
	tokenHash := hashPATToken(rawToken)
	input := RegisterInput{
		// The address the invitation (testutil.MakeInvitation) was sent to.
		Username: "bob", Email: "invite@example.com", DisplayName: "Bob", Password: "Str0ng!",
	}

	t.Run("valid invitation", func(t *testing.T) {
		inv := testutil.MakeInvitation(func(i *model.Invitation) { i.TokenHash = tokenHash })
		accepted := false
		svc := newUserService(
			&testutil.MockUserStore{
				CreateFunc: func(_ context.Context, _ *model.User) error { return nil },
			}, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			},
			&testutil.MockInvitationStore{
				GetByTokenHashFunc: func(_ context.Context, h string) (*model.Invitation, error) {
					if h == tokenHash {
						return inv, nil
					}
					return nil, errors.New("not found")
				},
				MarkAcceptedFunc: func(_ context.Context, _, _ string) error {
					accepted = true
					return nil
				},
			}, noopProvider(),
		)
		user, err := svc.AcceptInvitation(context.Background(), rawToken, input)
		if err != nil {
			t.Fatalf("AcceptInvitation: %v", err)
		}
		if user.Status != model.UserStatusActive {
			t.Fatalf("status = %q, want %q", user.Status, model.UserStatusActive)
		}
		if !accepted {
			t.Fatal("invitation was not marked accepted")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		svc := newUserService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			},
			&testutil.MockInvitationStore{
				GetByTokenHashFunc: func(_ context.Context, _ string) (*model.Invitation, error) {
					return nil, errors.New("not found")
				},
			}, noopProvider(),
		)
		_, err := svc.AcceptInvitation(context.Background(), "bad", input)
		if err == nil || err.Error() != "invalid invitation token" {
			t.Fatalf("err = %v, want 'invalid invitation token'", err)
		}
	})

	t.Run("already accepted", func(t *testing.T) {
		inv := testutil.MakeInvitation(func(i *model.Invitation) {
			i.TokenHash = tokenHash
			i.AcceptedAt = testutil.PastRFC3339(time.Hour)
		})
		svc := newUserService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			},
			&testutil.MockInvitationStore{
				GetByTokenHashFunc: func(_ context.Context, _ string) (*model.Invitation, error) { return inv, nil },
			}, noopProvider(),
		)
		_, err := svc.AcceptInvitation(context.Background(), rawToken, input)
		if err == nil || err.Error() != "invitation already accepted" {
			t.Fatalf("err = %v, want 'invitation already accepted'", err)
		}
	})

	t.Run("expired invitation", func(t *testing.T) {
		inv := testutil.MakeInvitation(func(i *model.Invitation) {
			i.TokenHash = tokenHash
			i.ExpiresAt = testutil.PastRFC3339(time.Hour)
		})
		svc := newUserService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			},
			&testutil.MockInvitationStore{
				GetByTokenHashFunc: func(_ context.Context, _ string) (*model.Invitation, error) { return inv, nil },
			}, noopProvider(),
		)
		_, err := svc.AcceptInvitation(context.Background(), rawToken, input)
		if err == nil || err.Error() != "invitation expired" {
			t.Fatalf("err = %v, want 'invitation expired'", err)
		}
	})
}

func TestUpdateProfile(t *testing.T) {
	user := testutil.MakeUser()

	t.Run("update email only", func(t *testing.T) {
		var updated *model.User
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) {
					u := *user // copy
					return &u, nil
				},
				UpdateFunc: func(_ context.Context, u *model.User) error { updated = u; return nil },
			}, nil, nil, nil, nil,
		)
		email := "new@example.com"
		result, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileInput{Email: &email})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if result.Email != "new@example.com" {
			t.Fatalf("Email = %q, want %q", result.Email, "new@example.com")
		}
		if updated.DisplayName != user.DisplayName {
			t.Fatalf("DisplayName changed unexpectedly")
		}
	})

	t.Run("update display name only", func(t *testing.T) {
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) {
					u := *user
					return &u, nil
				},
				UpdateFunc: func(_ context.Context, _ *model.User) error { return nil },
			}, nil, nil, nil, nil,
		)
		name := "New Name"
		result, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileInput{DisplayName: &name})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if result.DisplayName != "New Name" {
			t.Fatalf("DisplayName = %q, want %q", result.DisplayName, "New Name")
		}
		if result.Email != user.Email {
			t.Fatalf("Email changed unexpectedly")
		}
	})
}

func TestChangePassword(t *testing.T) {
	user := testutil.MakeUser(func(u *model.User) { u.PasswordHash = "oldhash" })

	t.Run("success", func(t *testing.T) {
		var updatedHash, keptSession string
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc:        func(_ context.Context, _ string) (*model.User, error) { return user, nil },
				UpdatePasswordFunc: func(_ context.Context, _, h string) error { updatedHash = h; return nil },
			},
			&testutil.MockSessionStore{
				DeleteByUserIDExceptFunc: func(_ context.Context, userID, keepID string) error {
					if userID != user.ID {
						t.Errorf("sessions deleted for %q, want %q", userID, user.ID)
					}
					keptSession = keepID
					return nil
				},
			},
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			}, nil, noopProvider(),
		)
		if err := svc.ChangePassword(context.Background(), user.ID, "old", "new", "current-session"); err != nil {
			t.Fatalf("ChangePassword: %v", err)
		}
		if updatedHash != "hashed" {
			t.Fatalf("hash = %q, want %q", updatedHash, "hashed")
		}
		if keptSession != "current-session" {
			t.Fatalf("kept session = %q, want the current one", keptSession)
		}
	})

	t.Run("without a session ends all sessions", func(t *testing.T) {
		var ended string
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc:        func(_ context.Context, _ string) (*model.User, error) { return user, nil },
				UpdatePasswordFunc: func(_ context.Context, _, _ string) error { return nil },
			},
			&testutil.MockSessionStore{
				DeleteByUserIDFunc: func(_ context.Context, userID string) error { ended = userID; return nil },
			},
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			}, nil, noopProvider(),
		)
		if err := svc.ChangePassword(context.Background(), user.ID, "old", "new", ""); err != nil {
			t.Fatalf("ChangePassword: %v", err)
		}
		if ended != user.ID {
			t.Fatalf("sessions ended for %q, want %q", ended, user.ID)
		}
	})

	t.Run("wrong old password", func(t *testing.T) {
		provider := noopProvider()
		provider.VerifyPasswordFunc = func(_, _ string) error { return errors.New("mismatch") }
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return user, nil },
			}, nil, nil, nil, provider,
		)
		err := svc.ChangePassword(context.Background(), user.ID, "wrong", "new", "")
		if err == nil || err.Error() != "current password is incorrect" {
			t.Fatalf("err = %v, want 'current password is incorrect'", err)
		}
	})

	t.Run("new password fails policy", func(t *testing.T) {
		provider := noopProvider()
		provider.ValidatePasswordFunc = func(_ string, _ auth.PasswordPolicy) error {
			return errors.New("too weak")
		}
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return user, nil },
			}, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			}, nil, provider,
		)
		err := svc.ChangePassword(context.Background(), user.ID, "old", "weak", "")
		if err == nil || err.Error() != "too weak" {
			t.Fatalf("err = %v, want 'too weak'", err)
		}
	})
}

func TestRequestDeletion(t *testing.T) {
	t.Run("moderation enabled", func(t *testing.T) {
		var updatedStatus string
		svc := newUserService(
			&testutil.MockUserStore{
				UpdateStatusFunc: func(_ context.Context, _, s string) error { updatedStatus = s; return nil },
			}, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{"moderation_enabled": "true"}),
			}, nil, nil,
		)
		if err := svc.RequestDeletion(context.Background(), "u1", ""); err != nil {
			t.Fatalf("RequestDeletion: %v", err)
		}
		if updatedStatus != model.UserStatusDeletionRequested {
			t.Fatalf("status = %q, want %q", updatedStatus, model.UserStatusDeletionRequested)
		}
	})

	t.Run("moderation disabled", func(t *testing.T) {
		deleted := false
		svc := newUserService(
			&testutil.MockUserStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			}, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{}),
			}, nil, nil,
		)
		if err := svc.RequestDeletion(context.Background(), "u1", ""); err != nil {
			t.Fatalf("RequestDeletion: %v", err)
		}
		if !deleted {
			t.Fatal("user was not deleted")
		}
	})
}

func TestAdminApproveUser(t *testing.T) {
	t.Run("pending user approved", func(t *testing.T) {
		user := testutil.MakeUser(func(u *model.User) { u.Status = model.UserStatusPendingApproval })
		var updatedStatus string
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc:      func(_ context.Context, _ string) (*model.User, error) { return user, nil },
				UpdateStatusFunc: func(_ context.Context, _, s string) error { updatedStatus = s; return nil },
			}, nil, nil, nil, nil,
		)
		if err := svc.AdminApproveUser(context.Background(), user.ID); err != nil {
			t.Fatalf("AdminApproveUser: %v", err)
		}
		if updatedStatus != model.UserStatusActive {
			t.Fatalf("status = %q, want %q", updatedStatus, model.UserStatusActive)
		}
	})

	t.Run("already active", func(t *testing.T) {
		user := testutil.MakeUser()
		svc := newUserService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return user, nil },
			}, nil, nil, nil, nil,
		)
		err := svc.AdminApproveUser(context.Background(), user.ID)
		if err == nil || err.Error() != "user is not pending approval" {
			t.Fatalf("err = %v, want 'user is not pending approval'", err)
		}
	})
}

func TestLoadPasswordPolicy(t *testing.T) {
	t.Run("defaults when no settings", func(t *testing.T) {
		svc := newUserService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: func(_ context.Context, _ string) (string, error) { return "", errors.New("not found") },
			}, nil, nil,
		)
		policy, err := svc.loadPasswordPolicy(context.Background())
		if err != nil {
			t.Fatalf("loadPasswordPolicy: %v", err)
		}
		if policy.MinLength != 10 {
			t.Errorf("MinLength = %d, want 10", policy.MinLength)
		}
		if !policy.RequireUppercase {
			t.Error("RequireUppercase should be true by default")
		}
		if !policy.RequireLowercase {
			t.Error("RequireLowercase should be true by default")
		}
		if !policy.RequireDigit {
			t.Error("RequireDigit should be true by default")
		}
		if policy.RequireSpecial {
			t.Error("RequireSpecial should be false by default")
		}
	})

	t.Run("all settings override defaults", func(t *testing.T) {
		svc := newUserService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{
					"password_min_length":        "8",
					"password_require_uppercase": "false",
					"password_require_lowercase": "false",
					"password_require_digit":     "false",
					"password_require_special":   "true",
				}),
			}, nil, nil,
		)
		policy, err := svc.loadPasswordPolicy(context.Background())
		if err != nil {
			t.Fatalf("loadPasswordPolicy: %v", err)
		}
		if policy.MinLength != 8 {
			t.Errorf("MinLength = %d, want 8", policy.MinLength)
		}
		if policy.RequireUppercase {
			t.Error("RequireUppercase should be false")
		}
		if policy.RequireLowercase {
			t.Error("RequireLowercase should be false")
		}
		if policy.RequireDigit {
			t.Error("RequireDigit should be false")
		}
		if !policy.RequireSpecial {
			t.Error("RequireSpecial should be true")
		}
	})

	t.Run("non-numeric min_length keeps default", func(t *testing.T) {
		svc := newUserService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: settingsGetFunc(map[string]string{"password_min_length": "abc"}),
			}, nil, nil,
		)
		policy, _ := svc.loadPasswordPolicy(context.Background())
		if policy.MinLength != 10 {
			t.Errorf("MinLength = %d, want 10", policy.MinLength)
		}
	})
}

func TestAdminLockUser(t *testing.T) {
	var updatedStatus string
	svc := newUserService(
		&testutil.MockUserStore{
			UpdateStatusFunc: func(_ context.Context, _, s string) error { updatedStatus = s; return nil },
		}, nil, nil, nil, nil,
	)
	if err := svc.AdminLockUser(context.Background(), "u1"); err != nil {
		t.Fatalf("AdminLockUser: %v", err)
	}
	if updatedStatus != model.UserStatusLocked {
		t.Fatalf("status = %q, want %q", updatedStatus, model.UserStatusLocked)
	}
}

func TestAdminUnlockUser(t *testing.T) {
	var updatedStatus string
	svc := newUserService(
		&testutil.MockUserStore{
			UpdateStatusFunc: func(_ context.Context, _, s string) error { updatedStatus = s; return nil },
		}, nil, nil, nil, nil,
	)
	if err := svc.AdminUnlockUser(context.Background(), "u1"); err != nil {
		t.Fatalf("AdminUnlockUser: %v", err)
	}
	if updatedStatus != model.UserStatusActive {
		t.Fatalf("status = %q, want %q", updatedStatus, model.UserStatusActive)
	}
}

func TestAdminDeleteUser(t *testing.T) {
	var deletedID string
	svc := newUserService(
		&testutil.MockUserStore{
			DeleteFunc: func(_ context.Context, id string) error { deletedID = id; return nil },
		}, nil, nil, nil, nil,
	)
	if err := svc.AdminDeleteUser(context.Background(), "u1"); err != nil {
		t.Fatalf("AdminDeleteUser: %v", err)
	}
	if deletedID != "u1" {
		t.Fatalf("deleted = %q, want %q", deletedID, "u1")
	}
}

func TestAdminSetPassword(t *testing.T) {
	var updatedHash, ended, kept string
	svc := newUserService(
		&testutil.MockUserStore{
			UpdatePasswordFunc: func(_ context.Context, _, h string) error { updatedHash = h; return nil },
		},
		&testutil.MockSessionStore{
			DeleteByUserIDExceptFunc: func(_ context.Context, userID, keepID string) error {
				ended, kept = userID, keepID
				return nil
			},
		},
		&testutil.MockSettingStore{
			GetFunc: settingsGetFunc(map[string]string{}),
		}, nil, noopProvider(),
	)
	if err := svc.AdminSetPassword(context.Background(), "u1", "NewPass1!", "admin-session"); err != nil {
		t.Fatalf("AdminSetPassword: %v", err)
	}
	if updatedHash != "hashed" {
		t.Fatalf("hash = %q, want %q", updatedHash, "hashed")
	}
	if ended != "u1" || kept != "admin-session" {
		t.Fatalf("sessions ended for %q keeping %q, want u1 keeping admin-session", ended, kept)
	}
}

func TestUserList(t *testing.T) {
	users := []*model.User{testutil.MakeUser()}
	svc := newUserService(
		&testutil.MockUserStore{
			ListFunc: func(_ context.Context, _ store.UserFilter) ([]*model.User, error) { return users, nil },
		}, nil, nil, nil, nil,
	)
	result, err := svc.List(context.Background(), store.UserFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
}

func TestUserGetByID(t *testing.T) {
	user := testutil.MakeUser()
	svc := newUserService(
		&testutil.MockUserStore{
			GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return user, nil },
		}, nil, nil, nil, nil,
	)
	result, err := svc.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if result.ID != user.ID {
		t.Fatalf("ID = %q, want %q", result.ID, user.ID)
	}
}

func TestRegisterRejectsTakenIdentity(t *testing.T) {
	input := RegisterInput{Username: "alice", Email: "alice@example.com", Password: "Str0ngP@ss!"}
	existing := func(context.Context, string) (*model.User, error) { return testutil.MakeUser(), nil }
	settings := &testutil.MockSettingStore{
		GetFunc: settingsGetFunc(map[string]string{"self_registration_enabled": "true"}),
	}

	t.Run("username", func(t *testing.T) {
		svc := newUserService(&testutil.MockUserStore{GetByUsernameFunc: existing}, nil, settings, nil, noopProvider())
		if _, err := svc.Register(context.Background(), input); !errors.Is(err, ErrUsernameTaken) {
			t.Fatalf("err = %v, want ErrUsernameTaken", err)
		}
	})
	t.Run("email", func(t *testing.T) {
		svc := newUserService(&testutil.MockUserStore{GetByEmailFunc: existing}, nil, settings, nil, noopProvider())
		if _, err := svc.Register(context.Background(), input); !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("err = %v, want ErrEmailTaken", err)
		}
	})
	t.Run("missing fields are an input error", func(t *testing.T) {
		svc := newUserService(&testutil.MockUserStore{}, nil, settings, nil, noopProvider())
		_, err := svc.Register(context.Background(), RegisterInput{Username: " ", Email: "a@b.c", Password: "x"})
		var inputErr *AccountInputError
		if !errors.As(err, &inputErr) {
			t.Fatalf("err = %v, want AccountInputError", err)
		}
	})
}

func TestAcceptInvitationProductMembership(t *testing.T) {
	rawToken := "producttoken"
	inv := testutil.MakeInvitation(func(i *model.Invitation) {
		i.TokenHash = hashPATToken(rawToken)
		i.ProductID = "prod-1"
		i.Role = model.RoleTester
	})
	newSvc := func(created *model.User, roles *[]*model.ProductRole) *UserService {
		svc := newUserService(
			&testutil.MockUserStore{
				CreateFunc: func(_ context.Context, u *model.User) error { *created = *u; return nil },
			}, nil,
			&testutil.MockSettingStore{GetFunc: settingsGetFunc(map[string]string{})},
			&testutil.MockInvitationStore{
				GetByTokenHashFunc: func(_ context.Context, _ string) (*model.Invitation, error) { return inv, nil },
				MarkAcceptedFunc:   func(_ context.Context, _, _ string) error { return nil },
			}, noopProvider(),
		)
		svc.productRoles = &testutil.MockProductRoleStore{
			CreateFunc: func(_ context.Context, r *model.ProductRole) error { *roles = append(*roles, r); return nil },
		}
		return svc
	}

	t.Run("grants the invited role", func(t *testing.T) {
		var created model.User
		var roles []*model.ProductRole
		user, err := newSvc(&created, &roles).AcceptInvitation(context.Background(), rawToken,
			RegisterInput{Username: "carol", Password: "Str0ng!"})
		if err != nil {
			t.Fatalf("AcceptInvitation: %v", err)
		}
		if len(roles) != 1 {
			t.Fatalf("roles created = %d, want 1", len(roles))
		}
		if r := roles[0]; r.UserID != user.ID || r.ProductID != "prod-1" || r.Role != model.RoleTester {
			t.Fatalf("role = %+v, want tester in prod-1 for %s", r, user.ID)
		}
		if created.Email != inv.Email {
			t.Fatalf("email = %q, want the invited address %q", created.Email, inv.Email)
		}
		if created.DisplayName != "carol" {
			t.Fatalf("display name = %q, want the username as fallback", created.DisplayName)
		}
	})

	t.Run("rejects another email address", func(t *testing.T) {
		var created model.User
		var roles []*model.ProductRole
		_, err := newSvc(&created, &roles).AcceptInvitation(context.Background(), rawToken,
			RegisterInput{Username: "carol", Email: "other@example.com", Password: "Str0ng!"})
		var inputErr *AccountInputError
		if !errors.As(err, &inputErr) {
			t.Fatalf("err = %v, want AccountInputError", err)
		}
		if len(roles) != 0 || created.ID != "" {
			t.Fatal("nothing should be created")
		}
	})
}
