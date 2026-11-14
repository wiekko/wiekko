package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/access"
	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/auth/totp"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

// twoFactorEnv wires the real stores against a migrated database, so the TOTP
// flows run through the actual SQL (replay guard, recovery codes, challenges)
// rather than mocks. Tests run it on every engine via dbtest.Each.
type twoFactorEnv struct {
	db    *store.DB
	svc   *Services
	user  *model.User
	clock time.Time
}

const testPassword = "Correct-Horse-9"

func newTwoFactorEnv(t *testing.T, driver string, conn *sql.DB) *twoFactorEnv {
	t.Helper()
	dialect := store.Dialect(driver)
	db := store.NewDB(conn, dialect)
	st := store.New(conn, dialect, crypto.TestKeyRing())
	provider := builtin.New(st.User, 1)
	blobs, err := storage.NewFS(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, provider, &config.Config{SessionTTL: time.Hour}, blobs)

	hash, err := provider.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	user := &model.User{
		ID: uuid.NewString(), Username: "alice", Email: "alice@example.com", DisplayName: "Alice",
		PasswordHash: hash, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.User.Create(context.Background(), user); err != nil {
		t.Fatal(err)
	}

	env := &twoFactorEnv{db: db, svc: svc, user: user, clock: time.Now()}
	svc.TwoFactor.now = func() time.Time { return env.clock }
	return env
}

// code returns the TOTP code for the user's current secret at the env clock.
func (e *twoFactorEnv) code(t *testing.T) string {
	t.Helper()
	u, err := e.svc.User.GetByID(context.Background(), e.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := totp.CodeAt(u.TOTPSecret, totp.Step(e.clock))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *twoFactorEnv) advance() { e.clock = e.clock.Add(totp.Period) }

// enable runs the full setup and returns the recovery codes.
func (e *twoFactorEnv) enable(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()
	if _, err := e.svc.TwoFactor.BeginSetup(ctx, e.user.ID); err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}
	codes, err := e.svc.TwoFactor.ConfirmSetup(ctx, e.user.ID, e.code(t))
	if err != nil {
		t.Fatalf("ConfirmSetup: %v", err)
	}
	e.advance()
	return codes
}

func TestTwoFactorSetup(t *testing.T) {
	ctx := context.Background()
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		env := newTwoFactorEnv(t, driver, conn)

		setup, err := env.svc.TwoFactor.BeginSetup(ctx, env.user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if setup.QRCodeSVG == "" || setup.URI == "" {
			t.Fatal("setup data incomplete")
		}

		// The secret must be encrypted at rest.
		var stored string
		if err := env.db.QueryRow(`SELECT totp_secret FROM users WHERE id = ?`, env.user.ID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored == "" || stored == setup.Secret {
			t.Fatalf("totp_secret stored in plaintext or missing: %q", stored)
		}

		// Pending setup does not yet require a code at login.
		res, err := env.svc.Auth.Login(ctx, "alice", testPassword, "", "")
		if err != nil || res.TOTPRequired || res.SessionID == "" {
			t.Fatalf("login during pending setup: res=%+v err=%v", res, err)
		}

		if _, err := env.svc.TwoFactor.ConfirmSetup(ctx, env.user.ID, "000000"); !errors.Is(err, ErrInvalidTOTPCode) {
			t.Fatalf("wrong confirm code: err = %v", err)
		}
		codes, err := env.svc.TwoFactor.ConfirmSetup(ctx, env.user.ID, env.code(t))
		if err != nil {
			t.Fatal(err)
		}
		if len(codes) != totp.RecoveryCodeCount {
			t.Fatalf("got %d recovery codes", len(codes))
		}
		if n, _ := env.svc.TwoFactor.RecoveryCodesRemaining(ctx, env.user.ID); n != totp.RecoveryCodeCount {
			t.Fatalf("remaining = %d", n)
		}
		if _, err := env.svc.TwoFactor.BeginSetup(ctx, env.user.ID); !errors.Is(err, ErrTOTPAlreadyEnabled) {
			t.Fatalf("second setup: err = %v", err)
		}
	})
}

func TestTwoFactorLogin(t *testing.T) {
	ctx := context.Background()
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		env := newTwoFactorEnv(t, driver, conn)
		recovery := env.enable(t)

		login := func() *LoginResult {
			t.Helper()
			res, err := env.svc.Auth.Login(ctx, "alice", testPassword, "", "")
			if err != nil {
				t.Fatalf("Login: %v", err)
			}
			if !res.TOTPRequired || res.SessionID != "" || res.ChallengeToken == "" {
				t.Fatalf("expected a challenge, got %+v", res)
			}
			return res
		}

		t.Run("valid code creates session", func(t *testing.T) {
			res := login()
			done, err := env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, env.code(t), "", "")
			if err != nil || done.SessionID == "" {
				t.Fatalf("CompleteTOTPLogin: %v", err)
			}
			if _, _, err := env.svc.Auth.ValidateSession(ctx, done.SessionID); err != nil {
				t.Fatalf("session invalid: %v", err)
			}
			// The challenge is single-use.
			if _, err := env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, env.code(t), "", ""); !errors.Is(err, ErrLoginChallengeInvalid) {
				t.Fatalf("challenge reuse: err = %v", err)
			}
		})

		t.Run("replayed code rejected", func(t *testing.T) {
			res := login()
			// Same step as the previous subtest: already consumed.
			_, err := env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, env.code(t), "", "")
			if !errors.Is(err, ErrTOTPCodeReused) || !errors.Is(err, ErrInvalidTOTPCode) {
				t.Fatalf("replay: err = %v", err)
			}
			env.advance()
			if _, err := env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, env.code(t), "", ""); err != nil {
				t.Fatalf("next step code: %v", err)
			}
		})

		t.Run("recovery code works once", func(t *testing.T) {
			res := login()
			if _, err := env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, recovery[0], "", ""); err != nil {
				t.Fatalf("recovery code: %v", err)
			}
			res = login()
			if _, err := env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, recovery[0], "", ""); !errors.Is(err, ErrInvalidTOTPCode) {
				t.Fatalf("reused recovery code: err = %v", err)
			}
			if n, _ := env.svc.TwoFactor.RecoveryCodesRemaining(ctx, env.user.ID); n != totp.RecoveryCodeCount-1 {
				t.Fatalf("remaining = %d", n)
			}
		})

		t.Run("attempt limit discards challenge", func(t *testing.T) {
			res := login()
			var err error
			for range LoginChallengeMaxAttempts {
				_, err = env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, "000000", "", "")
			}
			if !errors.Is(err, ErrLoginChallengeInvalid) {
				t.Fatalf("after %d wrong codes: err = %v", LoginChallengeMaxAttempts, err)
			}
			env.advance()
			if _, err := env.svc.Auth.CompleteTOTPLogin(ctx, res.ChallengeToken, env.code(t), "", ""); !errors.Is(err, ErrLoginChallengeInvalid) {
				t.Fatalf("challenge usable after limit: err = %v", err)
			}
		})

		t.Run("unknown challenge", func(t *testing.T) {
			if _, err := env.svc.Auth.CompleteTOTPLogin(ctx, "nope", "123456", "", ""); !errors.Is(err, ErrLoginChallengeInvalid) {
				t.Fatalf("err = %v", err)
			}
		})
	})
}

