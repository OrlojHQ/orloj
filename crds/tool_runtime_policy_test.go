package crds

import "testing"

func TestParseToolManifestRuntimePolicyYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: web-search
spec:
  type: http
  endpoint: https://api.search.example
  capabilities:
    - web.read
    - docs.search
  risk_level: medium
  runtime:
    timeout: 2s
    isolation_mode: none
    retry:
      max_attempts: 3
      backoff: 100ms
      max_backoff: 2s
      jitter: equal
  auth:
    secretRef: search-key
`)

	tool, err := ParseToolManifest(raw)
	if err != nil {
		t.Fatalf("parse tool manifest failed: %v", err)
	}
	if tool.Spec.RiskLevel != "medium" {
		t.Fatalf("expected risk_level=medium, got %q", tool.Spec.RiskLevel)
	}
	if len(tool.Spec.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(tool.Spec.Capabilities))
	}
	if tool.Spec.Runtime.Timeout != "2s" {
		t.Fatalf("expected runtime.timeout=2s, got %q", tool.Spec.Runtime.Timeout)
	}
	if tool.Spec.Runtime.IsolationMode != "none" {
		t.Fatalf("expected runtime.isolation_mode=none, got %q", tool.Spec.Runtime.IsolationMode)
	}
	if tool.Spec.Runtime.Retry.MaxAttempts != 3 {
		t.Fatalf("expected runtime.retry.max_attempts=3, got %d", tool.Spec.Runtime.Retry.MaxAttempts)
	}
	if tool.Spec.Runtime.Retry.Backoff != "100ms" {
		t.Fatalf("expected runtime.retry.backoff=100ms, got %q", tool.Spec.Runtime.Retry.Backoff)
	}
	if tool.Spec.Runtime.Retry.MaxBackoff != "2s" {
		t.Fatalf("expected runtime.retry.max_backoff=2s, got %q", tool.Spec.Runtime.Retry.MaxBackoff)
	}
	if tool.Spec.Runtime.Retry.Jitter != "equal" {
		t.Fatalf("expected runtime.retry.jitter=equal, got %q", tool.Spec.Runtime.Retry.Jitter)
	}
}

func TestToolNormalizeHighRiskDefaultsToSandboxedIsolation(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "db-write"},
		Spec: ToolSpec{
			Type:      "http",
			Endpoint:  "https://db.example",
			RiskLevel: "high",
		},
	}

	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.Runtime.IsolationMode != "sandboxed" {
		t.Fatalf("expected high-risk default isolation_mode=sandboxed, got %q", tool.Spec.Runtime.IsolationMode)
	}
	if tool.Spec.Runtime.Timeout != "30s" {
		t.Fatalf("expected default runtime.timeout=30s, got %q", tool.Spec.Runtime.Timeout)
	}
}

func TestToolNormalizeAcceptsValidToolTypes(t *testing.T) {
	validTypes := []string{"http", "external", "grpc", "queue", "webhook-callback", "HTTP", "External", ""}
	for _, toolType := range validTypes {
		tool := Tool{
			APIVersion: "orloj.dev/v1",
			Kind:       "Tool",
			Metadata:   ObjectMeta{Name: "valid-type"},
			Spec: ToolSpec{
				Type:     toolType,
				Endpoint: "https://api.example.com",
			},
		}
		if err := tool.Normalize(); err != nil {
			t.Fatalf("expected valid tool type %q to normalize, got %v", toolType, err)
		}
	}
}

func TestToolNormalizeRejectsInvalidToolType(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "bad-type"},
		Spec: ToolSpec{
			Type:     "ftp",
			Endpoint: "ftp://example.com",
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected invalid tool type normalization error")
	}
}

func TestToolNormalizeDefaultsEmptyTypeToHTTP(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "default-type"},
		Spec: ToolSpec{
			Endpoint: "https://api.example.com",
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.Type != "http" {
		t.Fatalf("expected default type=http, got %q", tool.Spec.Type)
	}
}

func TestToolNormalizeRejectsInvalidRetryJitter(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "bad-jitter"},
		Spec: ToolSpec{
			Runtime: ToolRuntimePolicy{
				Retry: ToolRetryPolicy{
					Jitter: "randomized",
				},
			},
		},
	}

	if err := tool.Normalize(); err == nil {
		t.Fatal("expected invalid jitter normalization error")
	}
}
