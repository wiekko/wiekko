package crypto

import (
	"strings"
	"testing"
)

func TestEncryptForBindsToContext(t *testing.T) {
	kr := TestKeyRing()

	ct, keyID, err := kr.EncryptFor("ada@example.com", "users:1:email")
	if err != nil {
		t.Fatalf("EncryptFor: %v", err)
	}
	if !strings.HasPrefix(ct, boundPrefix) {
		t.Fatalf("ciphertext %q is not marked as bound", ct)
	}

	got, err := kr.DecryptFor(ct, keyID, "users:1:email")
	if err != nil || got != "ada@example.com" {
		t.Fatalf("DecryptFor with the right context = %q, %v", got, err)
	}

	// Moved to another record or another field, the value must not decrypt.
	for _, other := range []string{"users:2:email", "users:1:display_name", ""} {
		if _, err := kr.DecryptFor(ct, keyID, other); err == nil {
			t.Errorf("DecryptFor accepted context %q", other)
		}
	}
	// Nor may the marker be stripped to pass it off as an unbound value.
	if _, err := kr.DecryptFor(strings.TrimPrefix(ct, boundPrefix), keyID, "users:2:email"); err == nil {
		t.Error("a bound ciphertext decrypted as an unbound one")
	}
}

func TestDecryptForReadsUnboundValues(t *testing.T) {
	kr := TestKeyRing()

	// Written before values were bound to a context.
	ct, keyID, err := kr.Encrypt("legacy")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := kr.DecryptFor(ct, keyID, "users:1:email")
	if err != nil || got != "legacy" {
		t.Fatalf("DecryptFor on an unbound value = %q, %v", got, err)
	}
}