func TestTwoFactorDisableAndReset(t *testing.T) {
	ctx := context.Background()
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		env := newTwoFactorEnv(t, driver, conn)
		env.enable(t)

		if err := env.svc.TwoFactor.Disable(ctx, env.user.ID, "wrong", env.code(t)); !errors.Is(err, ErrIncorrectPassword) {
			t.Fatalf("wrong password: err = %v", err)
		}

		// Enforcement blocks disabling.
		if err := env.svc.Setting.Update(ctx, map[string]string{SettingTwoFactorEnforced: "true"}); err != nil {
			t.Fatal(err)
		}
		if !env.svc.TwoFactor.RequiredFor(ctx, env.user) {
			t.Fatal("RequiredFor() = false")
		}
		if err := env.svc.TwoFactor.Disable(ctx, env.user.ID, testPassword, env.code(t)); !errors.Is(err, ErrTOTPEnforced) {
			t.Fatalf("disable while enforced: err = %v", err)
		}
		if err := env.svc.Setting.Update(ctx, map[string]string{SettingTwoFactorEnforced: "false"}); err != nil {
			t.Fatal(err)
		}

		newCodes, err := env.svc.TwoFactor.RegenerateRecoveryCodes(ctx, env.user.ID, env.code(t))
		if err != nil || len(newCodes) != totp.RecoveryCodeCount {
			t.Fatalf("regenerate: %v", err)
		}
		env.advance()

		if err := env.svc.TwoFactor.Disable(ctx, env.user.ID, testPassword, env.code(t)); err != nil {
			t.Fatalf("Disable: %v", err)
		}
		if n, _ := env.svc.TwoFactor.RecoveryCodesRemaining(ctx, env.user.ID); n != 0 {
			t.Fatalf("recovery codes left after disable: %d", n)
		}
		res, err := env.svc.Auth.Login(ctx, "alice", testPassword, "", "")
		if err != nil || res.TOTPRequired {
			t.Fatalf("login after disable: res=%+v err=%v", res, err)
		}

		// Admin reset, including for a login already waiting on the second step.
		env.enable(t)
		res, err = env.svc.Auth.Login(ctx, "alice", testPassword, "", "")
		if err != nil || !res.TOTPRequired {
			t.Fatalf("expected challenge: %+v %v", res, err)
		}
		if err := env.svc.TwoFactor.AdminReset(ctx, env.user.ID); err != nil {
			t.Fatalf("AdminReset: %v", err)
		}
		u, _ := env.svc.User.GetByID(ctx, env.user.ID)
		if u.TOTPEnabled || u.TOTPSecret != "" {
			t.Fatal("TOTP still configured after reset")
		}
	})
}

