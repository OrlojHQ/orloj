package agentruntime

import (
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestValidateEndpointURLSchemes(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty", "", true},
		{"http literal public", "http://93.184.216.34", false},
		{"https literal public", "https://93.184.216.34", false},
		{"grpc no scheme", "example.com:443", false},
		{"file scheme", "file:///etc/passwd", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"loopback literal", "http://127.0.0.1", true},
		{"loopback v6", "http://[::1]", true},
		{"aws metadata", "http://169.254.169.254/latest/meta-data", true},
		{"gcp metadata alias", "http://169.254.169.254/computeMetadata/v1", true},
		{"rfc1918", "http://10.0.0.5", true},
		{"rfc1918 192", "http://192.168.1.1", true},
		{"rfc1918 172", "http://172.16.0.1", true},
		{"ipv4-mapped loopback", "http://[::ffff:127.0.0.1]", true},
		{"cgnat", "http://100.64.1.2", true},
		{"link-local", "http://169.254.1.2", true},
		{"unspecified", "http://0.0.0.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEndpointURL(tc.url, false)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.url, err)
			}
		})
	}
}

func TestValidateEndpointURLAllowPrivate(t *testing.T) {
	// With allowPrivate=true, RFC1918 / ULA / CGNAT pass but loopback
	// and cloud metadata still fail.
	if err := ValidateEndpointURL("http://10.0.0.5", true); err != nil {
		t.Fatalf("expected RFC1918 to be allowed with allowPrivate=true: %v", err)
	}
	if err := ValidateEndpointURL("http://192.168.1.1", true); err != nil {
		t.Fatalf("expected 192.168.x to be allowed with allowPrivate=true: %v", err)
	}
	if err := ValidateEndpointURL("http://127.0.0.1", true); err == nil {
		t.Fatal("expected loopback to remain blocked even with allowPrivate=true")
	}
	if err := ValidateEndpointURL("http://169.254.169.254", true); err == nil {
		t.Fatal("expected cloud metadata to remain blocked even with allowPrivate=true")
	}
}

func TestCheckIPMappedIPv6Loopback(t *testing.T) {
	// ::ffff:127.0.0.1 should be recognized as loopback.
	ip := net.ParseIP("::ffff:127.0.0.1")
	if ip == nil {
		t.Fatal("failed to parse IPv4-mapped IPv6 address")
	}
	if err := checkIP(ip, true); err == nil {
		t.Fatal("expected IPv4-mapped loopback to be blocked")
	}
}

func TestSafeDialerControlBlocksLoopback(t *testing.T) {
	dialer := SafeDialer(false)
	err := dialer.Control("tcp", "127.0.0.1:80", fakeRawConn{})
	if err == nil {
		t.Fatal("expected Control hook to block loopback")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}

func TestSafeDialerControlBlocksMetadata(t *testing.T) {
	dialer := SafeDialer(true) // allowPrivate=true should NOT allow metadata
	err := dialer.Control("tcp", "169.254.169.254:80", fakeRawConn{})
	if err == nil {
		t.Fatal("expected Control hook to block cloud metadata even with allowPrivate=true")
	}
	if !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("expected metadata error, got %v", err)
	}
}

func TestSafeDialerControlAllowsPublic(t *testing.T) {
	dialer := SafeDialer(false)
	if err := dialer.Control("tcp", "93.184.216.34:443", fakeRawConn{}); err != nil {
		t.Fatalf("expected public address to be allowed: %v", err)
	}
}

func TestSafeDialerControlAllowsPrivateWhenFlagged(t *testing.T) {
	dialer := SafeDialer(true)
	if err := dialer.Control("tcp", "10.0.0.5:80", fakeRawConn{}); err != nil {
		t.Fatalf("expected RFC1918 to be allowed with allowPrivate=true: %v", err)
	}
}

func TestModelGatewaySafeDialerAllowsLoopbackOnlyWhenPrivate(t *testing.T) {
	publicPolicy := safeNetworkPolicy{}
	publicDialer := safeDialerWithPolicy(publicPolicy)
	if err := publicDialer.Control("tcp", "127.0.0.1:11434", fakeRawConn{}); err == nil {
		t.Fatal("expected model gateway public policy to block loopback")
	}

	privatePolicy := safeNetworkPolicy{allowPrivate: true, allowLoopback: true}
	privateDialer := safeDialerWithPolicy(privatePolicy)
	if err := privateDialer.Control("tcp", "127.0.0.1:11434", fakeRawConn{}); err != nil {
		t.Fatalf("expected trusted model gateway policy to allow loopback: %v", err)
	}
	if err := privateDialer.Control("tcp", "10.0.0.5:8000", fakeRawConn{}); err != nil {
		t.Fatalf("expected trusted model gateway policy to allow RFC1918: %v", err)
	}
	if err := privateDialer.Control("tcp", "169.254.169.254:80", fakeRawConn{}); err == nil {
		t.Fatal("expected trusted model gateway policy to block metadata")
	}
	if err := privateDialer.Control("tcp", "169.254.1.2:80", fakeRawConn{}); err == nil {
		t.Fatal("expected trusted model gateway policy to block link-local")
	}
	if err := privateDialer.Control("tcp", "0.0.0.0:80", fakeRawConn{}); err == nil {
		t.Fatal("expected trusted model gateway policy to block unspecified")
	}
}

func TestSafeHTTPClientUsesSafeDialer(t *testing.T) {
	client := SafeHTTPClient(false, 0)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// We can't easily trigger a dial against a private IP without
	// the risk of hanging; rely on SafeDialer tests for the policy
	// checks and confirm here that the helper returns a real client
	// with a non-nil Transport.
	if client.Transport == nil {
		t.Fatal("expected transport to be configured")
	}
}

func TestDefaultSafeHTTPClientIsShared(t *testing.T) {
	a := DefaultSafeHTTPClient(false)
	b := DefaultSafeHTTPClient(false)
	if a != b {
		t.Fatal("expected DefaultSafeHTTPClient to return a shared instance per policy")
	}
	if DefaultSafeHTTPClient(true) == DefaultSafeHTTPClient(false) {
		t.Fatal("expected private and public clients to differ")
	}
}

// fakeRawConn implements syscall.RawConn with no-op methods. SafeDialer's
// Control hook never calls through to it, but the signature requires a
// value of the interface type.
type fakeRawConn struct{}

func (fakeRawConn) Control(func(fd uintptr)) error           { return nil }
func (fakeRawConn) Read(func(fd uintptr) (done bool)) error  { return nil }
func (fakeRawConn) Write(func(fd uintptr) (done bool)) error { return nil }

var _ syscall.RawConn = fakeRawConn{}
