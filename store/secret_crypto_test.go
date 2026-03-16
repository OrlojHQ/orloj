package store

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	data := map[string]string{
		"value":  "c2stcHJvai14eHh4eHg=",
		"token":  "bXktdG9rZW4=",
		"secret": "aGVsbG8td29ybGQ=",
	}
	encrypted, err := encryptSecretData(key, data)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range encrypted {
		if !strings.HasPrefix(v, encryptedPrefix) {
			t.Errorf("key %q missing %q prefix", k, encryptedPrefix)
		}
		if v == data[k] {
			t.Errorf("key %q: encrypted value matches plaintext", k)
		}
	}
	decrypted, err := decryptSecretData(key, encrypted)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range data {
		if got := decrypted[k]; got != want {
			t.Errorf("key %q: got %q, want %q", k, got, want)
		}
	}
}

func TestDecryptPassesThroughUnencryptedValues(t *testing.T) {
	key := testKey(t)
	data := map[string]string{
		"plain": "c2stcHJvai14eHh4eHg=",
	}
	decrypted, err := decryptSecretData(key, data)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted["plain"] != data["plain"] {
		t.Errorf("unencrypted value should pass through unchanged")
	}
}

func TestEncryptDecryptNilKeyIsNoop(t *testing.T) {
	data := map[string]string{"value": "abc123"}
	enc, err := encryptSecretData(nil, data)
	if err != nil {
		t.Fatal(err)
	}
	if enc["value"] != "abc123" {
		t.Errorf("nil key should return data unchanged")
	}
	dec, err := decryptSecretData(nil, data)
	if err != nil {
		t.Fatal(err)
	}
	if dec["value"] != "abc123" {
		t.Errorf("nil key should return data unchanged")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	key1 := testKey(t)
	key2 := testKey(t)
	data := map[string]string{"value": "secret-data"}
	encrypted, err := encryptSecretData(key1, data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decryptSecretData(key2, encrypted)
	if err == nil {
		t.Fatal("expected decryption failure with wrong key")
	}
}

func TestParseEncryptionKeyHex(t *testing.T) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	key, err := ParseEncryptionKey(hex.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
}

func TestParseEncryptionKeyBase64(t *testing.T) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	key, err := ParseEncryptionKey(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
}

func TestParseEncryptionKeyEmpty(t *testing.T) {
	key, err := ParseEncryptionKey("")
	if err != nil {
		t.Fatal(err)
	}
	if key != nil {
		t.Fatal("empty input should return nil key")
	}
}

func TestParseEncryptionKeyInvalid(t *testing.T) {
	_, err := ParseEncryptionKey("too-short")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestEachEncryptionProducesUniqueCiphertext(t *testing.T) {
	key := testKey(t)
	data := map[string]string{"value": "same-plaintext"}
	enc1, _ := encryptSecretData(key, data)
	enc2, _ := encryptSecretData(key, data)
	if enc1["value"] == enc2["value"] {
		t.Error("two encryptions of the same plaintext should produce different ciphertexts (unique nonce)")
	}
}
