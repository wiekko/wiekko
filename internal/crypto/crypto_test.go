// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package crypto

import (
	"encoding/base64"
	"testing"
)

func validKeyJSON(keys ...string) string {
	if len(keys) == 0 {
		raw := make([]byte, 32)
		for i := range raw {
			raw[i] = byte(i)
		}
		keys = []string{base64.StdEncoding.EncodeToString(raw)}
	}
	result := "["
	for i, k := range keys {
		if i > 0 {
			result += ","
		}
		result += `{"id":"` + string(rune('1'+i)) + `","key":"` + k + `"}`
	}
	result += "]"
	return result
}

func makeKey(seed byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestNewKeyRing_Valid(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kr.keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(kr.keys))
	}
	if kr.keys[0].ID != "1" {
		t.Fatalf("expected key ID '1', got %q", kr.keys[0].ID)
	}
}

func TestNewKeyRing_Empty(t *testing.T) {
	_, err := NewKeyRing("")
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestNewKeyRing_EmptyArray(t *testing.T) {
	_, err := NewKeyRing("[]")
	if err == nil {
		t.Fatal("expected error for empty array")
	}
}

func TestNewKeyRing_InvalidJSON(t *testing.T) {
	_, err := NewKeyRing("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewKeyRing_WrongKeySize(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	_, err := NewKeyRing(`[{"id":"1","key":"` + short + `"}]`)
	if err == nil {
		t.Fatal("expected error for wrong key size")
	}
}

func TestNewKeyRing_EmptyID(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	_, err := NewKeyRing(`[{"id":"","key":"` + key + `"}]`)
	if err == nil {
		t.Fatal("expected error for empty key ID")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	plaintext := "user@example.com"
	ct, keyID, err := kr.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if keyID != "1" {
		t.Fatalf("expected key ID '1', got %q", keyID)
	}

	got, err := kr.Decrypt(ct, keyID)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, got)
	}
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ct, keyID, err := kr.Encrypt("")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := kr.Decrypt(ct, keyID)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestEncrypt_UniqueNonce(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ct1, _, _ := kr.Encrypt("same-input")
	ct2, _, _ := kr.Encrypt("same-input")

	if ct1 == ct2 {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts")
	}
}

func TestDecrypt_WrongKeyID(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ct, _, err := kr.Encrypt("test")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = kr.Decrypt(ct, "nonexistent")
	if err == nil {
		t.Fatal("expected error for wrong key ID")
	}
}

func TestDecrypt_CorruptedCiphertext(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = kr.Decrypt("dGhpcyBpcyBub3QgdmFsaWQ=", "1")
	if err == nil {
		t.Fatal("expected error for corrupted ciphertext")
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = kr.Decrypt("not-valid-base64!!!", "1")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestKeyRotation(t *testing.T) {
	key1 := makeKey(0)
	key2 := makeKey(100)

	// Encrypt with first key only.
	kr1, err := NewKeyRing(validKeyJSON(key1))
	if err != nil {
		t.Fatalf("setup kr1: %v", err)
	}
	ct, keyID, err := kr1.Encrypt("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if keyID != "1" {
		t.Fatalf("expected key ID '1', got %q", keyID)
	}

	// Create new KeyRing with key2 as active, key1 as old.
	kr2, err := NewKeyRing(`[{"id":"2","key":"` + key2 + `"},{"id":"1","key":"` + key1 + `"}]`)
	if err != nil {
		t.Fatalf("setup kr2: %v", err)
	}

	// Should decrypt with old key.
	got, err := kr2.Decrypt(ct, "1")
	if err != nil {
		t.Fatalf("decrypt with rotated keyring: %v", err)
	}
	if got != "secret" {
		t.Fatalf("expected 'secret', got %q", got)
	}

	// New encryptions use key2.
	ct2, keyID2, err := kr2.Encrypt("new-secret")
	if err != nil {
		t.Fatalf("encrypt with new key: %v", err)
	}
	if keyID2 != "2" {
		t.Fatalf("expected key ID '2', got %q", keyID2)
	}
	got2, err := kr2.Decrypt(ct2, keyID2)
	if err != nil {
		t.Fatalf("decrypt new: %v", err)
	}
	if got2 != "new-secret" {
		t.Fatalf("expected 'new-secret', got %q", got2)
	}
}

func TestBlindIndex_Deterministic(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	idx1 := kr.BlindIndex("user@example.com")
	idx2 := kr.BlindIndex("user@example.com")

	if idx1 != idx2 {
		t.Fatal("blind index should be deterministic")
	}
}

func TestBlindIndex_DifferentInputs(t *testing.T) {
	kr, err := NewKeyRing(validKeyJSON())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	idx1 := kr.BlindIndex("user1@example.com")
	idx2 := kr.BlindIndex("user2@example.com")

	if idx1 == idx2 {
		t.Fatal("different inputs should produce different blind indexes")
	}
}

func TestBlindIndex_DifferentKeys(t *testing.T) {
	kr1, _ := NewKeyRing(validKeyJSON(makeKey(0)))
	kr2, _ := NewKeyRing(validKeyJSON(makeKey(100)))

	idx1 := kr1.BlindIndex("same@example.com")
	idx2 := kr2.BlindIndex("same@example.com")

	if idx1 == idx2 {
		t.Fatal("different keys should produce different blind indexes")
	}
}

func TestTestKeyRing(t *testing.T) {
	kr := TestKeyRing()

	ct, keyID, err := kr.Encrypt("hello")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := kr.Decrypt(ct, keyID)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}

	idx := kr.BlindIndex("test")
	if idx == "" {
		t.Fatal("blind index should not be empty")
	}
}
