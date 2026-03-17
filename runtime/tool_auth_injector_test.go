package agentruntime_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	agentruntime "github.com/OrlojHQ/orloj/runtime"

	"github.com/OrlojHQ/orloj/resources"
)

type testSecretResolver struct {
	values map[string]string
}

func (r testSecretResolver) Resolve(_ context.Context, secretRef string) (string, error) {
	value, ok := r.values[strings.TrimSpace(secretRef)]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func TestAuthInjectorBearerProfile(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"my-api-key": "sk-test-12345",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	result, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "bearer",
		SecretRef: "my-api-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Headers["Authorization"] != "Bearer sk-test-12345" {
		t.Fatalf("expected Bearer header, got %q", result.Headers["Authorization"])
	}
	if result.EnvVars["TOOL_AUTH_BEARER"] != "sk-test-12345" {
		t.Fatalf("expected TOOL_AUTH_BEARER env var, got %q", result.EnvVars["TOOL_AUTH_BEARER"])
	}
	if result.Profile != "bearer" {
		t.Fatalf("expected profile=bearer, got %q", result.Profile)
	}
}

func TestAuthInjectorBearerDefaultWhenProfileEmpty(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"token": "tok-abc",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	result, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		SecretRef: "token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Headers["Authorization"] != "Bearer tok-abc" {
		t.Fatalf("expected default Bearer header, got %q", result.Headers["Authorization"])
	}
}

func TestAuthInjectorAPIKeyHeaderProfile(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"my-key": "key-12345",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	result, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:    "api_key_header",
		SecretRef:  "my-key",
		HeaderName: "X-Api-Key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Headers["X-Api-Key"] != "key-12345" {
		t.Fatalf("expected X-Api-Key header, got %v", result.Headers)
	}
	if result.EnvVars["TOOL_AUTH_HEADER_NAME"] != "X-Api-Key" {
		t.Fatalf("expected TOOL_AUTH_HEADER_NAME=X-Api-Key, got %q", result.EnvVars["TOOL_AUTH_HEADER_NAME"])
	}
	if result.EnvVars["TOOL_AUTH_HEADER_VALUE"] != "key-12345" {
		t.Fatalf("expected TOOL_AUTH_HEADER_VALUE=key-12345, got %q", result.EnvVars["TOOL_AUTH_HEADER_VALUE"])
	}
}

func TestAuthInjectorAPIKeyHeaderRequiresHeaderName(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"my-key": "key-12345",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	_, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "api_key_header",
		SecretRef: "my-key",
	})
	if err == nil {
		t.Fatal("expected error for missing headerName")
	}
}

func TestAuthInjectorBasicProfile(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"basic-creds": "admin:secret123",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	result, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "basic",
		SecretRef: "basic-creds",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret123"))
	if result.Headers["Authorization"] != expected {
		t.Fatalf("expected %q, got %q", expected, result.Headers["Authorization"])
	}
	if result.Profile != "basic" {
		t.Fatalf("expected profile=basic, got %q", result.Profile)
	}
}

func TestAuthInjectorBasicRejectsNoColon(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"bad-creds": "nocolonhere",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	_, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "basic",
		SecretRef: "bad-creds",
	})
	if err == nil {
		t.Fatal("expected error for missing colon in basic auth")
	}
}

func TestAuthInjectorOAuth2RequiresTokenCache(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"oauth:client_id":     "cid",
		"oauth:client_secret": "csec",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	_, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "oauth2_client_credentials",
		SecretRef: "oauth",
		TokenURL:  "https://auth.example.com/token",
	})
	if err == nil {
		t.Fatal("expected error when token cache is nil")
	}
}

func TestAuthInjectorOAuth2RequiresTokenURL(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"oauth:client_id":     "cid",
		"oauth:client_secret": "csec",
	}}
	cache := agentruntime.NewOAuth2TokenCache(nil)
	injector := agentruntime.NewAuthInjector(secrets, cache)

	_, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "oauth2_client_credentials",
		SecretRef: "oauth",
	})
	if err == nil {
		t.Fatal("expected error for missing tokenURL")
	}
}

func TestAuthInjectorNoAuthWhenEmpty(t *testing.T) {
	injector := agentruntime.NewAuthInjector(nil, nil)

	result, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Headers) != 0 || len(result.EnvVars) != 0 {
		t.Fatalf("expected empty result for no auth, got headers=%v envVars=%v", result.Headers, result.EnvVars)
	}
}

func TestAuthInjectorUnsupportedProfile(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{
		"key": "val",
	}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	_, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "custom_thing",
		SecretRef: "key",
	})
	if err == nil {
		t.Fatal("expected error for unsupported profile")
	}
}

func TestAuthInjectorSecretResolutionFailure(t *testing.T) {
	secrets := testSecretResolver{values: map[string]string{}}
	injector := agentruntime.NewAuthInjector(secrets, nil)

	_, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "bearer",
		SecretRef: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	toolErr, ok := agentruntime.AsToolError(err)
	if !ok {
		t.Fatalf("expected ToolError, got %T", err)
	}
	if toolErr.Code != agentruntime.ToolCodeSecretResolution {
		t.Fatalf("expected code=%s, got %s", agentruntime.ToolCodeSecretResolution, toolErr.Code)
	}
}

func TestAuthInjectorNoResolverConfigured(t *testing.T) {
	injector := agentruntime.NewAuthInjector(nil, nil)

	_, err := injector.Resolve(context.Background(), "test_tool", resources.ToolAuth{
		Profile:   "bearer",
		SecretRef: "some-key",
	})
	if err == nil {
		t.Fatal("expected error when no secret resolver configured")
	}
}
