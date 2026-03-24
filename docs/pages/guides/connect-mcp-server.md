# Connect an MCP Server

This guide is for platform engineers who want to connect external MCP (Model Context Protocol) servers to Orloj. You will register an MCP server, verify tool discovery, selectively import tools, and assign them to agents.

## Prerequisites

- Orloj server (`orlojd`) running with `--embedded-worker`
- `orlojctl` available (or `go run ./cmd/orlojctl`)
- An MCP server to connect (stdio-based or remote HTTP)

If you have not set up Orloj yet, follow the [Install](../getting-started/install.md) and [Quickstart](../getting-started/quickstart.md) guides first.

## Background

MCP servers are external processes or services that expose tools via the [Model Context Protocol](https://modelcontextprotocol.io/). Unlike regular Orloj tools (which are 1:1 -- one resource, one capability), a single MCP server can provide many tools.

Orloj bridges this gap with the `McpServer` resource kind. When you register an MCP server, the controller:

1. Connects using the configured transport (stdio or Streamable HTTP)
2. Calls `tools/list` to discover available tools
3. Auto-generates a `Tool` resource (type=mcp) for each discovered tool
4. Keeps tools in sync on every reconcile cycle

Generated tools are first-class `Tool` resources. Agents reference them by name just like any other tool.

## Step 1: Register an MCP Server (stdio)

Stdio MCP servers run as child processes. Orloj spawns the process, communicates via stdin/stdout using JSON-RPC 2.0, and manages its lifecycle.

Create a manifest (`github-mcp.yaml`):

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: github-mcp
spec:
  transport: stdio
  command: npx @github/mcp-server
  args:
    - "--token-from-env"
  env:
    - name: GITHUB_TOKEN
      secretRef: github-token
```

Create the secret for the token:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: github-token
spec:
  stringData:
    value: ghp_your_github_token_here
```

Apply both:

```bash
orlojctl apply -f github-token-secret.yaml
orlojctl apply -f github-mcp.yaml
```

## Step 2: Register an MCP Server (HTTP)

Remote MCP servers communicate over HTTP using the Streamable HTTP transport. Use this for MCP servers running as hosted services.

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: remote-mcp
spec:
  transport: http
  endpoint: https://mcp.example.com/rpc
  auth:
    secretRef: mcp-api-key
    profile: bearer
```

Apply:

```bash
orlojctl apply -f remote-mcp.yaml
```

## Step 3: Verify Tool Discovery

After applying, check the McpServer status:

```bash
orlojctl get mcp-servers
```

```
NAME          TRANSPORT  STATUS  TOOLS  LAST_SYNCED
github-mcp    stdio      Ready   12     2025-03-18T14:30:00Z
remote-mcp    http       Ready   5      2025-03-18T14:30:05Z
```

List the auto-generated tools:

```bash
orlojctl get tools
```

Each generated tool follows the naming convention `{server}--{mcp-tool-name}`:

```
NAME                           TYPE   STATUS
github-mcp--create-issue       mcp    Ready
github-mcp--search-repos       mcp    Ready
github-mcp--list-prs           mcp    Ready
...
```

Inspect a specific generated tool to see its rich schema:

```bash
orlojctl get tools github-mcp--create-issue -o json
```

The tool's `spec.input_schema` is populated directly from the MCP server's `tools/list` response. The model gateway uses this schema when formatting tool definitions for the LLM, giving it structured parameter information instead of the generic `{input: string}` fallback.

## Step 4: Filter Tools (Optional)

By default, all tools discovered from an MCP server are imported. If a server exposes many tools and you only need a subset, use `spec.tool_filter.include`:

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: github-mcp
spec:
  transport: stdio
  command: npx @github/mcp-server
  args:
    - "--token-from-env"
  env:
    - name: GITHUB_TOKEN
      secretRef: github-token
  tool_filter:
    include:
      - create_issue
      - search_repos
```

Only the listed tools will be generated as `Tool` resources. Tools not in the allowlist are still discovered (visible in `status.discoveredTools`) but are not imported.

Re-apply the manifest to update:

```bash
orlojctl apply -f github-mcp.yaml
```

Tools that were previously generated but are no longer in the allowlist are automatically deleted on the next reconcile cycle.

## Step 5: Assign Tools to an Agent

Generated MCP tools are referenced by name in `agent.spec.tools`, exactly like any other tool:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: github-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a GitHub assistant. Use your tools to help
    the user manage issues and search repositories.
  tools:
    - github-mcp--create-issue
    - github-mcp--search-repos
  limits:
    max_steps: 8
    timeout: 60s
```

Apply and submit a task:

```bash
orlojctl apply -f github-agent.yaml
```

When the agent runs, the LLM sees the rich tool schemas from the MCP server and can call tools with structured arguments. The `GovernedToolRuntime` automatically routes `type=mcp` tools through the `MCPToolRuntime`, which sends `tools/call` to the MCP server via the session manager.

## Step 6: Configure Reconnection (Optional)

MCP server connections can drop. The `reconnect` policy controls how aggressively Orloj retries:

```yaml
spec:
  reconnect:
    max_attempts: 5
    backoff: 2s
```

Defaults: 3 attempts with 2s backoff. If all attempts fail, the McpServer enters the `Error` phase. The controller retries on the next reconcile cycle.

## How It Works

The data flow for an MCP tool call:

```
Agent step
  → GovernedToolRuntime (policy, timeout, retry)
    → MCPToolRuntime (resolves mcp_server_ref)
      → McpSessionManager (connection pool)
        → McpTransport (stdio or HTTP)
          → MCP Server (tools/call JSON-RPC 2.0)
```

Key implementation details:

- **Session pooling**: One session per McpServer. Sessions are reused across tool calls and reconcile cycles.
- **Schema propagation**: `spec.input_schema` and `spec.description` from tool discovery flow through to the model gateway, so the LLM gets rich parameter definitions.
- **Garbage collection**: Generated tools carry an `orloj.dev/mcp-server` label. When an MCP server is deleted, all its generated tools are cleaned up.
- **Governance**: MCP tools participate in the full governance pipeline. You can create `ToolPermission` and `AgentRole` resources for them, same as any other tool.

## McpServer Spec Reference

| Field | Description |
|---|---|
| `transport` | **Required**. `stdio` or `http`. |
| `command` | stdio: command to spawn the MCP server process. |
| `args` | stdio: command arguments. |
| `env` | stdio: environment variables. Each entry has `name`, `value` (literal), or `secretRef` (resolved from Secret resource). |
| `endpoint` | http: the MCP server URL. |
| `auth.secretRef` | http: secret for authentication. |
| `auth.profile` | http: auth profile (`bearer`, `api_key_header`). Defaults to `bearer`. |
| `tool_filter.include` | Optional allowlist of MCP tool names to import. When empty, all tools are imported. |
| `reconnect.max_attempts` | Max reconnection attempts. Defaults to 3. |
| `reconnect.backoff` | Backoff duration between attempts. Defaults to `2s`. |

### Status Fields

| Field | Description |
|---|---|
| `phase` | `Pending`, `Connecting`, `Ready`, or `Error`. |
| `discoveredTools` | All tool names from `tools/list`, regardless of filter. |
| `generatedTools` | Tool resource names actually created. |
| `lastSyncedAt` | Timestamp of last successful reconcile. |
| `lastError` | Last error message, if any. |

## Next Steps

- [Tools and Isolation](../concepts/tools/tool.md) -- concept deep-dive on tool types and isolation modes
- [Build a Custom Tool](./build-custom-tool.md) -- for non-MCP tools that need custom implementation
- [Set Up Multi-Agent Governance](./setup-governance.md) -- enforce authorization on MCP tools
- [Resource Reference](../reference/resources/) -- full spec for all resource kinds
