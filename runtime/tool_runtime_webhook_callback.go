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
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// WebhookCallbackToolRuntime implements an async tool pattern: fire a
// ToolExecutionRequest to the tool endpoint, then poll a callback URL
// (or the same endpoint with the request ID) until a ToolExecutionResponse
// arrives or the context times out.
//
// Flow:
//  1. POST ToolExecutionRequest to Tool.spec.endpoint
//  2. Receive 202 Accepted (or 200 with immediate result)
//  3. If 202: poll GET {endpoint}/{request_id} until status != "pending"
type WebhookCallbackToolRuntime struct {
	registry     ToolCapabilityRegistry
	secrets      SecretResolver
	authInjector *AuthInjector
	client       HTTPDoer
	namespace    string
	pollInterval time.Duration
	callbacks    *callbackRegistry
}

// callbackRegistry stores async responses delivered via push callback.
type callbackRegistry struct {
	mu       sync.Mutex
	pending  map[string]chan ToolExecutionResponse
}

func newCallbackRegistry() *callbackRegistry {
	return &callbackRegistry{
		pending: make(map[string]chan ToolExecutionResponse),
	}
}

func (r *callbackRegistry) Register(requestID string) <-chan ToolExecutionResponse {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan ToolExecutionResponse, 1)
	r.pending[requestID] = ch
	return ch
}

func (r *callbackRegistry) Deliver(requestID string, resp ToolExecutionResponse) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.pending[requestID]
	if !ok {
		return false
	}
	select {
	case ch <- resp:
	default:
	}
	delete(r.pending, requestID)
	return true
}

func (r *callbackRegistry) Remove(requestID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, requestID)
}

func NewWebhookCallbackToolRuntime(registry ToolCapabilityRegistry, secrets SecretResolver, client HTTPDoer, pollInterval time.Duration) *WebhookCallbackToolRuntime {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	return &WebhookCallbackToolRuntime{
		registry:     registry,
		secrets:      secrets,
		authInjector: NewAuthInjector(secrets, nil),
		client:       client,
		pollInterval: pollInterval,
		callbacks:    newCallbackRegistry(),
	}
}

func (r *WebhookCallbackToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewWebhookCallbackToolRuntime(registry, nil, nil, 0)
	}
	return &WebhookCallbackToolRuntime{
		registry:     registry,
		secrets:      r.secrets,
		authInjector: r.authInjector,
		client:       r.client,
		namespace:    r.namespace,
		pollInterval: r.pollInterval,
		callbacks:    r.callbacks,
	}
}

