// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"wiekko.net/internal/auth/builtin"
	"wiekko.net/internal/config"
	"wiekko.net/internal/crypto"
	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/mail"
	"wiekko.net/internal/model"
	"wiekko.net/internal/storage"
	"wiekko.net/internal/store"
)

// fakeSender records messages instead of sending them.
type fakeSender struct {
	mu   sync.Mutex
	sent []mail.Message
	fail error
}

func (f *fakeSender) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, m)
	return nil
}
func (f *fakeSender) Name() string { return "fake" }

func (f *fakeSender) messages() []mail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]mail.Message(nil), f.sent...)
}

type mailEnv struct {
	st     *store.Store
	svc    *Services
	sender *fakeSender
	admin  *model.User
	alice  *model.User
}

const mailTestURL = "https://wiekko.example.com"

func newMailEnv(t *testing.T, driver string, conn *sql.DB, withMail bool) *mailEnv {
	t.Helper()
	st := store.New(conn, store.Dialect(driver), crypto.TestKeyRing())
	provider := builtin.New(st.User, 1)
	blobs, err := storage.NewFS(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{}
	var s mail.Sender
	if withMail {
		s = sender
	}
	svc := NewWithMail(st, provider, &config.Config{SessionTTL: time.Hour, PublicURL: mailTestURL}, blobs, s)

	mk := func(name string, admin bool, status string) *model.User {
		hash, err := provider.HashPassword(testPassword)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		u := &model.User{
			ID: uuid.NewString(), Username: name, Email: name + "@example.com", DisplayName: name,
			PasswordHash: hash, IsAdmin: admin, Status: status, CreatedAt: now, UpdatedAt: now,
		}
		if err := st.User.Create(context.Background(), u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	env := &mailEnv{st: st, svc: svc, sender: sender}
	env.admin = mk("admin", true, model.UserStatusActive)
	env.alice = mk("alice", false, model.UserStatusActive)
	mk("locked", false, model.UserStatusLocked)
	return env
}

var tokenInLink = regexp.MustCompile(`/(?:accept-invite|reset-password)/([0-9a-f]{64})`)

func linkToken(t *testing.T, m mail.Message) string {
	t.Helper()
	match := tokenInLink.FindStringSubmatch(m.Text)
	if match == nil {
		t.Fatalf("no link in message:\n%s", m.Text)
	}
	return match[1]
}

func TestInvitationSend(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()
		e := newMailEnv(t, driver, conn, true)

		inv, err := e.svc.Invitation.Send(ctx, e.admin, "new@example.com", "", "")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		msgs := e.sender.messages()
		if len(msgs) != 1 || msgs[0].To != "new@example.com" {
			t.Fatalf("sent %+v", msgs)
		}
		if !strings.Contains(msgs[0].Text, mailTestURL+"/accept-invite/") {
			t.Errorf("link not built from the public URL:\n%s", msgs[0].Text)
		}
		// The emailed token is the invitation's.
		details, err := e.svc.User.LookupInvitation(ctx, linkToken(t, msgs[0]))
		if err != nil || details.Email != "new@example.com" {
			t.Fatalf("lookup emailed token: %+v, %v", details, err)
		}
		if inv.ID == "" {
			t.Error("no invitation returned")
		}
	})
}

// Text the inviter controls never reaches the email.
func TestInvitationMailHasNoInviterText(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()
		e := newMailEnv(t, driver, conn, true)
		p, err := e.svc.Product.Create(ctx, e.admin, CreateProductInput{Name: "Buy cheap pills at spam.example"})
		if err != nil {
			t.Fatalf("create product: %v", err)
		}
		if _, err := e.svc.Invitation.Send(ctx, e.admin, "new@example.com", p.ID, model.RoleTester); err != nil {
			t.Fatalf("Send: %v", err)
		}
		m := e.sender.messages()[0]
		if strings.Contains(m.Text, "pills") || strings.Contains(m.Subject, "pills") {
			t.Errorf("product name leaked into the email:\n%s", m.Text)
		}
		if !strings.Contains(m.Text, "as tester") {
			t.Errorf("role missing:\n%s", m.Text)
		}
	})
}

