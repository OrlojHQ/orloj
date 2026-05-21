# McpServer

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

Represents a connection to an external MCP (Model Context Protocol) server. The McpServer controller discovers tools via `tools/list` and auto-generates `Tool` resources (type=mcp) for each.

## spec

- `transport` (string): **required**. `stdio` or `http`.
- `command` (string): stdio transport: command to spawn the MCP server process. Required unless `image` is set.
- `args` ([]string): stdio transport: command arguments.
- `env` ([]object): stdio transport: environment variables for the child process. Each entry has:
  - `name` (string): environment variable name.
  - `value` (string): literal value.
  - `secretRef` (string): resolve value from a Secret resource. Mutually exclusive with `value`.
  - `mountPath` (string): absolute path inside the container where the resolved value is written as a file. Only valid when `image` is set. The env var is set to the mount path so the MCP server can locate the file.
- `image` (string): stdio transport: container image. When set, the MCP server runs inside a Docker container (`docker run --rm -i`) with sandboxing.
- `image_pull_secret` (string): name of a Secret containing registry credentials for pulling `spec.image`. The Secret must contain either a `.dockerconfigjson` key with a complete Docker config JSON, or `registry`, `username`, and `password` keys. Requires `image` to be set.
- `idle_timeout` (duration string): duration after which an idle session is shut down (e.g. `5m`). Default `0` means never evict.
- `endpoint` (string): http transport: the MCP server URL.
- `auth` (object): http transport: authentication configuration.
  - `secretRef` (string): secret reference for auth.
  - `profile` (string): `bearer` or `api_key_header`. Defaults to `bearer`.
- `tool_filter` (object): optional tool import filtering.
  - `include` ([]string): allowlist of MCP tool names. When set, only listed tools are generated. When empty, all discovered tools are generated.
- `reconnect` (object): reconnection policy.
  - `max_attempts` (int): max reconnection attempts. Defaults to 3.
  - `backoff` (duration string): backoff between attempts. Defaults to `2s`.
- `default_tool_runtime` (object): default runtime policy inherited by all generated Tool resources. When set, each tool synced from this server receives this policy as its `spec.runtime`.
  - `timeout` (duration string): max tool call execution time (e.g. `30s`, `2m`).
  - `isolation_mode` (string): isolation mode for tool execution.
  - `retry` (object): retry policy (see [Tool resource](./tool.md)).
- `allowPrivate` (boolean): http transport only. When `true`, permits this MCP server's HTTP transport to connect to RFC 1918 / ULA / CGNAT addresses (e.g. in-cluster Services like `http://mcp.internal.svc.cluster.local:8000`). Loopback, link-local, cloud metadata, and unspecified addresses remain blocked regardless. Defaults to `false`; set `true` only for trusted internal MCP servers. Has no effect on `stdio` transport.

## Defaults and Validation

- `transport` is required. Must be `stdio` or `http`.
- `command` or `image` is required when `transport=stdio`.
- `endpoint` is required when `transport=http`.
- `image` is only valid with `transport=stdio`.
- `image_pull_secret` requires `image` to be set.
- `env[].secretRef` and `env[].value` are mutually exclusive.
- `env[].mountPath` requires `image` to be set and must be an absolute path.
- `idle_timeout` defaults to `0` (never evict).
- `reconnect.max_attempts` defaults to `3`.
- `reconnect.backoff` defaults to `2s`.

## status

- `phase`: `Pending`, `Connecting`, `Ready`, `Error`.
- `discoveredTools` ([]string): all tool names from the MCP server's `tools/list` response.
- `generatedTools` ([]string): names of the `Tool` resources actually created.
- `lastSyncedAt` (timestamp): last successful tool sync.
- `lastError` (string): last error message.

Guide: [Connect an MCP Server](../../guides/connect-mcp-server.md)

Examples: [`examples/resources/mcp-servers/`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/mcp-servers)

See also: [MCP server concepts](../../concepts/tools/mcp-server.md).
