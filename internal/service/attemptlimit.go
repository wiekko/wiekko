// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"errors"
	"sync"
	"time"
)

// Limits for wrong second-factor codes (TOTP and recovery codes), counted per
// user across all of that user's logins and sessions.
//
// A login challenge allows only a few guesses, but every correct password
// yields a new challenge, so without this limit someone who knows the password
// could guess the six-digit code as often as they like. Unlike a limit on
// wrong passwords, this one cannot be used to lock a stranger out: reaching it
// takes the user's password or session.
const (
	SecondFactorFailureWindow = time.Hour
	secondFactorMaxFailures   = 10
)

// Limits for creating accounts (self-registration and invitations) per client
// address. Every attempt counts, successful or not: each one costs a password
// hash, and nobody needs to create many accounts from one place.
const (
	RegistrationWindow        = time.Hour
	registrationMaxPerAddress = 20
)

// ErrTooManyAttempts is returned while a user's second factor or a client's
// account creation is blocked. RetryAfter on the wrapping AttemptsBlockedError
// says for how long.
var ErrTooManyAttempts = errors.New("too many attempts")

// AttemptsBlockedError carries how long the caller has to wait.
type AttemptsBlockedError struct {
	RetryAfter time.Duration
}

func (e *AttemptsBlockedError) Error() string { return ErrTooManyAttempts.Error() }
func (e *AttemptsBlockedError) Unwrap() error { return ErrTooManyAttempts }

// attemptLimiter counts events per key within a window that starts at the
// first event; reaching max blocks the key until that window ends. Like the
// login limiter it lives in memory: wiekkod is a single process, and a restart
// clears the counters. A nil limiter never blocks.
type attemptLimiter struct {
	max    int
	window time.Duration

	mu      sync.Mutex
	entries map[string]*loginFailures
	now     func() time.Time
}

func newAttemptLimiter(max int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{max: max, window: window, entries: map[string]*loginFailures{}, now: time.Now}
}

// check returns an *AttemptsBlockedError if key has reached the limit.
func (l *attemptLimiter) check(key string) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	e := l.entries[key]
	if e == nil || !now.Before(e.resetAt) || e.count < l.max {
		return nil
	}
	return &AttemptsBlockedError{RetryAfter: e.resetAt.Sub(now)}
}

// add records one event for key and reports whether that reached the limit.
func (l *attemptLimiter) add(key string) (reached bool) {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.entries) >= loginLimiterSweepAt {
		for k, e := range l.entries {
			if !now.Before(e.resetAt) {
				delete(l.entries, k)
			}
		}
	}
	e := l.entries[key]
	if e == nil || !now.Before(e.resetAt) {
		e = &loginFailures{resetAt: now.Add(l.window)}
		l.entries[key] = e
	}
	e.count++
	return e.count == l.max
}

// reset forgets key.
func (l *attemptLimiter) reset(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
