package store

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	password := "very-strong-pass"
	hash, err := GeneratePasswordHash(password)
	if err != nil {
		t.Fatalf("GeneratePasswordHash returned error: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}
	if strings.Contains(hash, password) {
		t.Fatalf("password hash must not include plaintext password")
	}

	ok, err := VerifyPasswordHash(hash, password)
	if err != nil {
		t.Fatalf("VerifyPasswordHash returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected password verification to succeed")
	}

	ok, err = VerifyPasswordHash(hash, "wrong-password")
	if err != nil {
		t.Fatalf("VerifyPasswordHash returned error for wrong password: %v", err)
	}
	if ok {
		t.Fatalf("expected wrong password verification to fail")
	}
}

func TestVerifyPasswordHashRejectsInvalidFormat(t *testing.T) {
	ok, err := VerifyPasswordHash("invalid-hash", "anything")
	if ok {
		t.Fatalf("expected invalid hash to fail verification")
	}
	if !errors.Is(err, ErrInvalidPasswordHash) {
		t.Fatalf("expected ErrInvalidPasswordHash, got %v", err)
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	if err := ValidatePasswordPolicy("short", 12); err == nil {
		t.Fatalf("expected short password to fail policy")
	}
	if err := ValidatePasswordPolicy("very-strong-pass", 12); err != nil {
		t.Fatalf("expected valid password to pass policy: %v", err)
	}
}
