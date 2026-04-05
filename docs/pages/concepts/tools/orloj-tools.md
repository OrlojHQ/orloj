# Orloj Built-in Tools

Orloj provides built-in tools under the `orloj.*` namespace that let agents interact with the Orloj control plane. These tools follow the same governance model as any other tool -- ToolPermission, AgentPolicy `blocked_tools`, and ToolApproval all apply.

## Available Tools

### `orloj.task.create`

Creates a new task from a template. The task runs independently (fire-and-forget) and the tool returns immediately with the created task's name and initial phase.

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `template` | string | Yes | Name of an existing task with `mode: template` |
| `input` | object | No | Key-value input overrides merged with template defaults |
| `labels` | object | No | Additional labels attached to the new task |

**Response:**

```json
{
  "status": "created",
  "name": "write-article-parent-task-a1b2c3d4",
  "phase": "Pending",
  "template": "write-article",
  "system": "writing-dept"
}
```

### `orloj.task.list`

Lists tasks in the current namespace, optionally filtered by labels.

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `labels` | object | No | Label key-value pairs to filter tasks |
| `limit` | integer | No | Maximum results to return (default 20) |

## Enabling Orloj Tools

Add the desired tool names to the agent's `spec.allowed_tools` list:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: researcher
spec:
  model_ref: gpt-4o
  prompt: "You are a research agent..."
  allowed_tools:
    - web-search
    - orloj.task.create
    - orloj.task.list
```

## Task Templates

`orloj.task.create` references tasks that have `mode: template`. Define a template task that serves as a blueprint:

```yaml
apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: write-article
spec:
  mode: template
  system: writing-dept
  input:
    topic: ""
    research_findings: ""
```

When an agent creates a task from this template, the child task gets `mode: run` and enters the normal execution pipeline.

## Lineage Tracking

Child tasks are automatically labeled for traceability:

- `orloj.dev/parent-task` -- name of the parent task
- `orloj.dev/depth` -- nesting depth (0 for root tasks, incremented for each level)
- `orloj.dev/created-by` -- always `orloj.task.create`

Use `orloj.task.list` with a label filter to find tasks spawned by a specific parent:

```json
{"labels": {"orloj.dev/parent-task": "my-research-task"}}
```

## Safety Limits

AgentPolicy supports two optional fields to prevent runaway task creation:

| Field | Default | Description |
|-------|---------|-------------|
| `max_child_depth` | 5 | Maximum nesting depth for chained task creation |
| `max_child_tasks` | 20 | Maximum child tasks a single execution can create |

```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: limit-task-creation
spec:
  apply_mode: global
  max_child_depth: 3
  max_child_tasks: 10
```

## Governance

Orloj tools are governed like any other tool:

- **Block with AgentPolicy:** `blocked_tools: [orloj.task.create]`
- **Require approval with ToolPermission:**

```yaml
apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: gate-task-creation
spec:
  tool_ref: orloj.task.create
  operation_rules:
    - operation_class: write
      verdict: approval_required
```

If no governance resources are configured, agents with orloj tools in their `allowed_tools` can use them freely.
