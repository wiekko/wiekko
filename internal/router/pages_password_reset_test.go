package router

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/mail"
)

const accountEnvURL = "https://nexmap.example.com"

type pageSender struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (s *pageSender) Send(_ context.Context, m mail.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}
func (s *pageSender) Name() string { return "page-test" }

func (s *pageSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

// waitFor waits until n messages were sent by the background mailer.
func (s *pageSender) waitFor(t *testing.T, n int) []mail.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.count() < n {
		if time.Now().After(deadline) {
			t.Fatalf("got %d messages, want %d", s.count(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]mail.Message(nil), s.sent...)
}

var resetLink = regexp.MustCompile(regexp.QuoteMeta(accountEnvURL) + `(/reset-password/[0-9a-f]{64})`)

// Without mail, there is no password reset, and the login page says so.
func TestPasswordResetPagesWithoutMail(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		for _, path := range []string{"/forgot-password", "/reset-password/" + strings.Repeat("a", 64)} {
			if resp, _ := e.get(path); resp.StatusCode != http.StatusNotFound {
				t.Errorf("GET %s: status %d, want 404", path, resp.StatusCode)
			}
		}
		if resp, _ := e.post("/forgot-password", url.Values{"identifier": {"root"}}); resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST /forgot-password: status %d, want 404", resp.StatusCode)
		}
		_, body := e.get("/login")
		if strings.Contains(body, "/forgot-password") || !strings.Contains(body, "Ask an administrator") {
			t.Error("login page offers a password reset without mail")
		}
	})
}

func TestPasswordResetPages(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		sender := &pageSender{}
		e := newAccountEnvWithMail(t, driver, db, sender)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go e.svc.Mail.Run(ctx)

		_, body := e.get("/login")
		if !strings.Contains(body, `href="/forgot-password"`) {
			t.Error("login page does not link the password reset")
		}
		if resp, _ := e.get("/forgot-password"); resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /forgot-password: %d", resp.StatusCode)
		}

		// Unknown and known accounts get the same answer.
		_, unknown := e.post("/forgot-password", url.Values{"identifier": {"nobody@example.com"}})
		resp, known := e.post("/forgot-password", url.Values{"identifier": {"root"}})
		if resp.StatusCode != http.StatusOK || unknown != known {
			t.Errorf("answers differ for known and unknown accounts (status %d)", resp.StatusCode)
		}
		msgs := sender.waitFor(t, 1)
		if len(msgs) != 1 || msgs[0].To != "root@localhost" {
			t.Fatalf("sent %+v", msgs)
		}
		m := resetLink.FindStringSubmatch(msgs[0].Text)
		if m == nil {
			t.Fatalf("no reset link in:\n%s", msgs[0].Text)
		}
		path := m[1]

		resp, _ = e.get(path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET reset page: %d", resp.StatusCode)
		}
		if resp.Header.Get("Referrer-Policy") != "no-referrer" || resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("token page headers: %v", resp.Header)
		}

		if resp, _ := e.post(path, url.Values{"password": {"New-Secret-123"}, "password_confirm": {"Other-Secret-123"}}); resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("mismatched passwords: status %d", resp.StatusCode)
		}
		resp, body = e.post(path, url.Values{"password": {"New-Secret-123"}, "password_confirm": {"New-Secret-123"}})
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Password changed") {
			t.Fatalf("reset: status %d", resp.StatusCode)
		}
		resp, _ = e.post("/login", url.Values{"username": {"root"}, "password": {"New-Secret-123"}})
		if !hasSession(resp) {
			t.Error("cannot sign in with the new password")
		}
		// The link is used up.
		if resp, _ := e.get(path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("used link: status %d, want 404", resp.StatusCode)
		}
	})
}
