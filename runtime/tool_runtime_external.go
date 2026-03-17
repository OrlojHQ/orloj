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
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// ExternalToolRuntime delegates tool execution to an external HTTP service.
// Tools with spec.type=external have their ToolExecutionRequest forwarded
// to spec.endpoint and the ToolExecutionResponse parsed from the reply.
type ExternalToolRuntime struct {
	registry     ToolCapabilityRegistry
	secrets      SecretResolver
	authInjector *AuthInjector
	client       HTTPDoer
	namespace    string
}

func NewExternalToolRuntime(registry ToolCapabilityRegistry, secrets SecretResolver, client HTTPDoer) *ExternalToolRuntime {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &ExternalToolRuntime{
		registry:     registry,
		secrets:      secrets,
		authInjector: NewAuthInjector(secrets, nil),
		client:       client,
	}
}

func (r *ExternalToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewExternalToolRuntime(registry, nil, nil)
	}
	return &ExternalToolRuntime{
		registry:     registry,
		secrets:      r.secrets,
		authInjector: r.authInjector,
		client:       r.client,
		namespace:    r.namespace,
	}
}

func (r *ExternalToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewExternalToolRuntime(nil, nil, nil)
	}
	copy := *r
	copy.namespace = resources.NormalizeNamespace(strings.TrimSpace(namespace))
	if aware, ok := copy.secrets.(namespaceAwareSecretResolver); ok {
		copy.secrets = aware.WithNamespace(copy.namespace)
	}
	copy.authInjector = NewAuthInjector(copy.secrets, nil)
	if r.authInjector != nil {
		copy.authInjector.tokenCache = r.authInjector.tokenCache
	}
	return &copy
}

func (r *ExternalToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
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
	if r.registry == nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			"missing tool registry for external runtime",
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}
	spec, ok := r.registry.Resolve(tool)
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
			fmt.Sprintf("tool=%s missing endpoint for external delegation", tool),
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}

	execReq := ToolExecutionRequest{
		ToolContractVersion: ToolContractVersionV1,
		RequestID:           fmt.Sprintf("ext-%s-%d", tool, time.Now().UnixNano()),
		Namespace:           r.namespace,
		Tool: ToolExecutionRequestTool{
			Name:         tool,
			Operation:    ToolOperationInvoke,
			Capabilities: spec.Capabilities,
			RiskLevel:    spec.RiskLevel,
		},
		InputRaw: input,
		Runtime: ToolExecutionRuntime{
			Mode: "external",
		},
		Attempt: 1,
	}

	payload, err := json.Marshal(execReq)
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			false,
			fmt.Sprintf("tool=%s failed to marshal execution request", tool),
			err,
			map[string]string{"tool": tool},
		)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
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
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Tool-Contract-Version", ToolContractVersionV1)

	if r.authInjector != nil {
		authResult, authErr := r.authInjector.Resolve(ctx, tool, spec.Auth)
		if authErr != nil {
			return "", authErr
		}
		for k, v := range authResult.Headers {
			httpReq.Header.Set(k, v)
		}
	}

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return "", mapExternalHTTPError(tool, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("tool=%s failed to read external response body", tool),
			err,
			map[string]string{"tool": tool},
		)
	}

	if resp.StatusCode >= 400 {
		return "", mapHTTPStatusToToolError(tool, resp.StatusCode, string(body))
	}

	var contractResp ToolExecutionResponse
	if err := json.Unmarshal(body, &contractResp); err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("tool=%s external service returned invalid contract response", tool),
			err,
			map[string]string{"tool": tool},
		)
	}

	if toErr := contractResp.ToError(); toErr != nil {
		return "", toErr
	}
	return strings.TrimSpace(contractResp.Output.Result), nil
}

func mapExternalHTTPError(tool string, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return NewToolError(
			ToolStatusError,
			ToolCodeTimeout,
			ToolReasonExecutionTimeout,
			true,
			fmt.Sprintf("external tool execution timed out for tool=%s", tool),
			err,
			map[string]string{"tool": tool, "isolation_mode": "external"},
		)
	case errors.Is(err, context.Canceled):
		return NewToolError(
			ToolStatusError,
			ToolCodeCanceled,
			ToolReasonExecutionCanceled,
			false,
			fmt.Sprintf("external tool execution canceled for tool=%s", tool),
			err,
			map[string]string{"tool": tool, "isolation_mode": "external"},
		)
	default:
		return NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("external tool request failed for tool=%s: %s", tool, RedactSensitive(err.Error())),
			err,
			map[string]string{"tool": tool, "isolation_mode": "external"},
		)
	}
}
