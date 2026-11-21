// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// Limits for failed password logins. Both count failures within a sliding
// window that starts at the first failure; reaching the limit blocks further
// attempts until that window ends.
//
//   - Per client address and username: stops guessing one account's password.
//   - Per client address: stops one client from spraying many usernames.
//
// There is deliberately no limit per username alone, across addresses: it
// would let anyone lock a known user (or the administrator) out of the
// instance by sending wrong passwords for them.
const (
	LoginFailureWindow     = 15 * time.Minute
	loginMaxFailuresPerKey = 5
	loginMaxFailuresPerIP  = 20
	loginLimiterSweepAt    = 10_000 // entries before expired ones are swept
)

// ErrTooManyLoginAttempts is returned by AuthService.Login while the client
// is blocked. RetryAfter on the wrapping LoginBlockedError says for how long.
var ErrTooManyLoginAttempts = errors.New("too many failed login attempts")

// LoginBlockedError carries how long the client has to wait.
type LoginBlockedError struct {
	RetryAfter time.Duration
}

func (e *LoginBlockedError) Error() string { return ErrTooManyLoginAttempts.Error() }
func (e *LoginBlockedError) Unwrap() error { return ErrTooManyLoginAttempts }

// loginLimiter counts failed logins in memory. wiekkod runs as a single
// process, so the counters need no shared storage; a restart clears them.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginFailures
	now     func() time.Time
}

type loginFailures struct {
	count   int
	resetAt time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: map[string]*loginFailures{}, now: time.Now}
}

func ipKey(ip string) string { return "ip\x00" + ip }
func userKey(ip, username string) string {
	return "user\x00" + ip + "\x00" + strings.ToLower(strings.TrimSpace(username))
}

// check returns a *LoginBlockedError if the client may not try now.
func (l *loginLimiter) check(ip, username string) error {
	if l == nil {
		return nil
	}
	ip = limiterIP(ip)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	var wait time.Duration
	for _, k := range []struct {
		key string
		max int
	}{{ipKey(ip), loginMaxFailuresPerIP}, {userKey(ip, username), loginMaxFailuresPerKey}} {
		e := l.entries[k.key]
		if e == nil || !now.Before(e.resetAt) {
			continue
		}
		if e.count >= k.max && e.resetAt.Sub(now) > wait {
			wait = e.resetAt.Sub(now)
		}
	}
	if wait > 0 {
		return &LoginBlockedError{RetryAfter: wait}
	}
	return nil
}

// fail records a failed attempt.
func (l *loginLimiter) fail(ip, username string) {
	if l == nil {
		return
	}
	ip = limiterIP(ip)
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
	for _, key := range []string{ipKey(ip), userKey(ip, username)} {
		e := l.entries[key]
		if e == nil || !now.Before(e.resetAt) {
			e = &loginFailures{resetAt: now.Add(LoginFailureWindow)}
			l.entries[key] = e
		}
		e.count++
	}
}

// succeed clears the account's counter for this client after a correct
// password. The per-address counter stays, so alternating a known account
// with guesses for another one does not reset the address limit.
func (l *loginLimiter) succeed(ip, username string) {
	if l == nil {
		return
	}
	ip = limiterIP(ip)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, userKey(ip, username))
}

// limiterIP reduces the recorded client address to the one the limiter keys
// on. With an X-Forwarded-For chain ("client, proxy1, ..."), only the last
// entry was added by our own proxy; earlier ones come from the client and
// could be changed on every request. The port of a RemoteAddr is dropped, as
// it changes with every connection.
func limiterIP(ip string) string {
	if i := strings.LastIndexByte(ip, ','); i >= 0 {
		ip = ip[i+1:]
	}
	ip = strings.TrimSpace(ip)
	if strings.HasPrefix(ip, "[") { // [::1]:1234
		if end := strings.IndexByte(ip, ']'); end > 0 {
			return ip[1:end]
		}
	}
	if strings.Count(ip, ":") == 1 { // 1.2.3.4:1234
		return ip[:strings.IndexByte(ip, ':')]
	}
	return ip
}
