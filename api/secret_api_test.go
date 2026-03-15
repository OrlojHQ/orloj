package api_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/crds"
)

func TestSecretCRUDAndNamespaceScoping(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/secrets", crds.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: crds.ObjectMeta{
			Name:      "openai-key",
			Namespace: "team-a",
		},
		Spec: crds.SecretSpec{
			StringData: map[string]string{
				"value": "sk-a",
			},
		},
	})
	postJSON(t, server.URL+"/v1/secrets", crds.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: crds.ObjectMeta{
			Name:      "openai-key",
			Namespace: "team-b",
		},
		Spec: crds.SecretSpec{
			StringData: map[string]string{
				"value": "sk-b",
			},
		},
	})

	resp, err := http.Get(server.URL + "/v1/secrets/openai-key?namespace=team-a")
	if err != nil {
		t.Fatalf("get team-a secret failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for team-a secret, got %d body=%s", resp.StatusCode, string(body))
	}
	var secret crds.Secret
	if err := json.NewDecoder(resp.Body).Decode(&secret); err != nil {
		t.Fatalf("decode secret failed: %v", err)
	}
	if secret.Metadata.Namespace != "team-a" {
		t.Fatalf("expected team-a namespace, got %q", secret.Metadata.Namespace)
	}
	decoded, err := base64.StdEncoding.DecodeString(secret.Spec.Data["value"])
	if err != nil {
		t.Fatalf("expected base64-encoded secret data, got %v", err)
	}
	if string(decoded) != "sk-a" {
		t.Fatalf("expected decoded secret sk-a, got %q", string(decoded))
	}

	respDefault, err := http.Get(server.URL + "/v1/secrets/openai-key")
	if err != nil {
		t.Fatalf("get default namespace secret failed: %v", err)
	}
	defer respDefault.Body.Close()
	if respDefault.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(respDefault.Body)
		t.Fatalf("expected 404 for default namespace secret lookup, got %d body=%s", respDefault.StatusCode, string(body))
	}
}