func TestInvitationSendRules(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()

		t.Run("mail off", func(t *testing.T) {
			e := newMailEnv(t, driver, dbtest.Open(t, driver), false)
			if _, err := e.svc.Invitation.Send(ctx, e.admin, "x@example.com", "", ""); !errors.Is(err, ErrMailDisabled) {
				t.Errorf("err = %v, want ErrMailDisabled", err)
			}
			if e.svc.Invitation.MailEnabled() || e.svc.PasswordReset.Enabled() {
				t.Error("mail reported as enabled")
			}
		})

		e := newMailEnv(t, driver, conn, true)
		for _, bad := range []string{"x@example.com\r\nBcc: y@example.com", "Eve <x@example.com>", "a@example.com, b@example.com", "nope"} {
			if _, err := e.svc.Invitation.Send(ctx, e.admin, bad, "", ""); err == nil {
				t.Errorf("Send accepted %q", bad)
			}
		}
		if _, err := e.svc.Invitation.Send(ctx, e.alice, "x@example.com", "", ""); !errors.Is(err, ErrForbidden) {
			t.Errorf("non-admin without product: err = %v", err)
		}
		if n := len(e.sender.messages()); n != 0 {
			t.Errorf("%d messages sent for rejected invitations", n)
		}

		// Per recipient: a few invitations a day, however many inviters.
		for i := range invitationMailsPerRecipient {
			if _, err := e.svc.Invitation.Send(ctx, e.admin, "Target@example.com", "", ""); err != nil {
				t.Fatalf("send %d: %v", i, err)
			}
		}
		if _, err := e.svc.Invitation.Send(ctx, e.admin, "target@EXAMPLE.com", "", ""); !errors.Is(err, ErrTooManyAttempts) {
			t.Errorf("recipient limit: err = %v", err)
		}

		// Per inviter.
		for i := range invitationMailsPerInviter - invitationMailsPerRecipient {
			if _, err := e.svc.Invitation.Send(ctx, e.admin, "r"+uuid.NewString()[:8]+"@example.com", "", ""); err != nil {
				t.Fatalf("send %d: %v", i, err)
			}
		}
		if _, err := e.svc.Invitation.Send(ctx, e.admin, "another@example.com", "", ""); !errors.Is(err, ErrTooManyAttempts) {
			t.Errorf("inviter limit: err = %v", err)
		}
		// Links stay available.
		if _, err := e.svc.Invitation.Create(ctx, e.admin, "another@example.com", "", ""); err != nil {
			t.Errorf("link after mail limit: %v", err)
		}
	})
}

// An invitation whose email failed is withdrawn.
func TestInvitationSendFailureWithdraws(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()
		e := newMailEnv(t, driver, conn, true)
		e.sender.fail = errors.New("relay down")
		if _, err := e.svc.Invitation.Send(ctx, e.admin, "new@example.com", "", ""); !errors.Is(err, ErrInvitationNotSent) {
			t.Fatalf("err = %v, want ErrInvitationNotSent", err)
		}
		invs, err := e.svc.Invitation.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(invs) != 0 {
			t.Errorf("%d invitations left after a failed send", len(invs))
		}
	})
}

