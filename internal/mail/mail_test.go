// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package mail

import (
	"context"
	"errors"
	"io"
	"mime/quotedprintable"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func mustAddr(t *testing.T, s string) *mail.Address {
	t.Helper()
	a, err := mail.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestConfigValidate(t *testing.T) {
	ok := func(c Config) Config {
		t.Helper()
		if err := c.Validate(); err != nil {
			t.Fatalf("Validate(%+v): %v", c, err)
		}
		return c
	}
	bad := func(name string, c Config) {
		t.Helper()
		if err := c.Validate(); err == nil {
			t.Errorf("%s: Validate accepted %+v", name, c)
		}
	}

	if c := ok(Config{}); c.Enabled() {
		t.Error("empty config must leave mail off")
	}

	c := ok(Config{Transport: "SMTP", From: "nexmap <noreply@example.com>", SMTPHost: "mail.example.com"})
	if c.Transport != "smtp" || c.SMTPPort != 587 || c.SMTPTLS != "starttls" || c.MaxPerHour != DefaultMaxPerHour {
		t.Errorf("smtp defaults: %+v", c)
	}
	if c := ok(Config{Transport: "smtp", From: "a@example.com", SMTPHost: "h", SMTPTLS: "tls"}); c.SMTPPort != 465 {
		t.Errorf("implicit TLS port = %d, want 465", c.SMTPPort)
	}
	ok(Config{Transport: "smtp", From: "a@example.com", SMTPHost: "localhost", SMTPTLS: "none"})
	ok(Config{Transport: "mailgun", From: "a@example.com", APIKey: "k", MailgunDomain: "mg.example.com", MailgunRegion: "EU"})
	ok(Config{Transport: "sendgrid", From: "a@example.com", APIKey: "k"})
	ok(Config{Transport: "mailpace", From: "a@example.com", APIKey: "k"})
	ok(Config{Transport: "lettermint", From: "a@example.com", APIKey: "k"})
	if c := ok(Config{Transport: "scaleway", From: "a@example.com", APIKey: "k", ScalewayProjectID: "p"}); c.ScalewayRegion != "fr-par" {
		t.Errorf("scaleway region = %q", c.ScalewayRegion)
	}

	bad("unknown transport", Config{Transport: "pigeon", From: "a@example.com"})
	bad("no from", Config{Transport: "sendgrid", APIKey: "k"})
	bad("bad from", Config{Transport: "sendgrid", From: "not an address", APIKey: "k"})
	bad("from with newline", Config{Transport: "sendgrid", From: "a@example.com\r\nBcc: x@example.com", APIKey: "k"})
	bad("negative cap", Config{Transport: "sendgrid", From: "a@example.com", APIKey: "k", MaxPerHour: -1})
	bad("smtp no host", Config{Transport: "smtp", From: "a@example.com"})
	bad("smtp none with credentials", Config{Transport: "smtp", From: "a@example.com", SMTPHost: "h", SMTPTLS: "none", SMTPUsername: "u", SMTPPassword: "p"})
	bad("smtp bad tls", Config{Transport: "smtp", From: "a@example.com", SMTPHost: "h", SMTPTLS: "maybe"})
	bad("smtp user without password", Config{Transport: "smtp", From: "a@example.com", SMTPHost: "h", SMTPUsername: "u"})
	bad("smtp bad port", Config{Transport: "smtp", From: "a@example.com", SMTPHost: "h", SMTPPort: 70000})
	bad("mailgun no domain", Config{Transport: "mailgun", From: "a@example.com", APIKey: "k"})
	bad("mailgun bad region", Config{Transport: "mailgun", From: "a@example.com", APIKey: "k", MailgunDomain: "d", MailgunRegion: "ap"})
	bad("mailgun domain with path", Config{Transport: "mailgun", From: "a@example.com", APIKey: "k", MailgunDomain: "d/../x"})
	bad("sendgrid no key", Config{Transport: "sendgrid", From: "a@example.com"})
	bad("scaleway no project", Config{Transport: "scaleway", From: "a@example.com", APIKey: "k"})
	bad("scaleway bad region", Config{Transport: "scaleway", From: "a@example.com", APIKey: "k", ScalewayProjectID: "p", ScalewayRegion: "us-east"})
}

func TestCheckMessage(t *testing.T) {
	good := Message{To: "ada@example.com", Subject: "Hello", Text: "line 1\nline 2"}
	if err := checkMessage(good); err != nil {
		t.Fatalf("checkMessage(good): %v", err)
	}
	for name, m := range map[string]Message{
		"empty":         {To: ""},
		"display name":  {To: "Ada <ada@example.com>"},
		"list":          {To: "ada@example.com, bob@example.com"},
		"semicolon":     {To: "ada@example.com;bob@example.com"},
		"crlf in to":    {To: "ada@example.com\r\nBcc: bob@example.com"},
		"crlf subject":  {To: "ada@example.com", Subject: "Hi\r\nBcc: bob@example.com"},
		"lf subject":    {To: "ada@example.com", Subject: "Hi\nX: y"},
		"not an addr":   {To: "ada"},
		"too long addr": {To: strings.Repeat("a", 250) + "@example.com"},
	} {
		if err := checkMessage(m); !errors.Is(err, ErrInvalidMessage) {
			t.Errorf("%s: err = %v, want ErrInvalidMessage", name, err)
		}
	}
}

func TestBuildMessage(t *testing.T) {
	from := mustAddr(t, "Nexmap Ünïcode <noreply@example.com>")
	msg := Message{To: "ada@example.com", Subject: "Réinitialiser", Text: "Hello\nopen https://nexmap.example.com/reset-password/" + strings.Repeat("ab", 32) + "\n"}
	data, err := buildMessage(from, msg, time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") {
		t.Error("message contains a bare LF")
	}
	head, body, found := strings.Cut(s, "\r\n\r\n")
	if !found {
		t.Fatal("no header/body separator")
	}
	for _, want := range []string{
		"To: ada@example.com\r\n",
		"Subject: =?utf-8?q?",
		"Content-Transfer-Encoding: quoted-printable",
		"Auto-Submitted: auto-generated",
		"Message-ID: <",
		"@example.com>",
		"Date: Sat, 14 Nov 2026 10:00:00 +0000",
	} {
		if !strings.Contains(head+"\r\n", want) {
			t.Errorf("headers lack %q:\n%s", want, head)
		}
	}
	if strings.Contains(head, "Ünïcode") {
		t.Error("display name is not encoded")
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decoded), "/reset-password/"+strings.Repeat("ab", 32)) {
		t.Errorf("link did not survive encoding: %q", decoded)
	}

	if _, err := buildMessage(from, Message{To: "ada@example.com", Subject: "x\r\nBcc: e@example.com"}, time.Now()); !errors.Is(err, ErrInvalidMessage) {
		t.Errorf("header injection: err = %v", err)
	}
}

type countingSender struct{ n int }

func (s *countingSender) Send(context.Context, Message) error { s.n++; return nil }
func (s *countingSender) Name() string                        { return "count" }

func TestCapped(t *testing.T) {
	inner := &countingSender{}
	c := NewCapped(inner, 2).(*capped)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	msg := Message{To: "a@example.com"}

	for i := range 2 {
		if err := c.Send(context.Background(), msg); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if err := c.Send(context.Background(), msg); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("third send: err = %v, want ErrLimitReached", err)
	}
	if inner.n != 2 {
		t.Errorf("inner sends = %d, want 2", inner.n)
	}
	now = now.Add(time.Hour)
	if err := c.Send(context.Background(), msg); err != nil {
		t.Errorf("after an hour: %v", err)
	}
}

func TestNewDisabled(t *testing.T) {
	s, err := New(Config{})
	if err != nil || s != nil {
		t.Fatalf("New(empty) = %v, %v; want nil, nil", s, err)
	}
}

// recorded is one request an API test server received.
type recorded struct {
	method, path string
	header       http.Header
	body         string
}

func apiServer(t *testing.T, status int, respBody string) (*httptest.Server, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*rec = recorded{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: string(b)}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

var testMsg = Message{To: "ada@example.com", Subject: "Your invitation", Text: "Open https://nexmap.example.com/accept-invite/tok"}

func TestMailgun(t *testing.T) {
	srv, rec := apiServer(t, http.StatusOK, `{"id":"x"}`)
	s := newMailgun(Config{APIKey: "key-1", MailgunDomain: "mg.example.com"}, mustAddr(t, "nexmap <noreply@example.com>"))
	s.baseURL = srv.URL
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	if rec.path != "/v3/mg.example.com/messages" {
		t.Errorf("path = %q", rec.path)
	}
	if u, p, _ := (&http.Request{Header: rec.header}).BasicAuth(); u != "api" || p != "key-1" {
		t.Errorf("basic auth = %q/%q", u, p)
	}
	for _, want := range []string{"to=ada%40example.com", "o%3Atracking-clicks=no", "subject=Your+invitation"} {
		if !strings.Contains(rec.body, want) {
			t.Errorf("body lacks %q: %s", want, rec.body)
		}
	}
	if eu := newMailgun(Config{MailgunRegion: "eu"}, mustAddr(t, "a@example.com")); eu.baseURL != "https://api.eu.mailgun.net" {
		t.Errorf("eu base = %q", eu.baseURL)
	}
}

func TestSendGrid(t *testing.T) {
	srv, rec := apiServer(t, http.StatusAccepted, "")
	s := newSendGrid(Config{APIKey: "SG.k"}, mustAddr(t, "nexmap <noreply@example.com>"))
	s.baseURL = srv.URL
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	if rec.path != "/v3/mail/send" || rec.header.Get("Authorization") != "Bearer SG.k" {
		t.Errorf("path %q, auth %q", rec.path, rec.header.Get("Authorization"))
	}
	for _, want := range []string{`"personalizations":[{"to":[{"email":"ada@example.com"}]}]`, `"click_tracking":{"enable":false`, `"type":"text/plain"`} {
		if !strings.Contains(rec.body, want) {
			t.Errorf("body lacks %s: %s", want, rec.body)
		}
	}
}

func TestMailPace(t *testing.T) {
	srv, rec := apiServer(t, http.StatusOK, `{"id":1,"status":"queued"}`)
	s := newMailPace(Config{APIKey: "mp-token"}, mustAddr(t, "noreply@example.com"))
	s.baseURL = srv.URL
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	if rec.path != "/api/v1/send" || rec.header.Get("MailPace-Server-Token") != "mp-token" {
		t.Errorf("path %q, token %q", rec.path, rec.header.Get("MailPace-Server-Token"))
	}
	if rec.header.Get("Idempotency-Key") == "" {
		t.Error("no Idempotency-Key")
	}
	if !strings.Contains(rec.body, `"textbody":"Open https://`) || !strings.Contains(rec.body, `"to":"ada@example.com"`) {
		t.Errorf("body = %s", rec.body)
	}
}

func TestScaleway(t *testing.T) {
	srv, rec := apiServer(t, http.StatusOK, `{"emails":[]}`)
	s := newScaleway(Config{APIKey: "scw-secret", ScalewayProjectID: "proj-1", ScalewayRegion: "fr-par"}, mustAddr(t, "nexmap <noreply@example.com>"))
	s.baseURL = srv.URL
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	if rec.path != "/transactional-email/v1alpha1/regions/fr-par/emails" || rec.header.Get("X-Auth-Token") != "scw-secret" {
		t.Errorf("path %q, token %q", rec.path, rec.header.Get("X-Auth-Token"))
	}
	for _, want := range []string{`"from":{"email":"noreply@example.com","name":"nexmap"}`, `"to":[{"email":"ada@example.com"}]`, `"project_id":"proj-1"`, `"text":"Open`} {
		if !strings.Contains(rec.body, want) {
			t.Errorf("body lacks %s: %s", want, rec.body)
		}
	}
}

func TestLettermint(t *testing.T) {
	srv, rec := apiServer(t, http.StatusAccepted, `{"message_id":"m","status":"pending"}`)
	s := newLettermint(Config{APIKey: "lm-token"}, mustAddr(t, "noreply@example.com"))
	s.baseURL = srv.URL
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	if rec.path != "/v1/send" || rec.header.Get("X-Lettermint-Token") != "lm-token" || rec.header.Get("Idempotency-Key") == "" {
		t.Errorf("path %q, headers %v", rec.path, rec.header)
	}
	for _, want := range []string{`"to":["ada@example.com"]`, `"track_clicks":false`} {
		if !strings.Contains(rec.body, want) {
			t.Errorf("body lacks %s: %s", want, rec.body)
		}
	}
}

// An API error names the status, never the response body, which may echo
// the recipient's address.
func TestAPIErrorHidesBody(t *testing.T) {
	srv, _ := apiServer(t, http.StatusBadRequest, `{"errors":{"to":["ada@example.com is blocked"]}}`)
	s := newMailPace(Config{APIKey: "t"}, mustAddr(t, "noreply@example.com"))
	s.baseURL = srv.URL
	err := s.Send(context.Background(), testMsg)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("err = %v, want status 400", err)
	}
	if strings.Contains(err.Error(), "ada@") {
		t.Errorf("error leaks the address: %v", err)
	}
}

// A redirect is not followed: it would carry the key and the message elsewhere.
func TestAPINoRedirect(t *testing.T) {
	var hits int
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	s := newSendGrid(Config{APIKey: "k"}, mustAddr(t, "noreply@example.com"))
	s.baseURL = srv.URL
	if err := s.Send(context.Background(), testMsg); err == nil {
		t.Error("a redirect counted as success")
	}
	if hits != 0 {
		t.Error("redirect was followed")
	}
}

func TestAPIRejectsInvalidMessage(t *testing.T) {
	srv, rec := apiServer(t, http.StatusOK, "")
	s := newLettermint(Config{APIKey: "k"}, mustAddr(t, "noreply@example.com"))
	s.baseURL = srv.URL
	if err := s.Send(context.Background(), Message{To: "a@example.com, b@example.com"}); !errors.Is(err, ErrInvalidMessage) {
		t.Errorf("err = %v", err)
	}
	if rec.path != "" {
		t.Error("an invalid message reached the API")
	}
}
