package resources

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

func TestToolNormalizeAuthDefaultsBearerWhenSecretRefSet(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "auth-default"},
		Spec: ToolSpec{
			Endpoint: "https://api.example.com",
			Auth:     ToolAuth{SecretRef: "my-key"},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.Auth.Profile != "bearer" {
		t.Fatalf("expected default auth.profile=bearer, got %q", tool.Spec.Auth.Profile)
	}
}

func TestToolNormalizeAuthAcceptsAllProfiles(t *testing.T) {
	profiles := []struct {
		profile    string
		headerName string
		tokenURL   string
	}{
		{"bearer", "", ""},
		{"api_key_header", "X-Key", ""},
		{"basic", "", ""},
		{"oauth2_client_credentials", "", "https://auth.example/token"},
	}
	for _, tc := range profiles {
		tool := Tool{
			APIVersion: "orloj.dev/v1",
			Kind:       "Tool",
			Metadata:   ObjectMeta{Name: "auth-" + tc.profile},
			Spec: ToolSpec{
				Endpoint: "https://api.example.com",
				Auth: ToolAuth{
					Profile:    tc.profile,
					SecretRef:  "my-secret",
					HeaderName: tc.headerName,
					TokenURL:   tc.tokenURL,
				},
			},
		}
		if err := tool.Normalize(); err != nil {
			t.Fatalf("expected valid auth profile %q to normalize, got %v", tc.profile, err)
		}
	}
}

func TestToolNormalizeAuthRejectsInvalidProfile(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "bad-profile"},
		Spec: ToolSpec{
			Endpoint: "https://api.example.com",
			Auth: ToolAuth{
				Profile:   "custom_thing",
				SecretRef: "my-secret",
			},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error for invalid auth profile")
	}
}

func TestToolNormalizeAuthRequiresSecretRef(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "no-secret"},
		Spec: ToolSpec{
			Endpoint: "https://api.example.com",
			Auth:     ToolAuth{Profile: "bearer"},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when profile set without secretRef")
	}
}

func TestToolNormalizeAuthAPIKeyHeaderRequiresHeaderName(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "no-header"},
		Spec: ToolSpec{
			Endpoint: "https://api.example.com",
			Auth: ToolAuth{
				Profile:   "api_key_header",
				SecretRef: "my-key",
			},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when api_key_header missing headerName")
	}
}

func TestToolNormalizeAuthOAuth2RequiresTokenURL(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "no-token-url"},
		Spec: ToolSpec{
			Endpoint: "https://api.example.com",
			Auth: ToolAuth{
				Profile:   "oauth2_client_credentials",
				SecretRef: "my-oauth",
			},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when oauth2_client_credentials missing tokenURL")
	}
}

func TestToolNormalizeAuthScopesAreTrimmed(t *testing.T) {
	tool := Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   ObjectMeta{Name: "scoped"},
		Spec: ToolSpec{
			Endpoint: "https://api.example.com",
			Auth: ToolAuth{
				Profile:   "oauth2_client_credentials",
				SecretRef: "my-oauth",
				TokenURL:  "https://auth.example/token",
				Scopes:    []string{"  read ", " write ", ""},
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if len(tool.Spec.Auth.Scopes) != 2 {
		t.Fatalf("expected 2 scopes after trimming, got %d: %v", len(tool.Spec.Auth.Scopes), tool.Spec.Auth.Scopes)
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

func TestToolNormalizeOperationClassesDefaults(t *testing.T) {
	tests := []struct {
		name      string
		riskLevel string
		expected  []string
	}{
		{"low risk defaults to read", "low", []string{"read"}},
		{"medium risk defaults to read", "medium", []string{"read"}},
		{"high risk defaults to write", "high", []string{"write"}},
		{"critical risk defaults to write", "critical", []string{"write"}},
		{"empty risk defaults to read (risk defaults to low)", "", []string{"read"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := Tool{Metadata: ObjectMeta{Name: "test"}, Spec: ToolSpec{RiskLevel: tt.riskLevel}}
			if err := tool.Normalize(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tool.Spec.OperationClasses) != len(tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, tool.Spec.OperationClasses)
			}
			for i, v := range tt.expected {
				if tool.Spec.OperationClasses[i] != v {
					t.Errorf("index %d: expected %q, got %q", i, v, tool.Spec.OperationClasses[i])
				}
			}
		})
	}
}

func TestToolNormalizeOperationClassesValidation(t *testing.T) {
	tool := Tool{Metadata: ObjectMeta{Name: "test"}, Spec: ToolSpec{OperationClasses: []string{"read", "execute"}}}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected invalid operation class error")
	}
}

func TestToolNormalizeOperationClassesDeduplicates(t *testing.T) {
	tool := Tool{Metadata: ObjectMeta{Name: "test"}, Spec: ToolSpec{OperationClasses: []string{"Read", "read", " READ "}}}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tool.Spec.OperationClasses) != 1 || tool.Spec.OperationClasses[0] != "read" {
		t.Fatalf("expected [read], got %v", tool.Spec.OperationClasses)
	}
}

func TestToolNormalizeOperationClassesAcceptsAll(t *testing.T) {
	tool := Tool{Metadata: ObjectMeta{Name: "test"}, Spec: ToolSpec{OperationClasses: []string{"read", "write", "delete", "admin"}}}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tool.Spec.OperationClasses) != 4 {
		t.Fatalf("expected 4 classes, got %d", len(tool.Spec.OperationClasses))
	}
}

