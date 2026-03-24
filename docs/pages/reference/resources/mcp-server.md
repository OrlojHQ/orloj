# McpServer

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

Represents a connection to an external MCP (Model Context Protocol) server. The McpServer controller discovers tools via `tools/list` and auto-generates `Tool` resources (type=mcp) for each.

## spec

- `transport` (string): **required**. `stdio` or `http`.
- `command` (string): stdio transport: command to spawn the MCP server process.
- `args` ([]string): stdio transport: command arguments.
- `env` ([]object): stdio transport: environment variables for the child process. Each entry has:
  - `name` (string): environment variable name.
  - `value` (string): literal value.
  - `secretRef` (string): resolve value from a Secret resource. Mutually exclusive with `value`.
- `endpoint` (string): http transport: the MCP server URL.
- `auth` (object): http transport: authentication configuration.
  - `secretRef` (string): secret reference for auth.
  - `profile` (string): `bearer` or `api_key_header`. Defaults to `bearer`.
- `tool_filter` (object): optional tool import filtering.
  - `include` ([]string): allowlist of MCP tool names. When set, only listed tools are generated. When empty, all discovered tools are generated.
- `reconnect` (object): reconnection policy.
  - `max_attempts` (int): max reconnection attempts. Defaults to 3.
  - `backoff` (duration string): backoff between attempts. Defaults to `2s`.

## Defaults and Validation

- `transport` is required. Must be `stdio` or `http`.
- `command` is required when `transport=stdio`.
- `endpoint` is required when `transport=http`.
- `env[].secretRef` and `env[].value` are mutually exclusive.
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
