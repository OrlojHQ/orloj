# AgentRole

An **AgentRole** is a named set of permission strings. Agents bind to roles through their `spec.roles` field, which grants them the associated permissions.

## Defining an AgentRole

```yaml
apiVersion: orloj.dev/v1
kind: AgentRole
metadata:
  name: analyst-role
spec:
  description: Can call web search style tools.
  permissions:
    - tool:web_search:invoke
    - capability:web.read
```

### Permission String Conventions

| Pattern | Meaning |
|---|---|
| `tool:<tool_name>:invoke` | Permission to invoke a specific tool. |
| `capability:<capability_name>` | Permission to use a declared capability. |

## How It Works

An agent that binds multiple roles accumulates the union of all granted permissions:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent-governed
spec:
  model_ref: openai-default
  roles:
    - analyst-role
    - vector-reader-role
  tools:
    - web_search
    - vector_db
```

During the [authorization flow](./), the runtime collects permissions from all bound roles and checks them against the [ToolPermission](./tool-permission.md) requirements for the requested tool.

Permissions are trimmed and deduplicated (case-insensitive) during normalization.

## Related

- [Governance Overview](./) -- how the three governance resources work together
- [AgentPolicy](./agent-policy.md)
- [ToolPermission](./tool-permission.md)
- [Resource Reference: AgentRole](../../reference/resources/agent-role.md)
