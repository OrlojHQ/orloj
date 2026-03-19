package main

import (
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/api"
)

func TestParseAuthModeAcceptsOSSModes(t *testing.T) {
	mode, err := parseAuthMode("off")
	if err != nil {
		t.Fatalf("expected off mode to be accepted: %v", err)
	}
	if mode != api.AuthModeOff {
		t.Fatalf("expected mode off, got %q", mode)
	}

	mode, err = parseAuthMode("LOCAL")
	if err != nil {
		t.Fatalf("expected local mode to be accepted: %v", err)
	}
	if mode != api.AuthModeLocal {
		t.Fatalf("expected mode local, got %q", mode)
	}
}

func TestParseAuthModeRejectsSSOInOSS(t *testing.T) {
	mode, err := parseAuthMode("sso")
	if mode != api.AuthModeSSO {
		t.Fatalf("expected parsed mode sso, got %q", mode)
	}
	if err == nil {
		t.Fatalf("expected sso mode to be rejected in OSS")
	}
	if !strings.Contains(err.Error(), "requires enterprise build/adapter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAuthModeRejectsInvalidMode(t *testing.T) {
	_, err := parseAuthMode("bad-mode")
	if err == nil {
		t.Fatalf("expected invalid mode to fail")
	}
	if !strings.Contains(err.Error(), "invalid auth mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}
