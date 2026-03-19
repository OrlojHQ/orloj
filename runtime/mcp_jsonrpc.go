package agentruntime

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// JSON-RPC 2.0 wire types for MCP communication.

type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *jsonrpcError    `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *jsonrpcError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

type jsonrpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// MCP-specific request/response param types.

type mcpInitializeParams struct {
	ProtocolVersion string          `json:"protocolVersion"`
	ClientInfo      McpServerInfo   `json:"clientInfo"`
	Capabilities    map[string]any  `json:"capabilities"`
}

type mcpToolsListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

type mcpToolsListResult struct {
	Tools      []McpToolDefinition `json:"tools"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

type mcpToolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

var globalMcpRequestID atomic.Int64

func nextMcpRequestID() int64 {
	return globalMcpRequestID.Add(1)
}
