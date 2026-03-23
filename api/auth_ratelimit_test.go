package api

import (
	"io"
	"net/http"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestExtractClientIP_XForwardedFor(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "  203.0.113.5 , 198.51.100.1")
	if got := extractClientIP(req); got != "203.0.113.5" {
		t.Fatalf("X-Forwarded-For: got %q", got)
	}
}

func TestExtractClientIP_XRealIP(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.Header.Set("X-Real-Ip", " 198.51.100.2 ")
	if got := extractClientIP(req); got != "198.51.100.2" {
		t.Fatalf("X-Real-Ip: got %q", got)
	}
}

func TestExtractClientIP_RemoteAddrHostPort(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:4242"
	if got := extractClientIP(req); got != "192.0.2.10" {
		t.Fatalf("RemoteAddr host:port: got %q", got)
	}
}

func TestExtractClientIP_RemoteAddrWithoutPort(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.11"
	if got := extractClientIP(req); got != "192.0.2.11" {
		t.Fatalf("RemoteAddr bare: got %q", got)
	}
}

func TestAuthRateLimiter_AllowWhenIPUnknown(t *testing.T) {
	rl := &authRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rate:     rate.Every(time.Hour),
		burst:    1,
	}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = ""
	// extractClientIP returns "" -> allow returns true without limiting
	if !rl.allow(req) {
		t.Fatal("expected allow when client IP is empty")
	}
}

func TestAuthRateLimiter_ExhaustsBurst(t *testing.T) {
	rl := &authRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rate:     rate.Every(24 * time.Hour),
		burst:    2,
	}
	req := mustReq(t, http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.99:1"

	if !rl.allow(req) || !rl.allow(req) {
		t.Fatal("expected first two requests to be allowed")
	}
	if rl.allow(req) {
		t.Fatal("expected third request to be rate limited")
	}
}

func mustReq(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
