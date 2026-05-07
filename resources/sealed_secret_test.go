package resources

import (
	"strings"
	"testing"
)

func TestSealUnsealSealedSecretRoundTrip(t *testing.T) {
	material, err := GenerateSealingKeyMaterial()
	if err != nil {
		t.Fatalf("generate sealing key material: %v", err)
	}

	secret := Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: ObjectMeta{
			Name:      "db-creds",
			Namespace: "team-a",
			Labels:    map[string]string{"app": "payments"},
		},
		Spec: SecretSpec{
			StringData: map[string]string{
				"username": "alice",
				"password": "s3cr3t",
			},
		},
	}

	sealed, err := SealSecret(secret, material.KeyID, material.PublicKey)
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}
	if got := len(sealed.Spec.EncryptedData); got != 2 {
		t.Fatalf("expected 2 sealed entries, got %d", got)
	}

	unsealed, err := UnsealSealedSecret(sealed, material.KeyID, material.PrivateKey)
	if err != nil {
		t.Fatalf("unseal sealed secret: %v", err)
	}
	if got := unsealed.Spec.Data["username"]; got != "YWxpY2U=" {
		t.Fatalf("expected username to round-trip as base64 plaintext, got %q", got)
	}
	if got := unsealed.Spec.Data["password"]; got != "czNjcjN0" {
		t.Fatalf("expected password to round-trip as base64 plaintext, got %q", got)
	}
	if got := unsealed.Metadata.Labels["app"]; got != "payments" {
		t.Fatalf("expected template labels to round-trip, got %q", got)
	}
}

func TestUnsealSealedSecretRejectsAADReplay(t *testing.T) {
	material, err := GenerateSealingKeyMaterial()
	if err != nil {
		t.Fatalf("generate sealing key material: %v", err)
	}

	sealed, err := SealSecret(Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: ObjectMeta{
			Name:      "api-key",
			Namespace: "team-a",
		},
		Spec: SecretSpec{
			StringData: map[string]string{"token": "secret-token"},
		},
	}, material.KeyID, material.PublicKey)
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}

	sealed.Metadata.Name = "api-key-copy"
	if _, err := UnsealSealedSecret(sealed, material.KeyID, material.PrivateKey); err == nil {
		t.Fatal("expected AAD mismatch to fail decryption")
	}
}

func TestSealedSecretAADIncludesAlgorithm(t *testing.T) {
	aad := sealedSecretAAD("team-a", "my-secret", "key1")
	if !strings.Contains(string(aad), sealedSecretAADVersion) {
		t.Fatalf("AAD should include algorithm version %q, got %q", sealedSecretAADVersion, string(aad))
	}
}
