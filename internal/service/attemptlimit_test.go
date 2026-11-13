package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"nexmap.net/internal/database/dbtest"
)

func TestAttemptLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newAttemptLimiter(3, time.Hour)
	l.now = func() time.Time { return now }

	for i := 1; i <= 3; i++ {
		if err := l.check("a"); err != nil {
			t.Fatalf("attempt %d blocked: %v", i, err)
		}
		if reached := l.add("a"); reached != (i == 3) {
			t.Fatalf("add %d: reached = %v", i, reached)
		}
	}

	err := l.check("a")
	var blocked *AttemptsBlockedError
	if !errors.As(err, &blocked) || !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("check at the limit = %v, want AttemptsBlockedError", err)
	}
	if blocked.RetryAfter != time.Hour {
		t.Errorf("RetryAfter = %v, want 1h", blocked.RetryAfter)
	}
	if err := l.check("b"); err != nil {
		t.Errorf("other key blocked: %v", err)
	}

	now = now.Add(time.Hour)
	if err := l.check("a"); err != nil {
		t.Errorf("still blocked after the window: %v", err)
	}

	l.add("c")
	l.add("c")
	l.reset("c")
	l.add("c")
	l.add("c")
	if err := l.check("c"); err != nil {
		t.Errorf("blocked although reset cleared the count: %v", err)
	}

	var none *attemptLimiter
	if none.add("x") || none.check("x") != nil {
		t.Error("a nil limiter must never block")
	}
	none.reset("x")
}

// TestSecondFactorLimit checks that wrong codes are counted per user across
// login challenges: a fresh challenge from another correct password must not
// buy more guesses.
func TestSecondFactorLimit(t *testing.T) {
	ctx := context.Background()
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		env := newTwoFactorEnv(t, driver, conn)
		env.enable(t)
		limiterNow := time.Now()
		env.svc.TwoFactor.limiter.now = func() time.Time { return limiterNow }

		challenge := func() string {
			t.Helper()
			res, err := env.svc.Auth.Login(ctx, "alice", testPassword, "", "")
			if err != nil || !res.TOTPRequired {
				t.Fatalf("Login: %+v, %v", res, err)
			}
			return res.ChallengeToken
		}

		// One wrong code per challenge, so the per-challenge cap never triggers.
		for i := 0; i < secondFactorMaxFailures; i++ {
			_, err := env.svc.Auth.CompleteTOTPLogin(ctx, challenge(), "000000", "", "")
			if !errors.Is(err, ErrInvalidTOTPCode) {
				t.Fatalf("wrong code %d: err = %v, want ErrInvalidTOTPCode", i+1, err)
			}
		}

		// Now even the correct code, on a new challenge, is refused.
		_, err := env.svc.Auth.CompleteTOTPLogin(ctx, challenge(), env.code(t), "", "")
		var blocked *AttemptsBlockedError
		if !errors.As(err, &blocked) {
			t.Fatalf("correct code while blocked: err = %v, want AttemptsBlockedError", err)
		}
		// The other uses of the second factor are blocked with it.
		if _, err := env.svc.TwoFactor.RegenerateRecoveryCodes(ctx, env.user.ID, env.code(t)); !errors.Is(err, ErrTooManyAttempts) {
			t.Fatalf("regenerate while blocked: err = %v, want ErrTooManyAttempts", err)
		}

		limiterNow = limiterNow.Add(SecondFactorFailureWindow)
		done, err := env.svc.Auth.CompleteTOTPLogin(ctx, challenge(), env.code(t), "", "")
		if err != nil || done.SessionID == "" {
			t.Fatalf("after the window: %+v, %v", done, err)
		}
	})
}

func TestRegistrationLimit(t *testing.T) {
	s := &UserService{regLimiter: newAttemptLimiter(2, time.Hour)}
	for i := 0; i < 2; i++ {
		if err := s.countRegistration("203.0.113.5"); err != nil {
			t.Fatalf("attempt %d blocked: %v", i+1, err)
		}
	}
	if err := s.countRegistration("203.0.113.5"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("third attempt: err = %v, want ErrTooManyAttempts", err)
	}
	if err := s.countRegistration("203.0.113.6"); err != nil {
		t.Errorf("other address blocked: %v", err)
	}
	if err := s.countRegistration(""); err != nil {
		t.Errorf("internal caller blocked: %v", err)
	}
}
