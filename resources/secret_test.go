package resources

import (
	"encoding/base64"
	"testing"
)

func TestParseSecretManifestStringDataEncodesData(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-key
  namespace: team-a
spec:
  stringData:
    value: sk-test-123
`)
	secret, err := ParseSecretManifest(raw)
	if err != nil {
		t.Fatalf("parse secret manifest failed: %v", err)
	}
	if secret.Metadata.Namespace != "team-a" {
		t.Fatalf("expected namespace team-a, got %q", secret.Metadata.Namespace)
	}
	encoded, ok := secret.Spec.Data["value"]
	if !ok {
		t.Fatal("expected spec.data.value from stringData")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("expected valid base64, got %v", err)
	}
	if string(decoded) != "sk-test-123" {
		t.Fatalf("expected decoded value sk-test-123, got %q", string(decoded))
	}
	if len(secret.Spec.StringData) != 0 {
		t.Fatalf("expected normalized stringData to be write-only empty map, got len=%d", len(secret.Spec.StringData))
	}
}

func TestParseSecretManifestForPutMergesRedactedPlaceholders(t *testing.T) {
	current := Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata:   ObjectMeta{Name: "s", Namespace: "ns"},
		Spec: SecretSpec{
			Data: map[string]string{
				"value": "c2tfc2VjcmV0", // base64 "sk_secret"
			},
		},
	}
	raw := []byte(`{
		"apiVersion": "orloj.dev/v1",
		"kind": "Secret",
		"metadata": { "name": "s", "namespace": "ns" },
		"spec": { "data": { "value": "***" } }
	}`)
	got, err := ParseSecretManifestForPut(raw, current)
	if err != nil {
		t.Fatalf("ParseSecretManifestForPut: %v", err)
	}
	if got.Spec.Data["value"] != current.Spec.Data["value"] {
		t.Fatalf("expected merged base64 value, got %q", got.Spec.Data["value"])
	}
}

func TestParseSecretManifestStringDataWithInlineComment(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: api-key
  namespace: team-a
spec:
  stringData:
    value: "sk-ant-api03-abc123" # replace-with-your-actual-key
`)
	secret, err := ParseSecretManifest(raw)
	if err != nil {
		t.Fatalf("parse secret manifest failed: %v", err)
	}
	encoded, ok := secret.Spec.Data["value"]
	if !ok {
		t.Fatal("expected spec.data.value from stringData")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("expected valid base64, got %v", err)
	}
	if string(decoded) != "sk-ant-api03-abc123" {
		t.Fatalf("expected decoded value %q, got %q (inline comment leaked into value)", "sk-ant-api03-abc123", string(decoded))
	}
}

func TestParseSecretManifestUnquotedValueWithInlineComment(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: api-key
spec:
  stringData:
    value: sk-test-123 # this is a comment
`)
	secret, err := ParseSecretManifest(raw)
	if err != nil {
		t.Fatalf("parse secret manifest failed: %v", err)
	}
	encoded, ok := secret.Spec.Data["value"]
	if !ok {
		t.Fatal("expected spec.data.value from stringData")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("expected valid base64, got %v", err)
	}
	if string(decoded) != "sk-test-123" {
		t.Fatalf("expected decoded value %q, got %q (inline comment leaked into value)", "sk-test-123", string(decoded))
	}
}

func TestSecretNormalizeRejectsInvalidBase64Data(t *testing.T) {
	secret := Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata:   ObjectMeta{Name: "bad"},
		Spec: SecretSpec{
			Data: map[string]string{
				"value": "not_base64",
			},
		},
	}
	if err := secret.Normalize(); err == nil {
		t.Fatal("expected invalid base64 normalization error")
	}
}