func TestToolPermissionOperationRulesNormalization(t *testing.T) {
	p := ToolPermission{
		Metadata: ObjectMeta{Name: "test"},
		Spec: ToolPermissionSpec{
			OperationRules: []OperationRule{
				{OperationClass: " Write ", Verdict: " Deny "},
				{OperationClass: "*", Verdict: "allow"},
				{OperationClass: "", Verdict: ""},
			},
		},
	}
	if err := p.Normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Spec.OperationRules[0].OperationClass != "write" || p.Spec.OperationRules[0].Verdict != "deny" {
		t.Errorf("rule 0: expected write/deny, got %s/%s", p.Spec.OperationRules[0].OperationClass, p.Spec.OperationRules[0].Verdict)
	}
	if p.Spec.OperationRules[1].OperationClass != "*" || p.Spec.OperationRules[1].Verdict != "allow" {
		t.Errorf("rule 1: expected */allow, got %s/%s", p.Spec.OperationRules[1].OperationClass, p.Spec.OperationRules[1].Verdict)
	}
	if p.Spec.OperationRules[2].OperationClass != "*" || p.Spec.OperationRules[2].Verdict != "allow" {
		t.Errorf("rule 2: expected */allow (defaults), got %s/%s", p.Spec.OperationRules[2].OperationClass, p.Spec.OperationRules[2].Verdict)
	}
}

func TestToolPermissionOperationRulesRejectsInvalidClass(t *testing.T) {
	p := ToolPermission{
		Metadata: ObjectMeta{Name: "test"},
		Spec: ToolPermissionSpec{
			OperationRules: []OperationRule{
				{OperationClass: "execute", Verdict: "allow"},
			},
		},
	}
	if err := p.Normalize(); err == nil {
		t.Fatal("expected invalid operation class error")
	}
}

func TestToolPermissionOperationRulesRejectsInvalidVerdict(t *testing.T) {
	p := ToolPermission{
		Metadata: ObjectMeta{Name: "test"},
		Spec: ToolPermissionSpec{
			OperationRules: []OperationRule{
				{OperationClass: "write", Verdict: "block"},
			},
		},
	}
	if err := p.Normalize(); err == nil {
		t.Fatal("expected invalid verdict error")
	}
}

func TestToolApprovalNormalize(t *testing.T) {
	a := ToolApproval{
		Metadata: ObjectMeta{Name: "test-approval"},
		Spec: ToolApprovalSpec{
			TaskRef: "my-task",
			Tool:    "my-tool",
		},
	}
	if err := a.Normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.APIVersion != "orloj.dev/v1" {
		t.Errorf("expected apiVersion orloj.dev/v1, got %s", a.APIVersion)
	}
	if a.Kind != "ToolApproval" {
		t.Errorf("expected kind ToolApproval, got %s", a.Kind)
	}
	if a.Spec.TTL != "10m" {
		t.Errorf("expected ttl 10m, got %s", a.Spec.TTL)
	}
	if a.Status.Phase != "Pending" {
		t.Errorf("expected phase Pending, got %s", a.Status.Phase)
	}
	if a.Status.ExpiresAt == "" {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestToolApprovalNormalizeRequiresFields(t *testing.T) {
	tests := []struct {
		name string
		spec ToolApprovalSpec
	}{
		{"missing task_ref", ToolApprovalSpec{Tool: "t"}},
		{"missing tool", ToolApprovalSpec{TaskRef: "t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := ToolApproval{Metadata: ObjectMeta{Name: "test"}, Spec: tt.spec}
			if err := a.Normalize(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestToolApprovalNormalizeRejectsInvalidTTL(t *testing.T) {
	a := ToolApproval{
		Metadata: ObjectMeta{Name: "test"},
		Spec:     ToolApprovalSpec{TaskRef: "t", Tool: "t", TTL: "invalid"},
	}
	if err := a.Normalize(); err == nil {
		t.Fatal("expected invalid TTL error")
	}
}

func TestToolApprovalNormalizeRejectsInvalidPhase(t *testing.T) {
	a := ToolApproval{
		Metadata: ObjectMeta{Name: "test"},
		Spec:     ToolApprovalSpec{TaskRef: "t", Tool: "t"},
		Status:   ToolApprovalStatus{Phase: "Unknown"},
	}
	if err := a.Normalize(); err == nil {
		t.Fatal("expected invalid phase error")
	}
}
