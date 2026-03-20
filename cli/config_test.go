package cli

import (
	"os"
	"testing"
)

func TestDefaultServerResolved_EnvPrecedence(t *testing.T) {
	t.Setenv("ORLOJCTL_SERVER", "")
	t.Setenv("ORLOJ_SERVER", "")
	cfg := &orlojctlConfig{
		CurrentProfile: "p",
		Profiles: map[string]profileEntry{
			"p": {Server: "https://from-profile.example"},
		},
	}
	if got := defaultServerResolved(cfg); got != "https://from-profile.example" {
		t.Fatalf("profile server: got %q", got)
	}

	t.Setenv("ORLOJ_SERVER", "https://from-env.example")
	if got := defaultServerResolved(cfg); got != "https://from-env.example" {
		t.Fatalf("ORLOJ_SERVER: got %q", got)
	}

	t.Setenv("ORLOJCTL_SERVER", "https://ctl.example")
	if got := defaultServerResolved(cfg); got != "https://ctl.example" {
		t.Fatalf("ORLOJCTL_SERVER: got %q", got)
	}
}

func TestDefaultServerResolved_Fallback(t *testing.T) {
	t.Setenv("ORLOJCTL_SERVER", "")
	t.Setenv("ORLOJ_SERVER", "")
	if got := defaultServerResolved(&orlojctlConfig{}); got != fallbackServer {
		t.Fatalf("fallback: got %q want %q", got, fallbackServer)
	}
}

func TestTokenFromProfile(t *testing.T) {
	t.Setenv("MY_TOK", "secret-from-env")
	cfg := &orlojctlConfig{
		CurrentProfile: "p",
		Profiles: map[string]profileEntry{
			"p": {TokenEnv: "MY_TOK"},
		},
	}
	if got := tokenFromProfile(cfg); got != "secret-from-env" {
		t.Fatalf("token_env: got %q", got)
	}

	cfg.Profiles["p"] = profileEntry{Token: "inline", TokenEnv: "MY_TOK"}
	if got := tokenFromProfile(cfg); got != "inline" {
		t.Fatalf("inline token should win: got %q", got)
	}
}

func TestTokenFromProfile_MissingEnv(t *testing.T) {
	_ = os.Unsetenv("MISSING_TOK_XYZ")
	cfg := &orlojctlConfig{
		CurrentProfile: "p",
		Profiles: map[string]profileEntry{
			"p": {TokenEnv: "MISSING_TOK_XYZ"},
		},
	}
	if got := tokenFromProfile(cfg); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}
