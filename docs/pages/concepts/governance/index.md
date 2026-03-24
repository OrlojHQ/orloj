# Governance and Policies

Orloj provides a built-in governance layer that controls what agents can do at runtime. Three resource types work together to enforce authorization: **[AgentPolicy](./agent-policy.md)** constrains execution parameters, **[AgentRole](./agent-role.md)** grants named permissions to agents, and **[ToolPermission](./tool-permission.md)** defines what permissions are required to invoke a tool.

Governance is fail-closed: if an agent uses roles and lacks the required permissions for a tool call, the call is denied with a `tool_permission_denied` error.

## Simple Path: `allowed_tools`

For most agents, you can skip roles and ToolPermission entirely by listing tools in the agent's `spec.allowed_tools` field. Tools in this list are pre-authorized and bypass RBAC checks:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  tools:
    - web_search
    - vector_db
  allowed_tools:
    - web_search
    - vector_db
  prompt: |
    You are a research assistant.
```

This agent can invoke both `web_search` and `vector_db` without any AgentRole or ToolPermission resources. `spec.tools` declares which tools the agent can select during execution; `spec.allowed_tools` declares which of those tools are pre-authorized.

AgentPolicy constraints (like `blocked_tools` and `max_tokens_per_run`) still apply. `allowed_tools` only bypasses the role-based permission check.

## Advanced Path: Roles and ToolPermission

For fine-grained access control, use [AgentRole](./agent-role.md) and [ToolPermission](./tool-permission.md) resources. This is recommended when you need per-tool permission auditing, scoped tool access across teams, or separation of duties between agent authors and platform operators.

## How Authorization Works

When an agent selects a tool call during execution, the runtime evaluates authorization in this order:

1. **AgentPolicy check** -- Is the tool in the policy's `blocked_tools` list? If yes, deny.
2. **ToolPermission lookup** -- Find the ToolPermission for this tool and action.
3. **Permission matching** -- Collect the agent's permissions from all bound AgentRoles. Check them against `required_permissions` using the configured `match_mode`.
4. **Decision** -- If all checks pass, the tool is invoked. If any check fails, the call returns `tool_permission_denied`.

```
Agent selects tool call
        │
        ▼
  AgentPolicy check
  (blocked_tools?)
        │
    ┌───┴───┐
    │blocked │──► Denied
    └───┬───┘
        │ allowed
        ▼
  ToolPermission lookup
        │
        ▼
  Permission matching
  (agent roles vs required)
        │
    ┌───┴───┐
    │ fail  │──► Denied (tool_permission_denied)
    └───┬───┘
        │ pass
        ▼
   Tool invoked
```

## Approval Workflow

When a tool call is flagged as `approval_required` by a [ToolPermission](./tool-permission.md) operation rule, the task pauses and a [ToolApproval](./tool-approval.md) resource is created. An external actor must approve or deny the request before the task can continue. See [ToolApproval](./tool-approval.md) for details.

## End-to-End Example

To set up a governed agent that can search the web but not access the filesystem:

**1. Define the role:**
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

**2. Define the tool permission:**
```yaml
apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: web-search-invoke
spec:
  tool_ref: web_search
  action: invoke
  match_mode: all
  required_permissions:
    - tool:web_search:invoke
    - capability:web.read
```

**3. Define the policy:**
```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: cost-policy
spec:
  apply_mode: scoped
  target_systems:
    - report-system-governed
  allowed_models:
    - gpt-4o
  blocked_tools:
    - filesystem_delete
```

**4. Bind the role to the agent:**
```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent-governed
spec:
  model_ref: openai-default
  roles:
    - analyst-role
  tools:
    - web_search
    - vector_db
```

In this configuration, `research-agent-governed` can invoke `web_search` (it holds the required permissions) but cannot invoke `vector_db` (it lacks `tool:vector_db:invoke`). Any attempt to call `filesystem_delete` is blocked by the policy regardless of permissions.

## Related

- [AgentPolicy](./agent-policy.md)
- [AgentRole](./agent-role.md)
- [ToolPermission](./tool-permission.md)
- [ToolApproval](./tool-approval.md)
- [Security and Isolation](../../operations/security.md)
- [Guide: Set Up Multi-Agent Governance](../../guides/setup-governance.md)
