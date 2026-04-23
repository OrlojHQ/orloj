package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func newSealedSecretTestAPI(t *testing.T, stores api.Stores) *api.Server {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	return api.NewServer(stores, agentruntime.NewManager(logger), logger)
}

func TestSealingKeyPublicReturns503WithoutActiveKey(t *testing.T) {
	server := newSealedSecretTestAPI(t, api.Stores{})
	req := httptest.NewRequest(http.MethodGet, "/v1/sealing-key/public", nil)
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "no active sealing key is configured") {
		t.Fatalf("expected missing key message, got %q", rr.Body.String())
	}
}

func TestSealingKeyPublicReturnsActiveKey(t *testing.T) {
	keyStore := store.NewSealingKeyStore()
	material, err := resources.GenerateSealingKeyMaterial()
	if err != nil {
		t.Fatalf("generate sealing key material: %v", err)
	}
	if _, err := keyStore.CreateActive(context.Background(), store.SealingKey{
		KeyID:         material.KeyID,
		PublicKeyPEM:  material.PublicKeyPEM,
		PrivateKeyPEM: material.PrivateKeyPEM,
	}); err != nil {
		t.Fatalf("create active sealing key: %v", err)
	}

	server := newSealedSecretTestAPI(t, api.Stores{SealingKeys: keyStore})
	req := httptest.NewRequest(http.MethodGet, "/v1/sealing-key/public", nil)
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		KeyID        string `json:"keyId"`
		Algorithm    string `json:"algorithm"`
		PublicKeyPEM string `json:"publicKeyPEM"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.KeyID != material.KeyID {
		t.Fatalf("expected key id %q, got %q", material.KeyID, payload.KeyID)
	}
	if payload.Algorithm != resources.SealingAlgorithm {
		t.Fatalf("expected algorithm %q, got %q", resources.SealingAlgorithm, payload.Algorithm)
	}
	if strings.TrimSpace(payload.PublicKeyPEM) != strings.TrimSpace(material.PublicKeyPEM) {
		t.Fatalf("expected public key PEM to round-trip")
	}
}

func TestSealedSecretCRUDHandlersWithoutNetworkServer(t *testing.T) {
	server := newSealedSecretTestAPI(t, api.Stores{SealedSecrets: store.NewSealedSecretStore()})

	body, err := json.Marshal(resources.SealedSecret{
		APIVersion: "orloj.dev/v1",
		Kind:       "SealedSecret",
		Metadata: resources.ObjectMeta{
			Name:      "db-creds",
			Namespace: "default",
		},
		Spec: resources.SealedSecretSpec{
			EncryptedData: map[string]resources.SealedValue{
				"username": {
					KeyID:      "key-1",
					WrappedKey: base64.StdEncoding.EncodeToString([]byte("wrapped")),
					Ciphertext: base64.StdEncoding.EncodeToString([]byte("ciphertext")),
				},
			},
			Template: resources.SealedSecretTemplateSecret{
				Labels: map[string]string{"app": "payments"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal sealed secret: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/v1/sealed-secrets?namespace=default", bytes.NewReader(body))
	postReq.Header.Set("Content-Type", "application/json")
	postRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(postRes, postReq)
	if postRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 from create, got %d body=%s", postRes.Code, postRes.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/sealed-secrets/db-creds?namespace=default", nil)
	getRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected 200 from get, got %d body=%s", getRes.Code, getRes.Body.String())
	}

	var stored resources.SealedSecret
	if err := json.Unmarshal(getRes.Body.Bytes(), &stored); err != nil {
		t.Fatalf("decode stored sealed secret: %v", err)
	}
	if stored.Metadata.Name != "db-creds" {
		t.Fatalf("expected stored name db-creds, got %q", stored.Metadata.Name)
	}
	if stored.Status.Phase != "Pending" {
		t.Fatalf("expected Pending status after create, got %q", stored.Status.Phase)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sealed-secrets?namespace=default", nil)
	listRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected 200 from list, got %d body=%s", listRes.Code, listRes.Body.String())
	}
	var list resources.SealedSecretList
	if err := json.Unmarshal(listRes.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode sealed secret list: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 list item, got %d", len(list.Items))
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/sealed-secrets/db-creds?namespace=default", nil)
	deleteRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from delete, got %d body=%s", deleteRes.Code, deleteRes.Body.String())
	}
}
