package agentruntime

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateEndpointURL checks that a URL is safe for outbound requests from
// tool and MCP runtimes. It blocks dangerous URL schemes and, when the host
// is a literal IP address, rejects loopback, link-local, cloud metadata, and
// (optionally) private addresses.
//
// When the host is a hostname (not a literal IP), scheme validation still
// applies but IP-level checks are deferred to the transport layer.
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

	switch u.Scheme {
	case "http", "https", "grpc", "grpcs":
	case "":
		// gRPC targets often omit scheme; allow bare host:port
	default:
		return fmt.Errorf("unsupported endpoint URL scheme %q", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return nil
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}

	return checkIP(ip, allowPrivate)
}

func checkIP(ip net.IP, allowPrivate bool) error {
	if ip.IsLoopback() {
		return fmt.Errorf("loopback address %s is not allowed", ip)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("link-local address %s is not allowed", ip)
	}
	if isCloudMetadata(ip) {
		return fmt.Errorf("cloud metadata address %s is not allowed", ip)
	}
	if !allowPrivate && ip.IsPrivate() {
		return fmt.Errorf("private address %s is not allowed", ip)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("unspecified address %s is not allowed", ip)
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
