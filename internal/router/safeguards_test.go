// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"wiekko.net/internal/access"
	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
)

// TestAccountSafeguards covers the rules that keep an instance and its
// products manageable and its accounts hard to take over.
func TestAccountSafeguards(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		e.invite("olga", model.RoleOwner)
		e.invite("casey", "")
		olga, err := e.st.User.GetByUsername(ctx, "olga")
		if err != nil {
			t.Fatal(err)
		}
		casey, err := e.st.User.GetByUsername(ctx, "casey")
		if err != nil {
			t.Fatal(err)
		}

		t.Run("identities ignore case", func(t *testing.T) {
			accept := func(username, email string) error {
				inv, err := e.svc.Invitation.Create(ctx, e.admin, email, "", "")
				if err != nil {
					// An invalid address is refused already when inviting.
					return err
				}
				_, err = e.svc.User.AcceptInvitation(ctx, inv.RawToken, service.RegisterInput{Username: username, Password: goodPassword})
				return err
			}
			if err := accept("CASEY", "other@example.com"); !errors.Is(err, service.ErrUsernameTaken) {
				t.Errorf("username differing in case: err = %v, want ErrUsernameTaken", err)
			}
			if err := accept("casey2", "Casey@Example.COM"); !errors.Is(err, service.ErrEmailTaken) {
				t.Errorf("email differing in case: err = %v, want ErrEmailTaken", err)
			}
			if err := accept("casey3", "not an address"); err == nil {
				t.Error("an invalid email address was accepted")
			}
			// Signing in works whatever the case.
			resp, _ := e.post("/login", url.Values{"username": {"Casey"}, "password": {goodPassword}})
			if !hasSession(resp) {
				t.Errorf("sign-in as Casey: status %d", resp.StatusCode)
			}
		})

		t.Run("profile email", func(t *testing.T) {
			taken := "OLGA@example.com"
			if _, err := e.svc.User.UpdateProfile(ctx, casey.ID, service.UpdateProfileInput{Email: &taken}); !errors.Is(err, service.ErrEmailTaken) {
				t.Errorf("taking another account's address: err = %v, want ErrEmailTaken", err)
			}
			bad := "nonsense"
			if _, err := e.svc.User.UpdateProfile(ctx, casey.ID, service.UpdateProfileInput{Email: &bad}); err == nil {
				t.Error("an invalid email address was accepted")
			}
		})

		t.Run("last owner", func(t *testing.T) {
			// root created the product and olga was invited as owner: two owners.
			if err := e.svc.Product.RemoveMember(ctx, e.admin, e.product.ID, olga.ID); err != nil {
				t.Fatalf("remove one of two owners: %v", err)
			}
			if err := e.svc.Product.RemoveMember(ctx, e.admin, e.product.ID, e.admin.ID); !errors.Is(err, service.ErrLastOwner) {
				t.Errorf("remove the last owner: err = %v, want ErrLastOwner", err)
			}
			if err := e.svc.Product.UpdateMember(ctx, e.admin, e.product.ID, e.admin.ID, model.RoleViewer); !errors.Is(err, service.ErrLastOwner) {
				t.Errorf("demote the last owner: err = %v, want ErrLastOwner", err)
			}
			if err := e.svc.Product.AddMember(ctx, e.admin, e.product.ID, casey.ID, "emperor"); err == nil {
				t.Error("an unknown role was accepted")
			}

			// The team change is part of the product's history.
			entries, err := e.svc.Audit.ListByEntity(ctx, e.admin, e.product.ID, "product", e.product.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, a := range entries {
				if a.FieldName == "role of user "+olga.ID && a.OldValue == model.RoleOwner && a.NewValue == "no access" {
					found = true
				}
			}
			if !found {
				t.Error("removing a member left no audit entry")
			}
		})

		t.Run("last administrator", func(t *testing.T) {
			if err := e.svc.User.AdminLockUser(ctx, e.admin.ID); !errors.Is(err, service.ErrLastAdmin) {
				t.Errorf("lock: err = %v, want ErrLastAdmin", err)
			}
			if err := e.svc.User.AdminDeleteUser(ctx, e.admin.ID); !errors.Is(err, service.ErrLastAdmin) {
				t.Errorf("delete: err = %v, want ErrLastAdmin", err)
			}
			if err := e.svc.User.RequestDeletion(ctx, e.admin.ID, goodPassword); !errors.Is(err, service.ErrLastAdmin) {
				t.Errorf("own deletion: err = %v, want ErrLastAdmin", err)
			}
			if err := e.svc.User.AdminLockUser(ctx, olga.ID); err != nil {
				t.Errorf("lock an ordinary user: %v", err)
			}
		})

		t.Run("password confirmation", func(t *testing.T) {
			account := access.Grants{access.Account: access.Write}
			_, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: account})
			if !errors.Is(err, service.ErrPasswordRequired) {
				t.Errorf("account token without password: err = %v, want ErrPasswordRequired", err)
			}
			_, err = e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: account, Password: "wrong"})
			if !errors.Is(err, service.ErrIncorrectPassword) {
				t.Errorf("account token with a wrong password: err = %v, want ErrIncorrectPassword", err)
			}
			if _, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: account, Password: goodPassword}); err != nil {
				t.Errorf("account token with the password: %v", err)
			}
			// Product data alone needs no confirmation.
			if _, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t"}); err != nil {
				t.Errorf("ordinary token: %v", err)
			}

			if err := e.svc.User.RequestDeletion(ctx, casey.ID, "wrong"); !errors.Is(err, service.ErrIncorrectPassword) {
				t.Errorf("delete account with a wrong password: err = %v, want ErrIncorrectPassword", err)
			}
			if err := e.svc.User.RequestDeletion(ctx, casey.ID, goodPassword); err != nil {
				t.Errorf("delete account with the password: %v", err)
			}
		})

		t.Run("settings", func(t *testing.T) {
			for key, value := range map[string]string{
				"password_min_length": "3", "2fa_enforced": "yes", "moderation_enabled": "",
			} {
				if err := e.svc.Setting.Update(ctx, map[string]string{key: value}); !errors.Is(err, service.ErrInvalidSetting) {
					t.Errorf("%s=%q: err = %v, want ErrInvalidSetting", key, value, err)
				}
			}
			if err := e.svc.Setting.Update(ctx, map[string]string{"password_min_length": "12", "2fa_enforced": "false"}); err != nil {
				t.Errorf("valid settings: %v", err)
			}
		})

		t.Run("cross-origin form post", func(t *testing.T) {
			post := func(site string) int {
				t.Helper()
				req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/login",
					strings.NewReader(url.Values{"username": {"root"}, "password": {goodPassword}}.Encode()))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if site != "" {
					req.Header.Set("Sec-Fetch-Site", site)
				}
				resp, err := e.client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				_ = readBody(t, resp)
				return resp.StatusCode
			}
			if got := post("cross-site"); got != http.StatusForbidden {
				t.Errorf("sign-in posted by another site: status %d, want 403", got)
			}
			if got := post("same-origin"); got != http.StatusSeeOther {
				t.Errorf("sign-in from the own page: status %d, want 303", got)
			}
			if got := post(""); got != http.StatusSeeOther {
				t.Errorf("sign-in from a non-browser client: status %d, want 303", got)
			}
		})
	})
}
