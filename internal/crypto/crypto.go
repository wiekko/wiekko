package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Key holds an encryption key with its version identifier.
type Key struct {
	ID  string `json:"id"`
	Raw []byte `json:"-"`
}

// keyJSON is the JSON representation of a key for parsing.
type keyJSON struct {
	ID  string `json:"id"`
	Key string `json:"key"` // base64-encoded 32-byte key
}

// KeyRing manages encryption keys. The first key is the active key
// used for encryption. All keys are available for decryption.
type KeyRing struct {
	keys    []Key
	hmacKey []byte // derived from active key for blind indexes
}

// NewKeyRing parses a JSON key configuration and returns a KeyRing.
// The JSON format is: [{"id":"1","key":"<base64-encoded-32-byte-key>"}]
// At least one key must be provided.
func NewKeyRing(jsonConfig string) (*KeyRing, error) {
	if jsonConfig == "" {
		return nil, errors.New("crypto: encryption key not configured; set NEXMAP_ENCRYPTION_KEY")
	}

	var rawKeys []keyJSON
	if err := json.Unmarshal([]byte(jsonConfig), &rawKeys); err != nil {
		return nil, fmt.Errorf("crypto: invalid key config JSON: %w", err)
	}

	if len(rawKeys) == 0 {
		return nil, errors.New("crypto: key config must contain at least one key")
	}

	keys := make([]Key, len(rawKeys))
	for i, rk := range rawKeys {
		if rk.ID == "" {
			return nil, fmt.Errorf("crypto: key at index %d has empty id", i)
		}
		raw, err := base64.StdEncoding.DecodeString(rk.Key)
		if err != nil {
			return nil, fmt.Errorf("crypto: key %q: invalid base64: %w", rk.ID, err)
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("crypto: key %q: expected 32 bytes, got %d", rk.ID, len(raw))
		}
		keys[i] = Key{ID: rk.ID, Raw: raw}
	}

	// Derive a separate HMAC key from the active key.
	mac := hmac.New(sha256.New, keys[0].Raw)
	mac.Write([]byte("blind-index"))
	hmacKey := mac.Sum(nil)

	return &KeyRing{keys: keys, hmacKey: hmacKey}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the active key.
// Returns base64-encoded ciphertext (nonce prepended) and the key ID.
func (kr *KeyRing) Encrypt(plaintext string) (ciphertext string, keyID string, err error) {
	active := kr.keys[0]

	block, err := aes.NewCipher(active.Raw)
	if err != nil {
		return "", "", fmt.Errorf("crypto: aes: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("crypto: gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", fmt.Errorf("crypto: nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), active.ID, nil
}

// Decrypt decrypts base64-encoded ciphertext using the key identified by keyID.
func (kr *KeyRing) Decrypt(ciphertext string, keyID string) (string, error) {
	key, err := kr.findKey(keyID)
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("crypto: invalid base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key.Raw)
	if err != nil {
		return "", fmt.Errorf("crypto: aes: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("crypto: ciphertext too short")
	}

	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}

	return string(plaintext), nil
}

// BlindIndex computes a deterministic HMAC-SHA256 blind index for equality lookups.
// Returns a hex-encoded string.
func (kr *KeyRing) BlindIndex(value string) string {
	mac := hmac.New(sha256.New, kr.hmacKey)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// findKey looks up a key by its ID.
func (kr *KeyRing) findKey(id string) (*Key, error) {
	for i := range kr.keys {
		if kr.keys[i].ID == id {
			return &kr.keys[i], nil
		}
	}
	return nil, fmt.Errorf("crypto: key %q not found", id)
}

// TestKeyRing returns a KeyRing with a fixed test key for use in tests.
func TestKeyRing() *KeyRing {
	// Fixed 32-byte key for deterministic tests.
	raw := []byte("test-key-32-bytes-long-pad-here!")
	mac := hmac.New(sha256.New, raw)
	mac.Write([]byte("blind-index"))
	return &KeyRing{
		keys:    []Key{{ID: "test", Raw: raw}},
		hmacKey: mac.Sum(nil),
	}
}
