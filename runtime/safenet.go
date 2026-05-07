package agentruntime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ValidateEndpointURL checks that a URL is safe for outbound requests from
// tool and MCP runtimes. It blocks dangerous URL schemes and, when the host
// is a literal IP address, rejects loopback, link-local, cloud metadata, and
// (optionally) private addresses.
//
// When the host is a hostname (not a literal IP), scheme validation still
// applies but IP-level checks are deferred to the dial layer installed by
// SafeHTTPClient / SafeDialer. Callers that build an *http.Client with
// SafeHTTPClient get dial-time enforcement automatically; callers that do
// not (e.g. code that predates SafeHTTPClient) MUST call SafeHTTPClient
// rather than relying on ValidateEndpointURL alone for SSRF protection.
//
// Pass allowPrivate=true to skip the private/internal address checks (e.g.
// for development or explicitly trusted internal services).
func ValidateEndpointURL(rawURL string, allowPrivate bool) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("empty endpoint URL")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// url.Parse("host:port") misinterprets host as the scheme when
	// there is no "://" separator. Only clear the scheme when the
	// opaque part is a pure port number (e.g. "example.com:443"),
	// so "javascript:alert(1)" still falls through to the reject.
	scheme := u.Scheme
	if scheme != "" && !strings.Contains(rawURL, "://") {
		if _, err := strconv.Atoi(u.Opaque); err == nil {
			scheme = ""
		}
	}

	switch scheme {
	case "http", "https", "grpc", "grpcs":
	case "":
		// gRPC targets often omit scheme; allow bare host:port
	default:
		return fmt.Errorf("unsupported endpoint URL scheme %q", u.Scheme)
	}

	// For URLs with a scheme, u.Hostname() extracts the host correctly.
	// For bare host:port (scheme-less), u.Hostname() returns empty because
	// url.Parse treated it as an opaque URI; extract via SplitHostPort.
	host := u.Hostname()
	if host == "" && scheme == "" {
		if h, _, splitErr := net.SplitHostPort(rawURL); splitErr == nil {
			host = h
		} else {
			host = rawURL
		}
	}
	if host == "" {
		return nil
	}

	if ip := net.ParseIP(host); ip != nil {
		return checkIP(ip, allowPrivate)
	}

	// For hostname-valued URLs, IP-level enforcement is performed at dial
	// time by SafeDialer's Control hook. This function only guarantees
	// that the URL is well-formed and uses an allowed scheme.
	return nil
}

type safeNetworkPolicy struct {
	allowPrivate  bool
	allowLoopback bool
}

func checkIP(ip net.IP, allowPrivate bool) error {
	return checkIPWithPolicy(ip, safeNetworkPolicy{allowPrivate: allowPrivate})
}

func checkIPWithPolicy(ip net.IP, policy safeNetworkPolicy) error {
	if ip == nil {
		return fmt.Errorf("nil IP address")
	}
	// Normalize IPv4-mapped IPv6 (::ffff:a.b.c.d) so that policy checks
	// use the canonical IPv4 form. Without this, an attacker could write
	// [::ffff:127.0.0.1] to slip past a naive text match; net.IP's helpers
	// already handle this, but we normalize explicitly for clarity.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() {
		if !policy.allowLoopback {
			return fmt.Errorf("loopback address %s is not allowed", ip)
		}
		return nil
	}
	// Cloud metadata check before link-local: 169.254.169.254 is in the
	// link-local range but deserves the more specific error message.
	if isCloudMetadata(ip) {
		return fmt.Errorf("cloud metadata address %s is not allowed", ip)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("link-local address %s is not allowed", ip)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("unspecified address %s is not allowed", ip)
	}
	if !policy.allowPrivate {
		if ip.IsPrivate() {
			return fmt.Errorf("private address %s is not allowed", ip)
		}
		if isCarrierGradeNAT(ip) {
			return fmt.Errorf("carrier-grade NAT address %s is not allowed", ip)
		}
	}
	return nil
}

