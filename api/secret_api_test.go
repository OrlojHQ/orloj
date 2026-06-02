package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestSecretCRUDAndNamespaceScoping(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/secrets?namespace=team-a", resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      "openai-key",
			Namespace: "team-a",
		},
		Spec: resources.SecretSpec{
			StringData: map[string]string{
				"value": "sk-a",
			},
		},
	})
	postJSON(t, server.URL+"/v1/secrets?namespace=team-b", resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      "openai-key",
			Namespace: "team-b",
		},
		Spec: resources.SecretSpec{
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
	var secret resources.Secret
	if err := json.NewDecoder(resp.Body).Decode(&secret); err != nil {
		t.Fatalf("decode secret failed: %v", err)
	}
	if secret.Metadata.Namespace != "team-a" {
		t.Fatalf("expected team-a namespace, got %q", secret.Metadata.Namespace)
	}
	// Secret values must be redacted in API responses.
	if val, ok := secret.Spec.Data["value"]; !ok {
		t.Fatalf("expected 'value' key in redacted secret data")
	} else if val != "***" {
		t.Fatalf("expected redacted secret value '***', got %q", val)
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

func TestSecretPutRenameWithRedactedDataValues(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/secrets", resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata:   resources.ObjectMeta{Name: "sec-old", Namespace: "default"},
		Spec: resources.SecretSpec{
			StringData: map[string]string{"value": "super-secret-token"},
		},
	})

	getResp, err := http.Get(server.URL + "/v1/secrets/sec-old")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getResp.Body)
		t.Fatalf("get: %d %s", getResp.StatusCode, string(b))
	}
	var cur resources.Secret
	if err := json.NewDecoder(getResp.Body).Decode(&cur); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cur.Spec.Data["value"] != "***" {
		t.Fatalf("expected redacted GET value")
	}
	cur.Metadata.Name = "sec-new"
	putBody, err := json.Marshal(cur)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/secrets/sec-old", bytes.NewReader(putBody))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", cur.Metadata.ResourceVersion)
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("expected 200 put, got %d body=%s", putResp.StatusCode, string(b))
	}

	getOld, err := http.Get(server.URL + "/v1/secrets/sec-old")
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	defer getOld.Body.Close()
	if getOld.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for old name, got %d", getOld.StatusCode)
	}
}
