package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUIRoutes(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	respRedirect, err := noRedirectClient.Get(server.URL + "/ui")
	if err != nil {
		t.Fatalf("get /ui failed: %v", err)
	}
	defer respRedirect.Body.Close()
	if respRedirect.StatusCode != http.StatusTemporaryRedirect {
		body, _ := io.ReadAll(respRedirect.Body)
		t.Fatalf("expected 307 for /ui redirect, got %d body=%s", respRedirect.StatusCode, string(body))
	}
	if location := respRedirect.Header.Get("Location"); location != "/ui/" {
		t.Fatalf("expected redirect to /ui/, got %q", location)
	}

	respIndex, err := http.Get(server.URL + "/ui/")
	if err != nil {
		t.Fatalf("get /ui/ failed: %v", err)
	}
	defer respIndex.Body.Close()
	body, err := io.ReadAll(respIndex.Body)
	if err != nil {
		t.Fatalf("read /ui/ body failed: %v", err)
	}

	switch respIndex.StatusCode {
	case http.StatusOK:
		html := string(body)
		if !strings.Contains(html, "id=\"root\"") {
			t.Fatalf("expected React root element in /ui/ body")
		}
		if !strings.Contains(html, "<script") {
			t.Fatalf("expected script tag in /ui/ body")
		}
	case http.StatusServiceUnavailable:
		msg := string(body)
		if !strings.Contains(msg, "frontend dist is not built") {
			t.Fatalf("expected build-required message for unbuilt UI, got body=%s", msg)
		}
	default:
		t.Fatalf("expected /ui/ status 200 or 503, got %d body=%s", respIndex.StatusCode, string(body))
	}
}

func TestUIUnbuiltModeConsistentAcrossPaths(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	respIndex, err := http.Get(server.URL + "/ui/")
	if err != nil {
		t.Fatalf("get /ui/ failed: %v", err)
	}
	defer respIndex.Body.Close()
	if respIndex.StatusCode != http.StatusServiceUnavailable {
		t.Skip("ui dist is built; skipping unbuilt-mode consistency test")
	}

	respAsset, err := http.Get(server.URL + "/ui/app.js")
	if err != nil {
		t.Fatalf("get /ui/app.js failed: %v", err)
	}
	defer respAsset.Body.Close()
	body, err := io.ReadAll(respAsset.Body)
	if err != nil {
		t.Fatalf("read /ui/app.js body failed: %v", err)
	}
	if respAsset.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /ui/app.js when dist is unbuilt, got %d body=%s", respAsset.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "frontend dist is not built") {
		t.Fatalf("expected build-required message in /ui/app.js unbuilt response")
	}
}
