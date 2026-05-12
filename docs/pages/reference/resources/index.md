# Resources

This section documents selected resource kinds in `orloj.dev/v1`, based on the runtime types and normalization logic in:

- `resources/agent.go`
- `resources/model_endpoint.go`
- `resources/resource_types.go`
- `resources/graph.go`

Each kind has a dedicated page; see [Resource reference pages](#resource-reference-pages) below.

## Common Conventions

- Every resource uses standard top-level fields: `apiVersion`, `kind`, `metadata`, `spec`, `status`.
- `metadata.name` is required for all resources.
- `metadata.namespace` defaults to `default` when omitted.
- Most resources default `status.phase` to `Pending` during normalization.

## Resource reference pages

- [Agent](./agent.md)
- [AgentSystem](./agent-system.md)
- [Task](./task.md)
- [TaskSchedule](./task-schedule.md)
- [TaskWebhook](./task-webhook.md)
- [Tool](./tool.md)
- [ModelEndpoint](./model-endpoint.md)
- [McpServer](./mcp-server.md)
- [Memory](./memory.md)
- [ContextAdapter](./context-adapter.md)
- [Secret](./secret.md)
- [SealedSecret](./sealed-secret.md)
- [AgentPolicy](./agent-policy.md)
- [AgentRole](./agent-role.md)
- [ToolPermission](./tool-permission.md)
- [ToolApproval](./tool-approval.md)
- [TaskApproval](./task-approval.md)
- [Worker](./worker.md)
- [EvalDataset](./eval-dataset.md)
- [EvalRun](./eval-run.md)