// TestTwoFactorScopedEnforcement runs the role and product rules (SC-107)
// through the real stores.
func TestTwoFactorScopedEnforcement(t *testing.T) {
	ctx := context.Background()
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		env := newTwoFactorEnv(t, driver, conn)
		svc := env.svc

		product, err := svc.Product.Create(ctx, env.user, CreateProductInput{Name: "Strict"})
		if err != nil {
			t.Fatalf("create product: %v", err)
		}

		// Role rules: alice owns a product.
		if err := svc.Setting.Update(ctx, map[string]string{"2fa_enforced_developer": "true"}); err != nil {
			t.Fatal(err)
		}
		if svc.TwoFactor.RequiredFor(ctx, env.user) {
			t.Fatal("developer rule applies to an owner")
		}
		if err := svc.Setting.Update(ctx, map[string]string{"2fa_enforced_owner": "true"}); err != nil {
			t.Fatal(err)
		}
		if !svc.TwoFactor.RequiredFor(ctx, env.user) {
			t.Fatal("owner rule does not apply to an owner")
		}
		if err := svc.Setting.Update(ctx, map[string]string{"2fa_enforced_owner": "false", "2fa_enforced_developer": "false"}); err != nil {
			t.Fatal(err)
		}

		// Requiring 2FA for the product needs 2FA of the owner.
		on := true
		if _, err := svc.Product.Update(ctx, env.user, product.ID, UpdateProductInput{TwoFactorRequired: &on}); !errors.Is(err, ErrOwnTwoFactorRequired) {
			t.Fatalf("require without own 2FA: err = %v", err)
		}
		env.enable(t)
		alice, err := svc.User.GetByID(ctx, env.user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Product.Update(ctx, alice, product.ID, UpdateProductInput{TwoFactorRequired: &on}); err != nil {
			t.Fatalf("require 2FA: %v", err)
		}

		req, err := svc.TwoFactor.Requirement(ctx, alice)
		if err != nil {
			t.Fatal(err)
		}
		if len(req.Products) != 1 || req.Products[0].ID != product.ID || len(req.Instance) != 0 {
			t.Fatalf("requirement = %+v", req)
		}
		if err := svc.TwoFactor.Disable(ctx, alice.ID, testPassword, env.code(t)); !errors.Is(err, ErrTOTPEnforced) {
			t.Fatalf("disable while a product requires 2FA: err = %v", err)
		}

		// A member without 2FA is locked out of the product, but not out of
		// Nexmap, and not when using a token.
		now := time.Now().UTC().Format(time.RFC3339)
		bob := &model.User{
			ID: uuid.NewString(), Username: "bob", Email: "bob@example.com", DisplayName: "Bob",
			PasswordHash: "x", Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
		}
		if err := svc.User.users.Create(ctx, bob); err != nil {
			t.Fatal(err)
		}
		if err := svc.Product.AddMember(ctx, alice, product.ID, bob.ID, model.RoleViewer); err != nil {
			t.Fatalf("add member: %v", err)
		}
		if svc.TwoFactor.RequiredFor(ctx, bob) {
			t.Fatal("a product requirement forces enrollment instance-wide")
		}
		if !svc.TwoFactor.ProductLocked(ctx, bob, product.ID) {
			t.Fatal("ProductLocked = false for a member without 2FA")
		}
		if _, err := svc.Product.GetByID(ctx, bob, product.ID); !errors.Is(err, ErrTwoFactorRequired) {
			t.Fatalf("member without 2FA: err = %v", err)
		}
		token := access.WithRestriction(ctx, &access.Restriction{Grants: access.Grants{access.Settings: access.Read}})
		if _, err := svc.Product.GetByID(token, bob, product.ID); err != nil {
			t.Fatalf("token request: %v", err)
		}
		list, err := svc.Product.ListWithRoles(ctx, bob)
		if err != nil || len(list) != 1 || !list[0].TwoFactorLocked {
			t.Fatalf("ListWithRoles = %+v, %v", list, err)
		}
	})
}