func (r *WebhookCallbackToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewWebhookCallbackToolRuntime(nil, nil, nil, 0)
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

// DeliverCallback allows external code to push an async response for a pending request.
func (r *WebhookCallbackToolRuntime) DeliverCallback(requestID string, resp ToolExecutionResponse) bool {
	if r == nil || r.callbacks == nil {
		return false
	}
	return r.callbacks.Deliver(requestID, resp)
}

func (r *WebhookCallbackToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
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
			"missing tool registry for webhook-callback runtime",
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
			fmt.Sprintf("tool=%s missing endpoint for webhook-callback", tool),
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}

	requestID := fmt.Sprintf("wh-%s-%d", tool, time.Now().UnixNano())
	execReq := ToolExecutionRequest{
		ToolContractVersion: ToolContractVersionV1,
		RequestID:           requestID,
		Namespace:           r.namespace,
		Tool: ToolExecutionRequestTool{
			Name:         tool,
			Operation:    ToolOperationInvoke,
			Capabilities: spec.Capabilities,
			RiskLevel:    spec.RiskLevel,
		},
		InputRaw: input,
		Runtime: ToolExecutionRuntime{
			Mode: "webhook-callback",
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

	var authHeaders map[string]string
	if r.authInjector != nil {
		authResult, authErr := r.authInjector.Resolve(ctx, tool, spec.Auth)
		if authErr != nil {
			return "", authErr
		}
		authHeaders = authResult.Headers
	}

	callbackCh := r.callbacks.Register(requestID)
	defer r.callbacks.Remove(requestID)

	submitResp, submitBody, err := r.submitRequest(ctx, endpoint, payload, authHeaders)
	if err != nil {
		return "", mapWebhookHTTPError(tool, err)
	}
	if submitResp.StatusCode >= 400 {
		return "", mapHTTPStatusToToolError(tool, submitResp.StatusCode, submitBody)
	}

	if submitResp.StatusCode == http.StatusOK {
		return r.parseContractResponse(tool, submitBody)
	}

	return r.awaitResult(ctx, tool, requestID, endpoint, authHeaders, callbackCh)
}

func (r *WebhookCallbackToolRuntime) submitRequest(ctx context.Context, endpoint string, payload []byte, authHeaders map[string]string) (*http.Response, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tool-Contract-Version", ToolContractVersionV1)
	for k, v := range authHeaders {
		req.Header.Set(k, v)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return &http.Response{StatusCode: resp.StatusCode, Header: resp.Header}, string(body), nil
}

func (r *WebhookCallbackToolRuntime) awaitResult(
	ctx context.Context,
	tool string,
	requestID string,
	endpoint string,
	authHeaders map[string]string,
	callbackCh <-chan ToolExecutionResponse,
) (string, error) {
	pollURL := strings.TrimRight(endpoint, "/") + "/" + requestID
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", NewToolError(
					ToolStatusError,
					ToolCodeTimeout,
					ToolReasonExecutionTimeout,
					true,
					fmt.Sprintf("webhook-callback tool execution timed out for tool=%s request_id=%s", tool, requestID),
					ctx.Err(),
					map[string]string{"tool": tool, "request_id": requestID},
				)
			}
			return "", NewToolError(
				ToolStatusError,
				ToolCodeCanceled,
				ToolReasonExecutionCanceled,
				false,
				fmt.Sprintf("webhook-callback tool execution canceled for tool=%s", tool),
				ctx.Err(),
				map[string]string{"tool": tool, "request_id": requestID},
			)

		case resp := <-callbackCh:
			if toErr := resp.ToError(); toErr != nil {
				return "", toErr
			}
			return strings.TrimSpace(resp.Output.Result), nil

		case <-ticker.C:
			result, done, err := r.pollOnce(ctx, tool, pollURL, authHeaders)
			if err != nil {
				return "", err
			}
			if done {
				return result, nil
			}
		}
	}
}

func (r *WebhookCallbackToolRuntime) pollOnce(ctx context.Context, tool, pollURL string, authHeaders map[string]string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return "", false, NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("tool=%s failed to build poll request", tool),
			err,
			map[string]string{"tool": tool},
		)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range authHeaders {
		req.Header.Set(k, v)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", false, nil
		}
		return "", false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		return "", false, nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode >= 400 {
		return "", true, mapHTTPStatusToToolError(tool, resp.StatusCode, string(body))
	}

	result, err := r.parseContractResponse(tool, string(body))
	if err != nil {
		return "", true, err
	}
	return result, true, nil
}

func (r *WebhookCallbackToolRuntime) parseContractResponse(tool string, body string) (string, error) {
	var contractResp ToolExecutionResponse
	if err := json.Unmarshal([]byte(body), &contractResp); err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("tool=%s returned invalid contract response", tool),
			err,
			map[string]string{"tool": tool},
		)
	}
	if toErr := contractResp.ToError(); toErr != nil {
		return "", toErr
	}
	return strings.TrimSpace(contractResp.Output.Result), nil
}

func mapWebhookHTTPError(tool string, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return NewToolError(
			ToolStatusError,
			ToolCodeTimeout,
			ToolReasonExecutionTimeout,
			true,
			fmt.Sprintf("webhook-callback tool submit timed out for tool=%s", tool),
			err,
			map[string]string{"tool": tool, "isolation_mode": "webhook-callback"},
		)
	case errors.Is(err, context.Canceled):
		return NewToolError(
			ToolStatusError,
			ToolCodeCanceled,
			ToolReasonExecutionCanceled,
			false,
			fmt.Sprintf("webhook-callback tool submit canceled for tool=%s", tool),
			err,
			map[string]string{"tool": tool, "isolation_mode": "webhook-callback"},
		)
	default:
		return NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("webhook-callback tool submit failed for tool=%s: %s", tool, RedactSensitive(err.Error())),
			err,
			map[string]string{"tool": tool, "isolation_mode": "webhook-callback"},
		)
	}
}
