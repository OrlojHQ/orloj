# MCP server examples

`McpServer` registers a Model Context Protocol server. The controller discovers tools via `tools/list` and materializes matching `Tool` resources (`spec.type: mcp`) for agents to bind.

## Prerequisites

- **Node.js** and **npx** on the machine running `orlojd` (stdio transport spawns the child process).
- Orloj server reconciling `McpServer` resources (see [Connect an MCP server](../../../docs/pages/guides/connect-mcp-server.md)).

## Sample: MCP “everything” server (stdio)

[`mcp_server_everything_stdio.yaml`](./mcp_server_everything_stdio.yaml) runs the reference `@modelcontextprotocol/server-everything` package with a narrow `tool_filter` so only a couple of tools are imported.

Apply:

```bash
go run ./cmd/orlojctl apply -f examples/resources/mcp-servers/mcp_server_everything_stdio.yaml
```

Inspect generated tools and bind them to agents like any other tool name (see the guide).

## Secrets and authenticated MCP servers

Stdio and HTTP transports can pull credentials from `Secret` resources via `spec.env[].secretRef` or `spec.auth` on the `McpServer`. Copy the patterns in [Connect an MCP server](../../../docs/pages/guides/connect-mcp-server.md); do not commit real tokens.
