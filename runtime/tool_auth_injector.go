package agentruntime

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// AuthResult holds resolved authentication artifacts for tool backends.
// Headers are used by HTTP-based backends; EnvVars are used by the container backend.
type AuthResult struct {
	Headers map[string]string
	EnvVars map[string]string
	Profile string
}

// AuthInjector centralizes auth resolution for all tool runtime backends.
// It resolves Tool.spec.auth into concrete headers/env based on the auth profile.
type AuthInjector struct {
	secrets    SecretResolver
	tokenCache *OAuth2TokenCache
}

func NewAuthInjector(secrets SecretResolver, tokenCache *OAuth2TokenCache) *AuthInjector {
	return &AuthInjector{
		secrets:    secrets,
		tokenCache: tokenCache,
	}
}

// Resolve produces an AuthResult for the given tool auth config.
// Returns an empty AuthResult (no error) when no auth is configured.
func (a *AuthInjector) Resolve(ctx context.Context, toolName string, auth resources.ToolAuth) (AuthResult, error) {
	profile := strings.TrimSpace(auth.Profile)
	secretRef := strings.TrimSpace(auth.SecretRef)

	if profile == "" && secretRef == "" {
		return AuthResult{}, nil
	}
	if profile == "" {
		profile = "bearer"
	}

	switch profile {
	case "bearer":
		return a.resolveBearer(ctx, toolName, secretRef)
	case "api_key_header":
		return a.resolveAPIKeyHeader(ctx, toolName, secretRef, strings.TrimSpace(auth.HeaderName))
	case "basic":
		return a.resolveBasic(ctx, toolName, secretRef)
	case "oauth2_client_credentials":
		return a.resolveOAuth2(ctx, toolName, auth)
	default:
		return AuthResult{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s unsupported auth profile %q", toolName, profile),
			nil,
			map[string]string{"tool": toolName, "auth_profile": profile},
		)
	}
}

func (a *AuthInjector) resolveSecret(ctx context.Context, toolName, secretRef string) (string, error) {
	if a.secrets == nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeSecretResolution,
			ToolReasonSecretResolution,
			false,
			fmt.Sprintf("tool=%s has auth.secretRef but no secret resolver is configured", toolName),
			ErrToolSecretResolution,
			map[string]string{"tool": toolName},
		)
	}
	value, err := a.secrets.Resolve(ctx, secretRef)
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeSecretResolution,
			ToolReasonSecretResolution,
			false,
			fmt.Sprintf("tool=%s secretRef=%s resolution failed", toolName, secretRef),
			fmt.Errorf("%w: %v", ErrToolSecretResolution, err),
			map[string]string{"tool": toolName, "secret_ref": secretRef},
		)
	}
	return value, nil
}

func (a *AuthInjector) resolveBearer(ctx context.Context, toolName, secretRef string) (AuthResult, error) {
	value, err := a.resolveSecret(ctx, toolName, secretRef)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		Profile: "bearer",
		Headers: map[string]string{"Authorization": "Bearer " + value},
		EnvVars: map[string]string{"TOOL_AUTH_BEARER": value},
	}, nil
}

func (a *AuthInjector) resolveAPIKeyHeader(ctx context.Context, toolName, secretRef, headerName string) (AuthResult, error) {
	if headerName == "" {
		return AuthResult{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s auth.profile=api_key_header requires headerName", toolName),
			nil,
			map[string]string{"tool": toolName},
		)
	}
	value, err := a.resolveSecret(ctx, toolName, secretRef)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		Profile: "api_key_header",
		Headers: map[string]string{headerName: value},
		EnvVars: map[string]string{
			"TOOL_AUTH_HEADER_NAME":  headerName,
			"TOOL_AUTH_HEADER_VALUE": value,
		},
	}, nil
}

func (a *AuthInjector) resolveBasic(ctx context.Context, toolName, secretRef string) (AuthResult, error) {
	value, err := a.resolveSecret(ctx, toolName, secretRef)
	if err != nil {
		return AuthResult{}, err
	}
	if !strings.Contains(value, ":") {
		return AuthResult{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s auth.profile=basic secret must contain username:password", toolName),
			nil,
			map[string]string{"tool": toolName},
		)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	return AuthResult{
		Profile: "basic",
		Headers: map[string]string{"Authorization": "Basic " + encoded},
		EnvVars: map[string]string{"TOOL_AUTH_BASIC": encoded},
	}, nil
}

func (a *AuthInjector) resolveOAuth2(ctx context.Context, toolName string, auth resources.ToolAuth) (AuthResult, error) {
	tokenURL := strings.TrimSpace(auth.TokenURL)
	if tokenURL == "" {
		return AuthResult{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s auth.profile=oauth2_client_credentials requires tokenURL", toolName),
			nil,
			map[string]string{"tool": toolName},
		)
	}

	secretRef := strings.TrimSpace(auth.SecretRef)
	clientID, err := a.resolveSecret(ctx, toolName, secretRef+":client_id")
	if err != nil {
		return AuthResult{}, err
	}
	clientSecret, err := a.resolveSecret(ctx, toolName, secretRef+":client_secret")
	if err != nil {
		return AuthResult{}, err
	}

	scope := strings.Join(auth.Scopes, " ")

	if a.tokenCache == nil {
		return AuthResult{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s oauth2 requires token cache but none configured", toolName),
			nil,
			map[string]string{"tool": toolName},
		)
	}

	token, err := a.tokenCache.GetToken(ctx, tokenURL, clientID, clientSecret, scope)
	if err != nil {
		return AuthResult{}, NewToolError(
			ToolStatusError,
			ToolCodeSecretResolution,
			ToolReasonSecretResolution,
			true,
			fmt.Sprintf("tool=%s oauth2 token exchange failed", toolName),
			err,
			map[string]string{"tool": toolName, "token_url": tokenURL},
		)
	}

	return AuthResult{
		Profile: "oauth2_client_credentials",
		Headers: map[string]string{"Authorization": "Bearer " + token},
		EnvVars: map[string]string{"TOOL_AUTH_BEARER": token},
	}, nil
}

// EvictOAuth2Token removes a cached OAuth2 token, used on 401 responses.
func (a *AuthInjector) EvictOAuth2Token(tokenURL, clientID string) {
	if a != nil && a.tokenCache != nil {
		a.tokenCache.Evict(tokenURL, clientID)
	}
}
