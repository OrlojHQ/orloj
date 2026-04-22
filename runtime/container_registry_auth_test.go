package agentruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type mapSecretResolver struct {
	secrets map[string]string
}

func (r *mapSecretResolver) Resolve(_ context.Context, ref string) (string, error) {
	v, ok := r.secrets[ref]
	if !ok {
		return "", ErrToolSecretNotFound
	}
	return v, nil
}

func TestResolveRegistryAuth_StructuredFields(t *testing.T) {
	resolver := &mapSecretResolver{secrets: map[string]string{
		"my-creds:registry": "ghcr.io",
		"my-creds:username": "bot",
		"my-creds:password": "token123",
	}}

	creds, err := resolveRegistryAuth(context.Background(), resolver, "my-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer creds.Cleanup()

	if creds.configDir == "" {
		t.Fatal("expected non-empty configDir")
	}

	data, err := os.ReadFile(filepath.Join(creds.configDir, "config.json"))
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	var cfg dockerConfigJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}

	entry, ok := cfg.Auths["ghcr.io"]
	if !ok {
		t.Fatal("expected ghcr.io auth entry")
	}

	expectedAuth := base64.StdEncoding.EncodeToString([]byte("bot:token123"))
	if entry.Auth != expectedAuth {
		t.Fatalf("auth mismatch: got %q, want %q", entry.Auth, expectedAuth)
	}

	envVal := creds.DockerConfigEnv()
	if envVal != "DOCKER_CONFIG="+creds.configDir {
		t.Fatalf("DockerConfigEnv() = %q, want DOCKER_CONFIG=%s", envVal, creds.configDir)
	}
}

func TestResolveRegistryAuth_DockerConfigJSON(t *testing.T) {
	rawConfig := `{"auths":{"ghcr.io":{"auth":"Ym90OnRva2VuMTIz"}}}`
	resolver := &mapSecretResolver{secrets: map[string]string{
		"my-creds:.dockerconfigjson": rawConfig,
	}}

	creds, err := resolveRegistryAuth(context.Background(), resolver, "my-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer creds.Cleanup()

	data, err := os.ReadFile(filepath.Join(creds.configDir, "config.json"))
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	if string(data) != rawConfig {
		t.Fatalf("config.json content mismatch:\ngot:  %s\nwant: %s", data, rawConfig)
	}
}

func TestResolveRegistryAuth_DockerConfigJSONTakesPrecedence(t *testing.T) {
	rawConfig := `{"auths":{"registry.example.com":{"auth":"eHh4"}}}`
	resolver := &mapSecretResolver{secrets: map[string]string{
		"my-creds:.dockerconfigjson": rawConfig,
		"my-creds:registry":          "ghcr.io",
		"my-creds:username":          "bot",
		"my-creds:password":          "token123",
	}}

	creds, err := resolveRegistryAuth(context.Background(), resolver, "my-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer creds.Cleanup()

	data, err := os.ReadFile(filepath.Join(creds.configDir, "config.json"))
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	if string(data) != rawConfig {
		t.Fatalf("expected .dockerconfigjson to take precedence, got: %s", data)
	}
}

func TestResolveRegistryAuth_MissingKeys(t *testing.T) {
	resolver := &mapSecretResolver{secrets: map[string]string{
		"my-creds:registry": "ghcr.io",
	}}

	_, err := resolveRegistryAuth(context.Background(), resolver, "my-creds")
	if err == nil {
		t.Fatal("expected error for missing username key")
	}
}

func TestResolveRegistryAuth_NilResolver(t *testing.T) {
	_, err := resolveRegistryAuth(context.Background(), nil, "my-creds")
	if err == nil {
		t.Fatal("expected error for nil resolver")
	}
}

func TestRegistryCredentials_Cleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-cleanup-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("{}"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	creds := &registryCredentials{configDir: tmpDir}
	creds.Cleanup()

	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("expected temp dir to be removed after Cleanup, err=%v", err)
	}
}

func TestRegistryCredentials_NilSafety(t *testing.T) {
	var creds *registryCredentials
	creds.Cleanup()
	if creds.DockerConfigEnv() != "" {
		t.Fatal("expected empty string from nil credentials")
	}
}
