package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// StreamableHTTPMcpTransport communicates with an MCP server over HTTP
// using the Streamable HTTP transport (JSON-RPC 2.0 over POST).
type StreamableHTTPMcpTransport struct {
	endpoint     string
	headers      map[string]string
	client       HTTPDoer
	sessionID    string
	allowPrivate bool
}

// StreamableHTTPMcpTransportConfig configures the HTTP transport.
type StreamableHTTPMcpTransportConfig struct {
	Endpoint     string
	Headers      map[string]string
	Client       HTTPDoer
	AllowPrivate bool // permit connections to private/internal IPs
}

func NewStreamableHTTPMcpTransport(cfg StreamableHTTPMcpTransportConfig) *StreamableHTTPMcpTransport {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &StreamableHTTPMcpTransport{
		endpoint:     strings.TrimRight(cfg.Endpoint, "/"),
		headers:      cfg.Headers,
		client:       client,
		allowPrivate: cfg.AllowPrivate,
	}
}

func (t *StreamableHTTPMcpTransport) Initialize(ctx context.Context) (*McpInitResult, error) {
	if err := ValidateEndpointURL(t.endpoint, t.allowPrivate); err != nil {
		return nil, fmt.Errorf("mcp http: endpoint blocked: %w", err)
	}
	resp, err := t.postRequest(ctx, "initialize", mcpInitializeParams{
		ProtocolVersion: "2025-03-26",
		ClientInfo:      McpServerInfo{Name: "orloj", Version: "1.0.0"},
		Capabilities:    map[string]any{},
	})
	if err != nil {
		return nil, fmt.Errorf("mcp http: initialize failed: %w", err)
	}

	var result McpInitResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp http: failed to decode initialize result: %w", err)
	}

	if _, err := t.postNotification(ctx, "notifications/initialized", nil); err != nil {
		return nil, fmt.Errorf("mcp http: initialized notification failed: %w", err)
	}

	return &result, nil
}

func (t *StreamableHTTPMcpTransport) ListTools(ctx context.Context) ([]McpToolDefinition, error) {
	var all []McpToolDefinition
	var cursor string
	for page := 0; page < maxToolsListPages; page++ {
		var params any
		if cursor != "" {
			params = mcpToolsListParams{Cursor: cursor}
		}
		resp, err := t.postRequest(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("mcp http: tools/list failed: %w", err)
		}
		var result mcpToolsListResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, fmt.Errorf("mcp http: failed to decode tools/list result: %w", err)
		}
		all = append(all, result.Tools...)
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return all, nil
}

func (t *StreamableHTTPMcpTransport) CallTool(ctx context.Context, name string, arguments map[string]any) (*McpToolResult, error) {
	resp, err := t.postRequest(ctx, "tools/call", mcpToolsCallParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp http: tools/call %s failed: %w", name, err)
	}
	var result McpToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp http: failed to decode tools/call result for %s: %w", name, err)
	}
	return &result, nil
}

func (t *StreamableHTTPMcpTransport) Close() error {
	return nil
}

func (t *StreamableHTTPMcpTransport) postRequest(ctx context.Context, method string, params any) (jsonrpcResponse, error) {
	id := nextMcpRequestID()
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return jsonrpcResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return jsonrpcResponse{}, fmt.Errorf("build HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	if t.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", t.sessionID)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return jsonrpcResponse{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if sid := httpResp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.sessionID = sid
	}

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 10*1024*1024))
	if err != nil {
		return jsonrpcResponse{}, fmt.Errorf("read response body: %w", err)
	}

	if httpResp.StatusCode >= 400 {
		return jsonrpcResponse{}, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return jsonrpcResponse{}, fmt.Errorf("decode JSON-RPC response: %w", err)
	}
	if resp.Error != nil {
		return resp, resp.Error
	}
	return resp, nil
}

func (t *StreamableHTTPMcpTransport) postNotification(ctx context.Context, method string, params any) (*http.Response, error) {
	notif := jsonrpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(notif)
	if err != nil {
		return nil, fmt.Errorf("marshal notification: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	if t.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", t.sessionID)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	_, _ = io.ReadAll(httpResp.Body)
	return httpResp, nil
}