func TestPasswordResetFlow(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()
		e := newMailEnv(t, driver, conn, true)
		pr := e.svc.PasswordReset

		// A session that the reset must end.
		login, err := e.svc.Auth.Login(ctx, "alice", testPassword, "192.0.2.1", "test")
		if err != nil {
			t.Fatalf("login: %v", err)
		}

		if err := pr.Request(ctx, "alice@example.com", "192.0.2.1"); err != nil {
			t.Fatalf("Request: %v", err)
		}
		e.svc.Mail.drain(ctx)
		msgs := e.sender.messages()
		if len(msgs) != 1 || msgs[0].To != "alice@example.com" {
			t.Fatalf("sent %+v", msgs)
		}
		token := linkToken(t, msgs[0])
		if !strings.Contains(msgs[0].Text, mailTestURL+"/reset-password/") {
			t.Errorf("link not built from the public URL:\n%s", msgs[0].Text)
		}
		if err := pr.Check(ctx, token); err != nil {
			t.Fatalf("Check: %v", err)
		}

		// A password the policy rejects leaves the link usable.
		if err := pr.Reset(ctx, token, "short"); err == nil {
			t.Fatal("weak password accepted")
		}
		if err := pr.Check(ctx, token); err != nil {
			t.Fatalf("link used up by a rejected password: %v", err)
		}

		const newPassword = "Brand-New-Secret-42"
		if err := pr.Reset(ctx, token, newPassword); err != nil {
			t.Fatalf("Reset: %v", err)
		}
		if _, err := e.svc.Auth.Login(ctx, "alice", newPassword, "192.0.2.1", "test"); err != nil {
			t.Errorf("login with new password: %v", err)
		}
		if _, err := e.st.Session.GetByID(ctx, login.SessionID); err == nil {
			t.Error("old session survived the reset")
		}
		// Single use.
		if err := pr.Reset(ctx, token, "Another-Secret-43"); !errors.Is(err, ErrPasswordResetInvalid) {
			t.Errorf("second use: err = %v", err)
		}
		if err := pr.Check(ctx, "0000"); !errors.Is(err, ErrPasswordResetInvalid) {
			t.Errorf("unknown token: err = %v", err)
		}
	})
}

// The form answers alike for every input and mails only real, active
// accounts, at their stored address.
func TestPasswordResetNoEnumeration(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()
		e := newMailEnv(t, driver, conn, true)
		pr := e.svc.PasswordReset

		for _, who := range []string{"nobody", "nobody@example.com", "locked", "locked@example.com"} {
			if err := pr.Request(ctx, who, ""); err != nil {
				t.Errorf("Request(%q) = %v, want nil", who, err)
			}
		}
		e.svc.Mail.drain(ctx)
		if n := len(e.sender.messages()); n != 0 {
			t.Fatalf("%d messages for unknown or locked accounts", n)
		}

		// Per account: a few emails, then silence, still with a nil answer.
		for range passwordResetMailsPerUser + 2 {
			if err := pr.Request(ctx, "alice", ""); err != nil {
				t.Fatalf("Request: %v", err)
			}
		}
		e.svc.Mail.drain(ctx)
		msgs := e.sender.messages()
		if len(msgs) != passwordResetMailsPerUser {
			t.Fatalf("sent %d, want %d", len(msgs), passwordResetMailsPerUser)
		}
		// Only the newest link works.
		if err := pr.Check(ctx, linkToken(t, msgs[0])); !errors.Is(err, ErrPasswordResetInvalid) {
			t.Errorf("older link still valid: %v", err)
		}
		if err := pr.Check(ctx, linkToken(t, msgs[len(msgs)-1])); err != nil {
			t.Errorf("newest link invalid: %v", err)
		}
	})
}

func TestPasswordResetLimitsAndLockedAccounts(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()
		e := newMailEnv(t, driver, conn, true)
		pr := e.svc.PasswordReset

		for range passwordResetsPerAddress {
			_ = pr.Request(ctx, "nobody", "198.51.100.7")
		}
		if err := pr.Request(ctx, "alice", "198.51.100.7"); !errors.Is(err, ErrTooManyAttempts) {
			t.Errorf("address limit: err = %v", err)
		}

		// A link stops working once the account is locked.
		if err := pr.Request(ctx, "alice", ""); err != nil {
			t.Fatal(err)
		}
		e.svc.Mail.drain(ctx)
		token := linkToken(t, e.sender.messages()[0])
		if err := e.svc.User.AdminLockUser(ctx, e.alice.ID); err != nil {
			t.Fatal(err)
		}
		if err := pr.Reset(ctx, token, "Brand-New-Secret-42"); !errors.Is(err, ErrPasswordResetInvalid) {
			t.Errorf("reset of a locked account: err = %v", err)
		}

		off := newMailEnv(t, driver, dbtest.Open(t, driver), false)
		if err := off.svc.PasswordReset.Request(ctx, "alice", ""); !errors.Is(err, ErrMailDisabled) {
			t.Errorf("mail off: err = %v", err)
		}
	})
}
