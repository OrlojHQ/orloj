package api

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// authRateLimiter enforces per-IP rate limiting on authentication endpoints
// to prevent brute-force and credential-stuffing attacks.
type authRateLimiter struct {
	mu             sync.Mutex
	limiters       map[string]*rateLimiterEntry
	rate           rate.Limit
	burst          int
	trustedProxies []*net.IPNet
	warnOnce       sync.Once
	logger         *log.Logger
	lastCleanup    time.Time
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newAuthRateLimiter(trustedProxies []*net.IPNet, logger *log.Logger) *authRateLimiter {
	rl := &authRateLimiter{
		limiters:       make(map[string]*rateLimiterEntry),
		rate:           rate.Limit(10.0 / 60.0), // 10 requests per minute sustained
		burst:          20,                      // allow short bursts for legitimate multi-step flows
		trustedProxies: trustedProxies,
		logger:         logger,
		lastCleanup:    time.Now(),
	}
	return rl
}

func (rl *authRateLimiter) allow(r *http.Request) bool {
	ip := extractClientIP(r, rl.trustedProxies)
	if ip == "" {
		return true
	}

	if len(rl.trustedProxies) == 0 && hasForwardingHeaders(r) {
		rl.warnOnce.Do(func() {
			if rl.logger != nil {
				rl.logger.Printf("WARNING: X-Forwarded-For/X-Real-IP header present but --trusted-proxies is not configured; forwarding headers are ignored for rate limiting. Set --trusted-proxies to the CIDR(s) of your reverse proxy to enable per-client IP extraction.")
			}
		})
	}

	rl.mu.Lock()
	rl.cleanupLocked(time.Now())
	entry, ok := rl.limiters[ip]
	if !ok {
		entry = &rateLimiterEntry{
			limiter: rate.NewLimiter(rl.rate, rl.burst),
		}
		rl.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	rl.mu.Unlock()

	return entry.limiter.Allow()
}

func (rl *authRateLimiter) cleanupLocked(now time.Time) {
	if rl == nil {
		return
	}
	if !rl.lastCleanup.IsZero() && now.Sub(rl.lastCleanup) < 5*time.Minute {
		return
	}
	cutoff := now.Add(-10 * time.Minute)
	for ip, entry := range rl.limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(rl.limiters, ip)
		}
	}
	rl.lastCleanup = now
}

// extractClientIP determines the real client IP for rate-limiting purposes.
//
// When trustedProxies is nil or empty, forwarding headers (X-Forwarded-For,
// X-Real-IP) are ignored and RemoteAddr is used directly. This is the secure
// default that prevents spoofed-header bypass of per-IP rate limits.
//
// When trustedProxies is configured, the function checks whether the
// immediate peer (RemoteAddr) is in the trusted set. If not, RemoteAddr is
// returned (untrusted peers cannot influence IP extraction via headers).
// If the peer is trusted, X-Forwarded-For is walked right-to-left, skipping
// entries that match trusted CIDRs, and the first untrusted entry is
// returned as the real client IP. If X-Forwarded-For is absent or all
// entries are trusted, X-Real-IP is checked, then RemoteAddr is used.
func extractClientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	peer := peerIP(r)
	if len(trustedProxies) == 0 {
		return peer
	}

	peerAddr := net.ParseIP(peer)
	if peerAddr == nil {
		return peer
	}
	if !isTrustedProxy(peerAddr, trustedProxies) {
		return peer
	}

	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			entry := strings.TrimSpace(parts[i])
			if entry == "" {
				continue
			}
			ip := net.ParseIP(entry)
			if ip == nil {
				return entry
			}
			if !isTrustedProxy(ip, trustedProxies) {
				return entry
			}
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-Ip")); xri != "" {
		return xri
	}
	return peer
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func hasForwardingHeaders(r *http.Request) bool {
	return r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-Ip") != ""
}

func isTrustedProxy(ip net.IP, trusted []*net.IPNet) bool {
	for _, cidr := range trusted {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// parseTrustedProxies parses a comma-separated list of CIDRs into a slice
// of *net.IPNet. Single IP addresses without a prefix length are treated
// as /32 (IPv4) or /128 (IPv6). An empty input returns nil, nil.
func parseTrustedProxies(raw string) ([]*net.IPNet, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	nets := make([]*net.IPNet, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			ip := net.ParseIP(p)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy address %q", p)
			}
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			nets = append(nets, &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(bits, bits),
			})
			continue
		}
		_, cidr, err := net.ParseCIDR(p)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", p, err)
		}
		nets = append(nets, cidr)
	}
	return nets, nil
}

// isTrustedPeer checks whether the immediate peer (RemoteAddr) of a request
// is in the trusted proxy set. Exported for use by isSecureRequest.
func isTrustedPeer(r *http.Request, trustedProxies []*net.IPNet) bool {
	if len(trustedProxies) == 0 {
		return false
	}
	ip := net.ParseIP(peerIP(r))
	if ip == nil {
		return false
	}
	return isTrustedProxy(ip, trustedProxies)
}

// ipRateLimiter is a generic per-IP rate limiter reusable across endpoints.
type ipRateLimiter struct {
	mu             sync.Mutex
	limiters       map[string]*rateLimiterEntry
	rate           rate.Limit
	burst          int
	trustedProxies []*net.IPNet
	lastCleanup    time.Time
}

func newIPRateLimiter(r rate.Limit, burst int, trustedProxies []*net.IPNet) *ipRateLimiter {
	return &ipRateLimiter{
		limiters:       make(map[string]*rateLimiterEntry),
		rate:           r,
		burst:          burst,
		trustedProxies: trustedProxies,
		lastCleanup:    time.Now(),
	}
}

func (rl *ipRateLimiter) Allow(r *http.Request) bool {
	ip := extractClientIP(r, rl.trustedProxies)
	if ip == "" {
		return true
	}
	rl.mu.Lock()
	now := time.Now()
	if now.Sub(rl.lastCleanup) >= 5*time.Minute {
		cutoff := now.Add(-10 * time.Minute)
		for k, e := range rl.limiters {
			if e.lastSeen.Before(cutoff) {
				delete(rl.limiters, k)
			}
		}
		rl.lastCleanup = now
	}
	entry, ok := rl.limiters[ip]
	if !ok {
		entry = &rateLimiterEntry{
			limiter: rate.NewLimiter(rl.rate, rl.burst),
		}
		rl.limiters[ip] = entry
	}
	entry.lastSeen = now
	rl.mu.Unlock()
	return entry.limiter.Allow()
}
