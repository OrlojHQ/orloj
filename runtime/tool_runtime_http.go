package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// defaultToolHTTPClient returns the shared safe HTTP client that enforces
// SSRF policy at dial time. The returned client matches the requested
// allowPrivate policy so that call-time validation and dial-time
// enforcement agree.
func defaultToolHTTPClient(allowPrivate bool) *http.Client {
	return DefaultSafeHTTPClient(allowPrivate)
}

// HTTPToolClient executes tools via HTTP POST against Tool.spec.endpoint.
// It replaces MockToolClient as the base runtime for isolation_mode=none.
type HTTPToolClient struct {
	registry       ToolCapabilityRegistry
	secrets        SecretResolver
	authInjector   *AuthInjector
	client         HTTPDoer
	clientInjected bool // true when caller passed a custom HTTPDoer
	allowPrivate   bool // allow requests to private/internal IPs (for dev)
}

// HTTPDoer abstracts HTTP request execution for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewHTTPToolClient(registry ToolCapabilityRegistry, secrets SecretResolver, client HTTPDoer) *HTTPToolClient {
	injected := client != nil
	if !injected {
		client = defaultToolHTTPClient(false)
	}
	return &HTTPToolClient{
		registry:       registry,
		secrets:        secrets,
		authInjector:   NewAuthInjector(secrets, nil),
		client:         client,
		clientInjected: injected,
	}
}

// SetAllowPrivateEndpoints permits HTTP tool calls to private/internal IP
// ranges (RFC 1918). Loopback and cloud metadata addresses are always blocked.
// When the runtime is using its internally-built safe HTTP client, this
// swaps the client for one whose dial-time policy matches, so call-time
// validation and dial-time enforcement stay consistent.
func (r *HTTPToolClient) SetAllowPrivateEndpoints(allow bool) {
	r.allowPrivate = allow
	if !r.clientInjected {
		r.client = defaultToolHTTPClient(allow)
	}
}

func NewHTTPToolClientWithAuth(registry ToolCapabilityRegistry, injector *AuthInjector, client HTTPDoer) *HTTPToolClient {
	injected := client != nil
	if !injected {
		client = defaultToolHTTPClient(false)
	}
	return &HTTPToolClient{
		registry:       registry,
		authInjector:   injector,
		client:         client,
		clientInjected: injected,
	}
}

func (r *HTTPToolClient) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewHTTPToolClient(registry, nil, nil)
	}
	return &HTTPToolClient{
		registry:       registry,
		secrets:        r.secrets,
		authInjector:   r.authInjector,
		client:         r.client,
		clientInjected: r.clientInjected,
		allowPrivate:   r.allowPrivate,
	}
}

func (r *HTTPToolClient) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewHTTPToolClient(nil, nil, nil)
	}
	copy := *r
	if aware, ok := copy.secrets.(namespaceAwareSecretResolver); ok {
		copy.secrets = aware.WithNamespace(namespace)
	}
	if copy.secrets != nil {
		copy.authInjector = NewAuthInjector(copy.secrets, nil)
		if r.authInjector != nil {
			copy.authInjector.tokenCache = r.authInjector.tokenCache
		}
	}
	return &copy
}

func (r *HTTPToolClient) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"missing tool name",
			ErrInvalidToolRuntimePolicy,
			map[string]string{"field": "tool"},
		)
	}
	spec, ok := r.resolveSpec(tool)
	if !ok {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeUnsupportedTool,
			ToolReasonToolUnsupported,
			false,
			fmt.Sprintf("unsupported tool %s", tool),
			ErrUnsupportedTool,
			map[string]string{"tool": tool},
		)
	}
	endpoint := strings.TrimSpace(spec.Endpoint)
	if endpoint == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s missing endpoint", tool),
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}

	if err := ValidateEndpointURL(endpoint, r.allowPrivate); err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s endpoint blocked: %s", tool, err),
			err,
			map[string]string{"tool": tool},
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte(input)))
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			false,
			fmt.Sprintf("tool=%s failed to build HTTP request: %s", tool, RedactSensitive(err.Error())),
			err,
			map[string]string{"tool": tool},
		)
	}
	req.Header.Set("Content-Type", "application/json")

	if r.authInjector != nil {
		authResult, authErr := r.authInjector.Resolve(ctx, tool, spec.Auth)
		if authErr != nil {
			return "", authErr
		}
		for k, v := range authResult.Headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", NewToolError(
				ToolStatusError,
				ToolCodeTimeout,
				ToolReasonExecutionTimeout,
				true,
				fmt.Sprintf("HTTP tool execution timed out for tool=%s", tool),
				err,
				map[string]string{"tool": tool},
			)
		}
		if errors.Is(err, context.Canceled) {
			return "", NewToolError(
				ToolStatusError,
				ToolCodeCanceled,
				ToolReasonExecutionCanceled,
				false,
				fmt.Sprintf("HTTP tool execution canceled for tool=%s", tool),
				err,
				map[string]string{"tool": tool},
			)
		}
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("HTTP tool request failed for tool=%s: %s", tool, RedactSensitive(err.Error())),
			err,
			map[string]string{"tool": tool},
		)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("tool=%s failed to read response body", tool),
			err,
			map[string]string{"tool": tool},
		)
	}

	if resp.StatusCode >= 400 {
		return "", mapHTTPStatusToToolError(tool, resp.StatusCode, string(body))
	}

	var contractResp ToolExecutionResponse
	if json.Unmarshal(body, &contractResp) == nil && isOrlojToolStatus(contractResp.Status) {
		if toErr := contractResp.ToError(); toErr != nil {
			return "", toErr
		}
		return strings.TrimSpace(contractResp.Output.Result), nil
	}

	return strings.TrimSpace(string(body)), nil
}

func (r *HTTPToolClient) resolveSpec(tool string) (resources.ToolSpec, bool) {
	if r.registry == nil {
		return resources.ToolSpec{}, false
	}
	return r.registry.Resolve(tool)
}

// isOrlojToolStatus reports whether s is one of the Orloj tool contract status
// values ("ok", "error", "denied"). This prevents accidentally treating third-party
// API responses that happen to have a "status" field (e.g. Vapi's "queued") as
// Orloj contract envelopes.
func isOrlojToolStatus(s string) bool {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case ToolExecutionStatusOK, ToolExecutionStatusError, ToolExecutionStatusDenied:
		return true
	}
	return false
}

func mapHTTPStatusToToolError(tool string, statusCode int, body string) error {
	retryable := statusCode == 429 || statusCode >= 500
	code := ToolCodeExecutionFailed
	reason := ToolReasonBackendFailure

	switch statusCode {
	case 401:
		code = ToolCodeAuthInvalid
		reason = ToolReasonAuthInvalid
		retryable = false
	case 403:
		code = ToolCodeAuthForbidden
		reason = ToolReasonAuthForbidden
		retryable = false
	}

	return NewToolError(
		ToolStatusError,
		code,
		reason,
		retryable,
		fmt.Sprintf("tool=%s HTTP %d: %s", tool, statusCode, RedactSensitive(compactBody(body))),
		nil,
		map[string]string{
			"tool":        tool,
			"http_status": fmt.Sprintf("%d", statusCode),
		},
	)
}

func compactBody(body string) string {
	value := strings.TrimSpace(body)
	if len(value) <= 400 {
		return value
	}
	return value[:400]
}
