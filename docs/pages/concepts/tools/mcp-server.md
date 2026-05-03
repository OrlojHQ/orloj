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

**stdio transport with container image** (runs inside Docker):

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: gmail
spec:
  transport: stdio
  image: mcp/gmail
  idle_timeout: "5m"
  env:
    - name: GMAIL_OAUTH_PATH
      secretRef: gmail-creds/oauth_keys
      mountPath: /secrets/gcp-oauth.keys.json
    - name: GMAIL_CREDENTIALS_PATH
      secretRef: gmail-creds/credentials
      mountPath: /secrets/credentials.json
```

When `image` is set, the MCP server runs inside a container (`docker run --rm -i`) with sandboxing (read-only filesystem, no capabilities, no privilege escalation). If `command` is also set, it overrides the image's entrypoint. If only `image` is set, the image's built-in entrypoint is used.

When `mountPath` is set on an env entry, the resolved value is written to an ephemeral host file and bind-mounted read-only into the container at that path. The env var is set to the mount path so the MCP server can locate the file. This enables MCP servers that require file-based credentials (OAuth JSON keys, service account files, TLS certificates).

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

| Field                 | Description                                                                                               |
| --------------------- | --------------------------------------------------------------------------------------------------------- |
| `transport`           | Required. `stdio` or `http`.                                                                              |
| `command`             | stdio: command to spawn the MCP server process. Required unless `image` is set.                           |
| `args`                | stdio: command arguments.                                                                                 |
| `env`                 | stdio: environment variables. Each entry supports `value` (literal) or `secretRef` (resolve from Secret). |
| `env[].mountPath`     | Absolute path inside the container where the resolved value is written as a file. Only valid with `image`. |
| `image`               | stdio: container image. When set, the MCP server runs inside a Docker container.                          |
| `idle_timeout`        | Duration after which an idle session is shut down (e.g. `5m`). Default `0` means never evict.             |
| `endpoint`            | http: the MCP server URL.                                                                                 |
| `auth`                | http: authentication configuration (`secretRef` + `profile`).                                             |
| `tool_filter.include` | Optional allowlist of MCP tool names. When set, only listed tools are generated.                          |
| `reconnect`           | Reconnection policy: `max_attempts` (default 3) and `backoff` (default 2s).                               |
| `resources`           | Container resource overrides: `memory`, `cpus`, `pids_limit`. Overrides global `--tool-container-*` flags. Only applies when `image` is set. |

## How It Works

When an McpServer resource is applied:

1. The McpServer controller establishes a connection using the configured transport.
2. It calls `tools/list` to discover available tools.
3. For each discovered tool (filtered by `tool_filter.include` if set), it creates a `Tool` resource with `type: mcp`, `mcp_server_ref`, and `mcp_tool_name`.
4. The `description` and `input_schema` from the MCP server are propagated to the generated Tool, giving the LLM rich tool definitions.

At invocation time, the `MCPToolRuntime` resolves the server reference, obtains a session from the `McpSessionManager`, and sends a `tools/call` JSON-RPC 2.0 request through the appropriate transport.

## Container Resources

Container-backed MCP servers inherit the global `--tool-container-memory`, `--tool-container-cpus`, and `--tool-container-pids-limit` defaults. For servers that need more resources (e.g. Chromium-based tools like Playwright), override per-server:

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: playwright-mcp
spec:
  transport: stdio
  image: playwright-mcp:latest
  resources:
    memory: 1g
    cpus: "1.0"
    pids_limit: 512
```

Operators can enforce an upper bound with `--tool-container-max-memory`, `--tool-container-max-cpus`, and `--tool-container-max-pids-limit` on `orlojd`. Manifests exceeding the ceiling are rejected at apply time.

## Idle Timeout and Session Lifecycle

When `idle_timeout` is set, sessions are automatically shut down after the specified duration of inactivity:

1. **Apply** -- session spins up, `tools/list` discovers tools, Tool resources are written to the store.
2. **Idle period** -- after `idle_timeout` with no `tools/call`, the reaper closes the session (kills the process or container).
3. **Tool resources persist** -- agents can still see the discovered tools even while the session is down.
4. **Next task** -- when an agent calls a tool, `GetOrCreate` transparently recreates the session.
5. **Warm reuse** -- if multiple tool calls arrive within the timeout window, they reuse the same session.

This is especially useful with container-backed MCP servers: the container is only running while tools are actively being called, then automatically shuts down between tasks.

### Image Choice and Cold Start

When a session is recreated after being reaped, the cold-start cost depends on the image strategy:

- **Pre-built image** (MCP server baked in): 1-3 seconds. Recommended for production.
- **Generic base image + `npx -y`**: re-downloads the package on every cold start because `--rm` wipes the container. Not recommended with `idle_timeout`.
- **Bare host process** (no image): npm cache persists between restarts on the host.

## Status

The McpServer status tracks connection and tool sync state:

| Field             | Description                                                 |
| ----------------- | ----------------------------------------------------------- |
| `phase`           | `Pending`, `Connecting`, `Ready`, or `Error`.               |
| `discoveredTools` | All tool names from the MCP server's `tools/list` response. |
| `generatedTools`  | Names of the `Tool` resources actually created.             |
| `lastSyncedAt`    | Timestamp of last successful tool sync.                     |

## Related

- [Tool](./tool.md) -- the auto-generated tool resources
- [Secret](./secret.md) -- credentials for MCP server auth
- [Resource Reference: McpServer](../../reference/resources/mcp-server.md)
- [Guide: Connect an MCP Server](../../guides/connect-mcp-server.md)
