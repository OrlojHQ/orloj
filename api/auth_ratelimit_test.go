package api

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// parseTrustedProxies
// ---------------------------------------------------------------------------

func TestParseTrustedProxies_Empty(t *testing.T) {
	nets, err := parseTrustedProxies("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nets != nil {
		t.Fatalf("expected nil, got %v", nets)
	}
}

func TestParseTrustedProxies_CIDRs(t *testing.T) {
	nets, err := parseTrustedProxies("10.0.0.0/8, 172.16.0.0/12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nets) != 2 {
		t.Fatalf("expected 2 nets, got %d", len(nets))
	}
}

func TestParseTrustedProxies_SingleIP(t *testing.T) {
	nets, err := parseTrustedProxies("192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nets) != 1 {
		t.Fatalf("expected 1 net, got %d", len(nets))
	}
	if ones, _ := nets[0].Mask.Size(); ones != 32 {
		t.Fatalf("expected /32 mask, got /%d", ones)
	}
}

func TestParseTrustedProxies_Invalid(t *testing.T) {
	_, err := parseTrustedProxies("not-a-cidr")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

// ---------------------------------------------------------------------------
// extractClientIP — no trusted proxies (secure default)
// ---------------------------------------------------------------------------

func TestExtractClientIP_NoTrust_IgnoresXFF(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:4242"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 198.51.100.1")
	if got := extractClientIP(req, nil); got != "192.0.2.10" {
		t.Fatalf("expected RemoteAddr, got %q", got)
	}
}

func TestExtractClientIP_NoTrust_IgnoresXRealIP(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:4242"
	req.Header.Set("X-Real-Ip", "198.51.100.2")
	if got := extractClientIP(req, nil); got != "192.0.2.10" {
		t.Fatalf("expected RemoteAddr, got %q", got)
	}
}

func TestExtractClientIP_NoTrust_RemoteAddrHostPort(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:4242"
	if got := extractClientIP(req, nil); got != "192.0.2.10" {
		t.Fatalf("RemoteAddr host:port: got %q", got)
	}
}

