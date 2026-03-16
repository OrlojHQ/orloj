package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const encryptedPrefix = "enc:"

// ParseEncryptionKey decodes a hex- or base64-encoded 256-bit AES key.
// Returns nil without error when raw is empty (encryption disabled).
func ParseEncryptionKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if key, err := hex.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := base64.StdEncoding.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	return nil, fmt.Errorf("secret-encryption-key must be a 256-bit key encoded as hex (64 chars) or base64 (44 chars)")
}

func encryptSecretData(key []byte, data map[string]string) (map[string]string, error) {
	if len(key) == 0 || len(data) == 0 {
		return data, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret encryption: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret encryption: %w", err)
	}
	out := make(map[string]string, len(data))
	for k, v := range data {
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return nil, fmt.Errorf("secret encryption: %w", err)
		}
		ciphertext := gcm.Seal(nonce, nonce, []byte(v), nil)
		out[k] = encryptedPrefix + base64.StdEncoding.EncodeToString(ciphertext)
	}
	return out, nil
}

func decryptSecretData(key []byte, data map[string]string) (map[string]string, error) {
	if len(key) == 0 || len(data) == 0 {
		return data, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret decryption: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret decryption: %w", err)
	}
	out := make(map[string]string, len(data))
	for k, v := range data {
		if !strings.HasPrefix(v, encryptedPrefix) {
			out[k] = v
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, encryptedPrefix))
		if err != nil {
			return nil, fmt.Errorf("secret decryption: key %q: invalid base64: %w", k, err)
		}
		if len(raw) < gcm.NonceSize() {
			return nil, fmt.Errorf("secret decryption: key %q: ciphertext too short", k)
		}
		nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return nil, fmt.Errorf("secret decryption: key %q: %w", k, err)
		}
		out[k] = string(plaintext)
	}
	return out, nil
}
