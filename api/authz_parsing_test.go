package api

import (
	"strings"
	"testing"
)

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

func TestParseTokenEnvConfigMixedCaseRolesNormalize(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "bot-a:tok-a:Writer,tok-b:ADMIN,bot-c:tok-c:ReaDer")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(cfg))
	}
	if p := cfg[hashToken("tok-a")]; p.Role != "writer" {
		t.Fatalf("expected Writer to normalize to writer, got %q", p.Role)
	}
	if p := cfg[hashToken("tok-b")]; p.Role != "admin" {
		t.Fatalf("expected ADMIN to normalize to admin, got %q", p.Role)
	}
	if p := cfg[hashToken("tok-c")]; p.Role != "reader" {
		t.Fatalf("expected ReaDer to normalize to reader, got %q", p.Role)
	}
}

func TestParseTokenEnvConfigWhitespaceHandling(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "  bot : tok-ws : writer , tok-ws2 : reader ")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cfg))
	}
	p, ok := cfg[hashToken("tok-ws")]
	if !ok {
		t.Fatal("expected whitespace-padded named token to be parsed")
	}
	if p.Name != "bot" || p.Role != "writer" {
		t.Fatalf("unexpected principal: %+v", p)
	}
	if _, ok := cfg[hashToken("tok-ws2")]; !ok {
		t.Fatal("expected whitespace-padded legacy token to be parsed")
	}
}

func TestParseTokenEnvConfigEmptyAndAllWhitespaceEntries(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "bot:tok-ok:admin,  ,\t,")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 1 {
		t.Fatalf("expected 1 entry (blanks skipped), got %d", len(cfg))
	}
}

func TestParseTokenEnvConfigInvalidRolesSkipped(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "bot:tok-super:superadmin,bot2:tok-root:root,tok-empty:")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	// "tok-empty:" has empty role -> fallback to "reader" -> valid
	if len(cfg) != 1 {
		t.Fatalf("expected 1 valid entry (empty role falls back to reader), got %d", len(cfg))
	}
	p, ok := cfg[hashToken("tok-empty")]
	if !ok {
		t.Fatal("expected empty-role legacy token with reader fallback")
	}
	if p.Role != "reader" {
		t.Fatalf("expected role reader for empty role fallback, got %q", p.Role)
	}
}

func TestParseTokenEnvConfigVeryLongToken(t *testing.T) {
	long := strings.Repeat("a", 10000)
	t.Setenv("ORLOJ_API_TOKENS", "longbot:"+long+":writer")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 1 {
		t.Fatalf("expected 1 entry for long token, got %d", len(cfg))
	}
	p, ok := cfg[hashToken(long)]
	if !ok {
		t.Fatal("expected long token to be hashed and stored")
	}
	if p.Name != "longbot" || p.Role != "writer" {
		t.Fatalf("unexpected principal: %+v", p)
	}
}

func TestParseTokenEnvConfigDuplicateTokenLastWins(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "first:dup-tok:reader,second:dup-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 1 {
		t.Fatalf("expected 1 deduplicated entry, got %d", len(cfg))
	}
	p := cfg[hashToken("dup-tok")]
	if p.Name != "second" || p.Role != "admin" {
		t.Fatalf("expected last-wins semantics (second/admin), got %+v", p)
	}
}

func TestParseTokenEnvConfigColonInTokenSkipped(t *testing.T) {
	// 4+ parts means the entry is skipped
	t.Setenv("ORLOJ_API_TOKENS", "bot:tok:en:value:admin,valid-tok:reader")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 1 {
		t.Fatalf("expected 1 valid entry (colon-in-token skipped), got %d", len(cfg))
	}
	if _, ok := cfg[hashToken("valid-tok")]; !ok {
		t.Fatal("expected valid-tok to be present")
	}
}

func TestParseTokenEnvConfigCommaEdgeCases(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", ",tok-lead:reader,,tok-mid:writer,")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 2 {
		t.Fatalf("expected 2 entries (empty segments skipped), got %d", len(cfg))
	}
}

func TestParseTokenEnvConfigControllerRole(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "worker:tok-ctrl:controller")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cfg))
	}
	if p := cfg[hashToken("tok-ctrl")]; p.Role != "controller" {
		t.Fatalf("expected controller role, got %q", p.Role)
	}
}

func TestParseTokenEnvConfigBothEnvVarsTokensWins(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "bot:tok-multi:writer")
	t.Setenv("ORLOJ_API_TOKEN", "tok-single")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cfg))
	}
	if _, ok := cfg[hashToken("tok-multi")]; !ok {
		t.Fatal("expected ORLOJ_API_TOKENS to take precedence over ORLOJ_API_TOKEN")
	}
	if _, ok := cfg[hashToken("tok-single")]; ok {
		t.Fatal("ORLOJ_API_TOKEN should be ignored when ORLOJ_API_TOKENS is valid")
	}
}

func TestParseTokenEnvConfigNeitherEnvVarSet(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	cfg := parseTokenEnvConfig()
	if len(cfg) != 0 {
		t.Fatalf("expected 0 entries with no env vars, got %d", len(cfg))
	}
}
