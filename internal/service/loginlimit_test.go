package service

import (
	"errors"
	"testing"
	"time"
)

func newTestLimiter() (*loginLimiter, *time.Time) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newLoginLimiter()
	l.now = func() time.Time { return now }
	return l, &now
}

func TestLoginLimiterPerUsername(t *testing.T) {
	l, now := newTestLimiter()
	for i := 0; i < loginMaxFailuresPerKey; i++ {
		if err := l.check("10.0.0.1", "ada"); err != nil {
			t.Fatalf("attempt %d blocked: %v", i+1, err)
		}
		l.fail("10.0.0.1", "ada")
	}

	err := l.check("10.0.0.1", "Ada ")
	var blocked *LoginBlockedError
	if !errors.As(err, &blocked) || !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("check after limit = %v, want LoginBlockedError", err)
	}
	if blocked.RetryAfter != LoginFailureWindow {
		t.Errorf("RetryAfter = %v, want %v", blocked.RetryAfter, LoginFailureWindow)
	}

	// Other accounts from the same address, and the same account from
	// another address, are not affected.
	if err := l.check("10.0.0.1", "bo"); err != nil {
		t.Errorf("other username blocked: %v", err)
	}
	if err := l.check("10.0.0.2", "ada"); err != nil {
		t.Errorf("other address blocked: %v", err)
	}

	*now = now.Add(LoginFailureWindow)
	if err := l.check("10.0.0.1", "ada"); err != nil {
		t.Errorf("still blocked after the window: %v", err)
	}
}

func TestLoginLimiterPerAddress(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < loginMaxFailuresPerIP; i++ {
		// A new username each time, so only the address limit can trigger.
		user := string(rune('a' + i))
		if err := l.check("10.0.0.1", user); err != nil {
			t.Fatalf("attempt %d blocked: %v", i+1, err)
		}
		l.fail("10.0.0.1", user)
	}
	if err := l.check("10.0.0.1", "fresh"); !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("check after address limit = %v, want blocked", err)
	}
	if err := l.check("10.0.0.2", "fresh"); err != nil {
		t.Errorf("other address blocked: %v", err)
	}
}

func TestLoginLimiterSuccessClearsUsername(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < loginMaxFailuresPerKey-1; i++ {
		l.fail("10.0.0.1", "ada")
	}
	l.succeed("10.0.0.1", "ada")
	for i := 0; i < loginMaxFailuresPerKey-1; i++ {
		l.fail("10.0.0.1", "ada")
	}
	if err := l.check("10.0.0.1", "ada"); err != nil {
		t.Errorf("blocked although a success reset the count: %v", err)
	}
}

func TestLoginLimiterNilIsDisabled(t *testing.T) {
	var l *loginLimiter
	l.fail("10.0.0.1", "ada")
	l.succeed("10.0.0.1", "ada")
	if err := l.check("10.0.0.1", "ada"); err != nil {
		t.Errorf("nil limiter blocked: %v", err)
	}
}

func TestLimiterIP(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7":              "203.0.113.7",
		"203.0.113.7:51234":        "203.0.113.7",
		"[2001:db8::1]:443":        "2001:db8::1",
		"2001:db8::1":              "2001:db8::1",
		"1.1.1.1, 203.0.113.7":     "203.0.113.7",
		"spoofed,  10.1.2.3:8080 ": "10.1.2.3",
	} {
		if got := limiterIP(in); got != want {
			t.Errorf("limiterIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAcceptURL(t *testing.T) {
	s := &InvitationService{}
	if got := s.AcceptURL("tok", "https://req.example.com"); got != "https://req.example.com/accept-invite/tok" {
		t.Errorf("from request: %q", got)
	}
	s.publicURL = "https://nexmap.example.com"
	if got := s.AcceptURL("tok", "http://internal:8080"); got != "https://nexmap.example.com/accept-invite/tok" {
		t.Errorf("from public URL: %q", got)
	}
}