func TestExtractClientIP_NoTrust_RemoteAddrWithoutPort(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.11"
	if got := extractClientIP(req, nil); got != "192.0.2.11" {
		t.Fatalf("RemoteAddr bare: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractClientIP — with trusted proxies
// ---------------------------------------------------------------------------

func mustParseCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, cidr, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("bad CIDR %q: %v", s, err)
	}
	return cidr
}

func TestExtractClientIP_TrustedPeer_SingleHopXFF(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	if got := extractClientIP(req, trusted); got != "203.0.113.5" {
		t.Fatalf("expected client IP from XFF, got %q", got)
	}
}

func TestExtractClientIP_TrustedPeer_MultiHopXFF(t *testing.T) {
	trusted := []*net.IPNet{
		mustParseCIDR(t, "10.0.0.0/8"),
		mustParseCIDR(t, "172.16.0.0/12"),
	}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	// client -> proxy1 (172.16.0.5) -> proxy2 (10.0.0.1)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 172.16.0.5")
	if got := extractClientIP(req, trusted); got != "203.0.113.5" {
		t.Fatalf("expected real client IP, got %q", got)
	}
}

func TestExtractClientIP_UntrustedPeer_IgnoresXFF(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.99:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	if got := extractClientIP(req, trusted); got != "192.0.2.99" {
		t.Fatalf("expected peer IP for untrusted peer, got %q", got)
	}
}

func TestExtractClientIP_AllTrustedChain_FallsToPeer(t *testing.T) {
	trusted := []*net.IPNet{
		mustParseCIDR(t, "10.0.0.0/8"),
		mustParseCIDR(t, "172.16.0.0/12"),
	}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 172.16.0.5")
	if got := extractClientIP(req, trusted); got != "10.0.0.1" {
		t.Fatalf("expected peer IP when all XFF entries trusted, got %q", got)
	}
}

func TestExtractClientIP_TrustedPeer_XRealIP(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Real-Ip", "198.51.100.2")
	if got := extractClientIP(req, trusted); got != "198.51.100.2" {
		t.Fatalf("expected X-Real-IP, got %q", got)
	}
}

func TestExtractClientIP_UntrustedPeer_IgnoresXRealIP(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.99:1234"
	req.Header.Set("X-Real-Ip", "198.51.100.2")
	if got := extractClientIP(req, trusted); got != "192.0.2.99" {
		t.Fatalf("expected peer IP for untrusted peer, got %q", got)
	}
}

func TestExtractClientIP_TrustedPeer_EmptyXFF_NoPeer(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	if got := extractClientIP(req, trusted); got != "10.0.0.1" {
		t.Fatalf("expected peer IP when no forwarding headers, got %q", got)
	}
}

func TestExtractClientIP_SpoofAttempt_RotatingXFF(t *testing.T) {
	// Core vulnerability test: attacker sends different X-Forwarded-For
	// values but no trusted proxies are configured.
	for _, spoofed := range []string{"1.2.3.4", "5.6.7.8", "9.10.11.12"} {
		req := mustReq(t, http.MethodPost, "/v1/auth/login", nil)
		req.RemoteAddr = "192.0.2.99:1234"
		req.Header.Set("X-Forwarded-For", spoofed)
		if got := extractClientIP(req, nil); got != "192.0.2.99" {
			t.Fatalf("spoof attempt with XFF=%q: expected RemoteAddr, got %q", spoofed, got)
		}
	}
}

// ---------------------------------------------------------------------------
// authRateLimiter
// ---------------------------------------------------------------------------

func TestAuthRateLimiter_AllowWhenIPUnknown(t *testing.T) {
	rl := &authRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rate:     rate.Every(time.Hour),
		burst:    1,
	}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = ""
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

	if !rl.allow(req) {
		t.Fatal("expected first request to be allowed")
	}
	if !rl.allow(req) {
		t.Fatal("expected second request to be allowed")
	}
	if rl.allow(req) {
		t.Fatal("expected third request to be rate limited")
	}
}

func TestAuthRateLimiter_SpoofCannotBypass(t *testing.T) {
	rl := &authRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rate:     rate.Every(24 * time.Hour),
		burst:    1,
	}
	// First request exhausts the burst.
	req1 := mustReq(t, http.MethodPost, "/v1/auth/login", nil)
	req1.RemoteAddr = "192.0.2.99:1"
	if !rl.allow(req1) {
		t.Fatal("expected first request allowed")
	}
	// Second request with a spoofed XFF should still be limited (same RemoteAddr).
	req2 := mustReq(t, http.MethodPost, "/v1/auth/login", nil)
	req2.RemoteAddr = "192.0.2.99:1"
	req2.Header.Set("X-Forwarded-For", "1.2.3.4")
	if rl.allow(req2) {
		t.Fatal("spoofed XFF should not bypass rate limit when no trusted proxies configured")
	}
}

// ---------------------------------------------------------------------------
// isSecureRequest
// ---------------------------------------------------------------------------

func TestIsSecureRequest_NoTrust_IgnoresProtoHeader(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:4242"
	req.Header.Set("X-Forwarded-Proto", "https")
	if isSecureRequest(req, nil) {
		t.Fatal("expected false when no trusted proxies")
	}
}

func TestIsSecureRequest_TrustedPeer_ReadsProtoHeader(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-Proto", "https")
	if !isSecureRequest(req, trusted) {
		t.Fatal("expected true when trusted peer sends X-Forwarded-Proto: https")
	}
}

func TestIsSecureRequest_UntrustedPeer_IgnoresProtoHeader(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	req := mustReq(t, http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.99:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	if isSecureRequest(req, trusted) {
		t.Fatal("expected false for untrusted peer even with header")
	}
}

func TestIsSecureRequest_TLSAlwaysSecure(t *testing.T) {
	req := mustReq(t, http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{}
	if !isSecureRequest(req, nil) {
		t.Fatal("expected true when TLS is set")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustReq(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
