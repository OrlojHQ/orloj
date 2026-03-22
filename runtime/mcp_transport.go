package agentruntime

import "context"

// maxToolsListPages caps the number of tools/list pagination rounds to
// prevent a malicious or misconfigured MCP server from causing infinite loops.
const maxToolsListPages = 100

// McpTransport abstracts the MCP JSON-RPC 2.0 communication layer.
// Implementations handle stdio (child process) and Streamable HTTP transports.
type McpTransport interface {
	Initialize(ctx context.Context) (*McpInitResult, error)
	ListTools(ctx context.Context) ([]McpToolDefinition, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (*McpToolResult, error)
	Close() error
}

// McpInitResult captures the server's response to the initialize handshake.
type McpInitResult struct {
	ProtocolVersion string            `json:"protocolVersion"`
	ServerInfo      McpServerInfo     `json:"serverInfo"`
	Capabilities    McpCapabilities   `json:"capabilities"`
}

type McpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type McpCapabilities struct {
	Tools *McpToolCapability `json:"tools,omitempty"`
}

type McpToolCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// McpToolDefinition describes one tool exposed by an MCP server via tools/list.
type McpToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// McpToolResult captures the response from a tools/call invocation.
type McpToolResult struct {
	Content []McpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// McpContent is one content block in a tool result.
type McpContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// McpTextResult returns the concatenated text content of a tool result.
func (r *McpToolResult) McpTextResult() string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	if len(r.Content) == 1 {
		return r.Content[0].Text
	}
	var out string
	for _, c := range r.Content {
		if c.Text != "" {
			if out != "" {
				out += "\n"
			}
			out += c.Text
		}
	}
	return out
}
