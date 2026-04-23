package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type mockSealTransport struct {
	publicKeyPayload []byte
}

func (m *mockSealTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	statusCode := http.StatusNotFound
	body := []byte(`{"error":"not found"}`)
	if r.Method == http.MethodGet && r.URL.Path == "/v1/sealing-key/public" {
		statusCode = http.StatusOK
		body = m.publicKeyPayload
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    r,
	}, nil
}

func withMockSealTransport(t *testing.T, payload []byte, fn func()) {
	t.Helper()
	oldTransport := http.DefaultTransport
	oldClient := http.DefaultClient
	http.DefaultTransport = &mockSealTransport{publicKeyPayload: payload}
	defer func() {
		http.DefaultTransport = oldTransport
		http.DefaultClient = oldClient
	}()
	fn()
}

func sealingPublicKeyPayload(t *testing.T) []byte {
	t.Helper()
	keyMaterial, err := resources.GenerateSealingKeyMaterial()
	if err != nil {
		t.Fatalf("generate sealing key material: %v", err)
	}
	payload, err := json.Marshal(sealingPublicKeyResponse{
		KeyID:        keyMaterial.KeyID,
		Algorithm:    resources.SealingAlgorithm,
		PublicKeyPEM: keyMaterial.PublicKeyPEM,
	})
	if err != nil {
		t.Fatalf("marshal public key payload: %v", err)
	}
	return payload
}

func TestMarshalSealOutputYAMLUsesAPIFieldNames(t *testing.T) {
	sealed := resources.SealedSecret{
		APIVersion: "orloj.dev/v1",
		Kind:       "SealedSecret",
		Metadata:   resources.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: resources.SealedSecretSpec{
			EncryptedData: map[string]resources.SealedValue{
				"value": {
					KeyID:      "kid",
					WrappedKey: "d3JhcHBlZA==",
					Ciphertext: "Y2lwaGVydGV4dA==",
				},
			},
		},
	}
	raw, err := marshalSealOutput(sealed, "yaml")
	if err != nil {
		t.Fatalf("marshal yaml: %v", err)
	}
	out := string(raw)
	for _, want := range []string{"apiVersion: orloj.dev/v1", "kind: SealedSecret", "encryptedData:", "wrappedKey:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestRunSealSecretFromFileWritesDefaultSealedManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "secret.yaml")
	writeManifest(t, manifestPath, `apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-api-key
  namespace: team-a
spec:
  stringData:
    value: sk-test-123
`)

	var (
		out string
		err error
	)
	withMockSealTransport(t, sealingPublicKeyPayload(t), func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"seal", "secret", "-f", manifestPath, "--server", testServerURL()})
		})
	})
	if err != nil {
		t.Fatalf("run seal secret from file: %v", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "secret.sealed.yaml") {
		t.Fatalf("expected write confirmation, got %q", out)
	}

	sealedPath := filepath.Join(dir, "secret.sealed.yaml")
	raw, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatalf("read sealed output: %v", err)
	}
	if strings.Contains(string(raw), "sk-test-123") {
		t.Fatalf("sealed manifest should not contain plaintext secret:\n%s", string(raw))
	}
	sealed, err := resources.ParseSealedSecretManifest(raw)
	if err != nil {
		t.Fatalf("parse sealed manifest: %v", err)
	}
	if sealed.Kind != "SealedSecret" {
		t.Fatalf("expected kind SealedSecret, got %q", sealed.Kind)
	}
	if sealed.Metadata.Name != "openai-api-key" || sealed.Metadata.Namespace != "team-a" {
		t.Fatalf("unexpected metadata: %#v", sealed.Metadata)
	}
	if _, ok := sealed.Spec.EncryptedData["value"]; !ok {
		t.Fatalf("expected encryptedData.value, got %#v", sealed.Spec.EncryptedData)
	}
}

func TestRunSealSecretFromLiteralWritesDefaultFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	var out string
	withMockSealTransport(t, sealingPublicKeyPayload(t), func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{
				"seal", "secret", "openai-api-key",
				"--from-literal", "value=sk-inline-456",
				"--from-literal", "org=acme",
				"--server", testServerURL(),
			})
		})
	})
	if err != nil {
		t.Fatalf("run seal secret from literal: %v", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "openai-api-key.sealed.yaml") {
		t.Fatalf("expected write confirmation, got %q", out)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "openai-api-key.sealed.yaml"))
	if err != nil {
		t.Fatalf("read sealed output: %v", err)
	}
	if strings.Contains(string(raw), "sk-inline-456") {
		t.Fatalf("sealed manifest should not contain plaintext secret:\n%s", string(raw))
	}
	sealed, err := resources.ParseSealedSecretManifest(raw)
	if err != nil {
		t.Fatalf("parse sealed manifest: %v", err)
	}
	if sealed.Metadata.Namespace != resources.DefaultNamespace {
		t.Fatalf("expected default namespace, got %q", sealed.Metadata.Namespace)
	}
	if _, ok := sealed.Spec.EncryptedData["value"]; !ok {
		t.Fatalf("expected encryptedData.value, got %#v", sealed.Spec.EncryptedData)
	}
	if _, ok := sealed.Spec.EncryptedData["org"]; !ok {
		t.Fatalf("expected encryptedData.org, got %#v", sealed.Spec.EncryptedData)
	}
}
