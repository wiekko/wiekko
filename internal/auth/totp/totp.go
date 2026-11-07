// Package totp implements time-based one-time passwords (RFC 6238) with the
// parameters every common authenticator app supports: HMAC-SHA1, 6 digits,
// 30-second steps. It also provides one-time recovery codes and a QR code
// rendering of the provisioning URI.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 default; required for authenticator app compatibility.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Digits is the number of digits in a generated code.
	Digits = 6
	// Period is the length of one time step.
	Period = 30 * time.Second
	// Skew is the number of steps accepted before and after the current one,
	// to tolerate clock drift between server and authenticator.
	Skew = 1

	secretBytes = 20 // 160 bits, as recommended by RFC 4226
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new random base32-encoded shared secret.
func GenerateSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("totp: generate secret: %w", err)
	}
	return b32.EncodeToString(b), nil
}

// Step returns the time step number for t.
func Step(t time.Time) int64 {
	return t.Unix() / int64(Period/time.Second)
}

// CodeAt returns the code for the given secret and time step.
func CodeAt(secret string, step int64) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	return hotp(key, step), nil
}

// Validate checks code against secret at time now, allowing Skew steps of
// drift either way. It returns the matched step so callers can reject a code
// that was already used (replay protection); ok is false when nothing matched.
func Validate(secret, code string, now time.Time) (step int64, ok bool) {
	code = NormalizeCode(code)
	if len(code) != Digits {
		return 0, false
	}
	key, err := decodeSecret(secret)
	if err != nil {
		return 0, false
	}
	current := Step(now)
	for d := int64(-Skew); d <= Skew; d++ {
		s := current + d
		if subtle.ConstantTimeCompare([]byte(hotp(key, s)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// NormalizeCode strips the whitespace and dashes users tend to type.
func NormalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(code))
}

// IsNumericCode reports whether code (after normalization) looks like a TOTP
// code rather than a recovery code.
func IsNumericCode(code string) bool {
	code = NormalizeCode(code)
	if len(code) != Digits {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ProvisioningURI builds the otpauth:// URI authenticator apps import.
func ProvisioningURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period/time.Second)))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// FormatSecret groups a secret in blocks of four for manual entry.
func FormatSecret(secret string) string {
	var b strings.Builder
	for i, r := range secret {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	s = strings.TrimRight(s, "=")
	key, err := b32.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("totp: invalid secret: %w", err)
	}
	return key, nil
}

func hotp(key []byte, counter int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for range Digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", Digits, bin%mod)
}

// --- Recovery codes ---------------------------------------------------------

// RecoveryCodeCount is how many recovery codes a user receives at once.
const RecoveryCodeCount = 10

// recoveryAlphabet avoids characters that are easily confused (0/o, 1/l/i).
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// GenerateRecoveryCodes returns n random recovery codes formatted as
// "xxxxx-xxxxx" (50 bits of entropy each).
func GenerateRecoveryCodes(n int) ([]string, error) {
	codes := make([]string, n)
	buf := make([]byte, 10)
	for i := range codes {
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("totp: generate recovery code: %w", err)
		}
		var b strings.Builder
		for j, c := range buf {
			if j == 5 {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[int(c)%len(recoveryAlphabet)])
		}
		codes[i] = b.String()
	}
	return codes, nil
}

// HashRecoveryCode returns the storage hash of a recovery code. Codes carry
// enough entropy that a plain SHA-256 is adequate; the input is normalized so
// that case, spaces and dashes don't matter.
func HashRecoveryCode(code string) string {
	norm := strings.ToLower(NormalizeCode(code))
	h := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(h[:])
}
