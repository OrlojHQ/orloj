package api

import "testing"

func TestParseTokenEnvConfigSupportsNamedAndLegacyFormats(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "ops-bot:token-1:writer,token-2:reader,broken-entry, :token-3:admin,token-4:badrole")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 2 {
		t.Fatalf("expected 2 valid token entries, got %d", len(cfg))
	}
	principal, ok := cfg[hashToken("token-1")]
	if !ok {
		t.Fatal("expected named token to be parsed")
	}
	if principal.Name != "ops-bot" || principal.Role != "writer" {
		t.Fatalf("unexpected principal for named token: %+v", principal)
	}
	legacy, ok := cfg[hashToken("token-2")]
	if !ok {
		t.Fatal("expected legacy token to be parsed")
	}
	if legacy.Name != "" || legacy.Role != "reader" {
		t.Fatalf("unexpected principal for legacy token: %+v", legacy)
	}
}

func TestParseTokenEnvConfigFallsBackToSingleToken(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "single-admin-token")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 1 {
		t.Fatalf("expected 1 fallback token, got %d", len(cfg))
	}
	principal, ok := cfg[hashToken("single-admin-token")]
	if !ok {
		t.Fatal("expected single token to be present")
	}
	if principal.Role != "admin" {
		t.Fatalf("expected fallback role admin, got %q", principal.Role)
	}
}
