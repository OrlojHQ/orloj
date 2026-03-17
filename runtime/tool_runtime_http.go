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

	"github.com/OrlojHQ/orloj/crds"
)

// HTTPToolClient executes tools via HTTP POST against Tool.spec.endpoint.
// It replaces MockToolClient as the base runtime for isolation_mode=none.
type HTTPToolClient struct {
	registry     ToolCapabilityRegistry
	secrets      SecretResolver
	authInjector *AuthInjector
	client       HTTPDoer
}

// HTTPDoer abstracts HTTP request execution for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewHTTPToolClient(registry ToolCapabilityRegistry, secrets SecretResolver, client HTTPDoer) *HTTPToolClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPToolClient{
		registry:     registry,
		secrets:      secrets,
		authInjector: NewAuthInjector(secrets, nil),
		client:       client,
	}
}

func NewHTTPToolClientWithAuth(registry ToolCapabilityRegistry, injector *AuthInjector, client HTTPDoer) *HTTPToolClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPToolClient{
		registry:     registry,
		authInjector: injector,
		client:       client,
	}
}

func (r *HTTPToolClient) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewHTTPToolClient(registry, nil, nil)
	}
	return &HTTPToolClient{
		registry:     registry,
		secrets:      r.secrets,
		authInjector: r.authInjector,
		client:       r.client,
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
	if json.Unmarshal(body, &contractResp) == nil && strings.TrimSpace(contractResp.Status) != "" {
		if toErr := contractResp.ToError(); toErr != nil {
			return "", toErr
		}
		return strings.TrimSpace(contractResp.Output.Result), nil
	}

	return strings.TrimSpace(string(body)), nil
}

func (r *HTTPToolClient) resolveSpec(tool string) (crds.ToolSpec, bool) {
	if r.registry == nil {
		return crds.ToolSpec{}, false
	}
	return r.registry.Resolve(tool)
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
