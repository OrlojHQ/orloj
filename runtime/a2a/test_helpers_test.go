package a2a

import (
	"net/http"
	"time"
)

// newTestClient creates a Client that allows loopback connections,
// suitable for use with httptest.NewServer in tests. The standard
// NewClient constructor blocks loopback at the URL-validation layer
// even with AllowPrivate=true (SSRF defence-in-depth).
func newTestClient(opts ...func(*Client)) *Client {
	c := &Client{
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		allowPrivate:      true,
		skipURLValidation: true,
		cardCache:         make(map[string]*cachedCard),
		cardCacheTTL:      5 * time.Minute,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