// isCloudMetadata returns true for the well-known cloud instance metadata
// service addresses (AWS, GCP, Azure).
func isCloudMetadata(ip net.IP) bool {
	metadataAddrs := []string{
		"169.254.169.254", // AWS, GCP, Azure IMDS
		"fd00:ec2::254",   // AWS IMDSv2 IPv6
	}
	for _, addr := range metadataAddrs {
		if ip.Equal(net.ParseIP(addr)) {
			return true
		}
	}
	return false
}

// isCarrierGradeNAT returns true for addresses in the RFC 6598 100.64.0.0/10
// carrier-grade NAT range. Go's net.IP.IsPrivate does not cover this range
// but it is effectively private from an SSRF perspective.
var cgnatNet = &net.IPNet{
	IP:   net.IPv4(100, 64, 0, 0),
	Mask: net.CIDRMask(10, 32),
}

func isCarrierGradeNAT(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		return cgnatNet.Contains(v4)
	}
	return false
}

// SafeDialer returns a *net.Dialer whose Control hook enforces SSRF policy
// against the actual IP the runtime is about to connect to. This closes
// the hostname-bypass gap in ValidateEndpointURL and defends against DNS
// rebinding: even if a hostname resolves to a public IP during validation
// and a private IP at dial time, the dial is aborted.
//
// Pass allowPrivate=true to permit connections to RFC 1918 / ULA / CGNAT
// addresses (e.g. self-hosted Ollama, vLLM, LM Studio, internal services).
// Loopback, link-local, cloud metadata, and unspecified addresses are
// always blocked regardless of allowPrivate.
func SafeDialer(allowPrivate bool) *net.Dialer {
	return safeDialerWithPolicy(safeNetworkPolicy{allowPrivate: allowPrivate})
}

func safeDialerWithPolicy(policy safeNetworkPolicy) *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			// The resolver has already turned the hostname into an IP by
			// the time Control is invoked; address is "host:port" where
			// host is the literal IP family-appropriate form.
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("invalid dial address %q: %w", address, err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("dial address %q did not resolve to an IP", host)
			}
			if err := checkIPWithPolicy(ip, policy); err != nil {
				return fmt.Errorf("blocked outbound connection to %s: %w", address, err)
			}
			return nil
		},
	}
}

// Shared package-level safe HTTP clients. Callers that don't need a
// custom timeout should prefer DefaultSafeHTTPClient to avoid spinning
// up per-instance transports.
var (
	sharedSafeHTTPClientPublic  = SafeHTTPClient(false, 30*time.Second)
	sharedSafeHTTPClientPrivate = SafeHTTPClient(true, 30*time.Second)
)

// DefaultSafeHTTPClient returns a shared *http.Client whose transport
// enforces SSRF policy at dial time. Prefer this over constructing a
// fresh client when the default 30-second timeout is acceptable.
func DefaultSafeHTTPClient(allowPrivate bool) *http.Client {
	if allowPrivate {
		return sharedSafeHTTPClientPrivate
	}
	return sharedSafeHTTPClientPublic
}

// SafeHTTPClient returns an *http.Client whose transport dials through
// SafeDialer, enforcing SSRF policy on every outbound connection. Pass
// timeout<=0 to use the default 30-second timeout.
func SafeHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	return safeHTTPClientWithPolicy(safeNetworkPolicy{allowPrivate: allowPrivate}, timeout)
}

// SafeModelGatewayHTTPClient returns a safe HTTP client for trusted model
// endpoints. With allowPrivate=true it permits loopback and private model
// servers, but still blocks cloud metadata, link-local, and unspecified
// addresses.
func SafeModelGatewayHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	return safeHTTPClientWithPolicy(safeNetworkPolicy{
		allowPrivate:  allowPrivate,
		allowLoopback: allowPrivate,
	}, timeout)
}

func safeHTTPClientWithPolicy(policy safeNetworkPolicy, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer := safeDialerWithPolicy(policy)
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
