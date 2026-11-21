// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// rfcSecret is the SHA-1 test key from RFC 6238 Appendix B.
var rfcSecret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))

func TestCodeAtRFC6238Vectors(t *testing.T) {
	// RFC 6238 publishes 8-digit codes; the 6-digit code is their last six digits.
	vectors := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, v := range vectors {
		got, err := CodeAt(rfcSecret, Step(time.Unix(v.unix, 0)))
		if err != nil {
			t.Fatalf("CodeAt(%d): %v", v.unix, err)
		}
		if got != v.want {
			t.Errorf("CodeAt(%d) = %s, want %s", v.unix, got, v.want)
		}
	}
}

func TestValidate(t *testing.T) {
	now := time.Unix(1111111111, 0)
	cur, _ := CodeAt(rfcSecret, Step(now))
	prev, _ := CodeAt(rfcSecret, Step(now)-1)
	old, _ := CodeAt(rfcSecret, Step(now)-2)

	if step, ok := Validate(rfcSecret, cur, now); !ok || step != Step(now) {
		t.Fatalf("current code rejected (ok=%v step=%d)", ok, step)
	}
	if step, ok := Validate(rfcSecret, prev, now); !ok || step != Step(now)-1 {
		t.Fatalf("previous-step code rejected (ok=%v step=%d)", ok, step)
	}
	if _, ok := Validate(rfcSecret, old, now); ok {
		t.Fatal("code two steps old accepted")
	}
	if _, ok := Validate(rfcSecret, cur[:3]+" "+cur[3:], now); !ok {
		t.Fatal("code with space rejected")
	}
	if _, ok := Validate(rfcSecret, "12345", now); ok {
		t.Fatal("short code accepted")
	}
	if _, ok := Validate("not base32!", cur, now); ok {
		t.Fatal("invalid secret accepted")
	}
}

func TestGenerateSecret(t *testing.T) {
	a, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateSecret()
	if a == b {
		t.Fatal("secrets are not random")
	}
	if len(a) != 32 {
		t.Fatalf("secret length = %d, want 32", len(a))
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("Wiekko", "alice", "ABCDEF")
	for _, want := range []string{"otpauth://totp/Wiekko:alice?", "secret=ABCDEF", "issuer=Wiekko", "digits=6", "period=30"} {
		if !strings.Contains(uri, want) {
			t.Errorf("URI %q missing %q", uri, want)
		}
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes(RecoveryCodeCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("got %d codes", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 11 || c[5] != '-' {
			t.Errorf("bad code format %q", c)
		}
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		seen[c] = true
		if IsNumericCode(c) {
			t.Errorf("recovery code %q mistaken for TOTP code", c)
		}
	}
	if HashRecoveryCode(codes[0]) != HashRecoveryCode(" "+strings.ToUpper(strings.ReplaceAll(codes[0], "-", ""))) {
		t.Fatal("hash not normalized")
	}
}

func TestQRCodeSVG(t *testing.T) {
	svg, err := QRCodeSVG(ProvisioningURI("Wiekko", "alice", rfcSecret))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "<path d=\"M") {
		t.Fatalf("unexpected SVG: %.80s", svg)
	}
}
