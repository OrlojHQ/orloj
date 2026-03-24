# McpServer

An **McpServer** represents a connection to an external MCP (Model Context Protocol) server. The McpServer controller discovers tools via `tools/list` and auto-generates [Tool](./tool.md) resources (type=mcp) for each discovered tool.

## Defining an McpServer

**stdio transport** (spawns a child process):

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: everything-server
spec:
  transport: stdio
  command: npx
  args:
    - -y
    - "@modelcontextprotocol/server-everything"
  env:
    - name: API_KEY
      secretRef: mcp-api-key
  tool_filter:
    include:
      - echo
      - add
  reconnect:
    max_attempts: 3
    backoff: 2s
```

**HTTP transport** (connects to a running server):

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: remote-server
spec:
  transport: http
  endpoint: https://mcp.example.com
  auth:
    secretRef: mcp-auth-token
    profile: bearer
```

### Key Fields

| Field | Description |
|---|---|
| `transport` | Required. `stdio` or `http`. |
| `command` | stdio: command to spawn the MCP server process. |
| `args` | stdio: command arguments. |
| `env` | stdio: environment variables. Each entry supports `value` (literal) or `secretRef` (resolve from Secret). |
| `endpoint` | http: the MCP server URL. |
| `auth` | http: authentication configuration (`secretRef` + `profile`). |
| `tool_filter.include` | Optional allowlist of MCP tool names. When set, only listed tools are generated. |
| `reconnect` | Reconnection policy: `max_attempts` (default 3) and `backoff` (default 2s). |

## How It Works

When an McpServer resource is applied:

1. The McpServer controller establishes a connection using the configured transport.
2. It calls `tools/list` to discover available tools.
3. For each discovered tool (filtered by `tool_filter.include` if set), it creates a `Tool` resource with `type: mcp`, `mcp_server_ref`, and `mcp_tool_name`.
4. The `description` and `input_schema` from the MCP server are propagated to the generated Tool, giving the LLM rich tool definitions.

At invocation time, the `MCPToolRuntime` resolves the server reference, obtains a session from the `McpSessionManager`, and sends a `tools/call` JSON-RPC 2.0 request through the appropriate transport.

## Status

The McpServer status tracks connection and tool sync state:

| Field | Description |
|---|---|
| `phase` | `Pending`, `Connecting`, `Ready`, or `Error`. |
| `discoveredTools` | All tool names from the MCP server's `tools/list` response. |
| `generatedTools` | Names of the `Tool` resources actually created. |
| `lastSyncedAt` | Timestamp of last successful tool sync. |

## Related

- [Tool](./tool.md) -- the auto-generated tool resources
- [Secret](./secret.md) -- credentials for MCP server auth
- [Resource Reference: McpServer](../../reference/resources/mcp-server.md)
- [Guide: Connect an MCP Server](../../guides/connect-mcp-server.md)
