# Set Up Multi-Agent Governance

This guide is for platform engineers who need to enforce tool authorization and model constraints on their agent systems. You will create policies, roles, and tool permissions, deploy a governed agent system, and verify that unauthorized tool calls are denied.

## Prerequisites

- Orloj server (`orlojd`) and at least one worker running
- `orlojctl` available
- Familiarity with the [Governance and Policies](../concepts/governance.md) concepts

## What You Will Build

A governed version of the report pipeline where:
- An AgentPolicy restricts model usage and blocks dangerous tools
- AgentRoles grant specific tool permissions to agents
- ToolPermissions define authorization requirements for each tool
- An agent that attempts an unauthorized tool call is denied

## Step 1: Define the Tools

Start by creating the tools your agents will reference:

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: web_search
spec:
  type: http
  endpoint: https://api.search.com
  auth:
    secretRef: search-api-key
```

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: vector_db
spec:
  type: http
  endpoint: https://api.vector-db.local/query
```

Apply both:
```bash
orlojctl apply -f web_search_tool.yaml
orlojctl apply -f vector_db_tool.yaml
```

## Step 2: Create Tool Permissions

Define what permissions are required to invoke each tool:

```yaml
apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: web-search-invoke
spec:
  tool_ref: web_search
  action: invoke
  match_mode: all
  apply_mode: global
  required_permissions:
    - tool:web_search:invoke
    - capability:web.read
```

```yaml
apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: vector-db-invoke
spec:
  tool_ref: vector_db
  action: invoke
  match_mode: all
  apply_mode: global
  required_permissions:
    - tool:vector_db:invoke
```

Apply:
```bash
orlojctl apply -f web_search_invoke_permission.yaml
orlojctl apply -f vector_db_invoke_permission.yaml
```

## Step 3: Create Agent Roles

Roles bundle permissions that can be bound to agents:

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

```yaml
apiVersion: orloj.dev/v1
kind: AgentRole
metadata:
  name: vector-reader-role
spec:
  description: Can query vector knowledge tools.
  permissions:
    - tool:vector_db:invoke
```

Apply:
```bash
orlojctl apply -f analyst_role.yaml
orlojctl apply -f vector_reader_role.yaml
```

## Step 4: Create an Agent Policy

The policy scopes model and tool constraints to a specific agent system:

```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: cost-policy
spec:
  apply_mode: scoped
  target_systems:
    - report-system-governed
  max_tokens_per_run: 50000
  allowed_models:
    - gpt-4o
  blocked_tools:
    - filesystem_delete
```

Apply:
```bash
orlojctl apply -f cost_policy.yaml
```

## Step 5: Deploy the Governed Agent

Create an agent that binds the `analyst-role` but **not** `vector-reader-role`. This means it will be authorized for `web_search` but denied for `vector_db`:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent-governed
spec:
  model_ref: openai-default
  prompt: |
    You are a research assistant.
    Produce concise evidence-backed answers.
  roles:
    - analyst-role
  tools:
    - web_search
    - vector_db
  memory:
    ref: research-memory
  limits:
    max_steps: 6
    timeout: 30s
```

Even though `vector_db` is listed in `tools`, the agent lacks the `tool:vector_db:invoke` permission, so any attempt to call it will be denied.

## Step 6: Wire the System and Submit a Task

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: report-system-governed
spec:
  agents:
    - planner-agent
    - research-agent-governed
    - writer-agent
  graph:
    planner-agent:
      next: research-agent-governed
    research-agent-governed:
      next: writer-agent
```

```bash
orlojctl apply -f report_system_governed.yaml
orlojctl apply -f weekly_report_governed_task.yaml
```

## Step 7: Verify Governance Enforcement

Check the task trace for authorization events:
```bash
orlojctl trace task weekly-report-governed
```

In the trace output, look for:
- Successful `web_search` invocations (agent holds required permissions)
- `tool_permission_denied` errors for any `vector_db` attempts (agent lacks `tool:vector_db:invoke`)

## Allowing Both Tools

To grant the agent access to both tools, add `vector-reader-role` to its roles:

```yaml
spec:
  roles:
    - analyst-role
    - vector-reader-role
```

The pre-built example for this is `examples/agents/research_agent_governed_allow.yaml` with `examples/agent-systems/report_system_governed_allow.yaml`.

## Next Steps

- [Governance and Policies](../concepts/governance.md) -- deeper dive into the authorization model
- [Security and Isolation](../operations/security.md) -- operational security controls
- [Configure Model Routing](./configure-model-routing.md) -- set up provider-specific model endpoints
