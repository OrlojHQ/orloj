# Agent

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `model_ref` (string): reference to a `ModelEndpoint` (`name` or `namespace/name`).
- `prompt` (string): agent instruction prompt.
- `tools` ([]string): tool names available to the agent.
- `allowed_tools` ([]string): tools pre-authorized without RBAC. Bypasses AgentRole/ToolPermission checks for listed tools.
- `roles` ([]string): bound `AgentRole` names.
- `memory` (object):
  - `ref` (string): reference to a `Memory` resource. This attaches the memory backend to the agent. See [Memory](../../concepts/memory/index.md).
  - `allow` ([]string): explicit built-in memory operations allowed for the agent: `read`, `write`, `search`, `list`, `ingest`.
  - `type` (string)
  - `provider` (string)
- `limits` (object):
  - `max_steps` (int)
  - `timeout` (string duration)
- `execution` (object): optional per-agent execution contract.
  - `profile` (string): `dynamic` (default) or `contract`.
  - `tool_sequence` ([]string): required tool names when `profile=contract`. Tracked as a set (order-independent).
  - `required_output_markers` ([]string): strings that should appear in final model output when `profile=contract`. Treated as best-effort: missing markers at `max_steps` produce a warning, not a hard failure, when all tools completed.
  - `duplicate_tool_call_policy` (string): `short_circuit` (default) or `deny`. In `short_circuit` mode, duplicate tool calls reuse cached results and inject a completion hint. This applies to **all profiles**, not just `contract`.
  - `on_contract_violation` (string): `observe` or `non_retryable_error` (default). In `observe` mode, violations are logged as telemetry events but do not stop execution or deadletter the task.
  - `tool_use_behavior` (string): Controls what happens after a tool call succeeds. See [Tool Use Behavior](#tool-use-behavior) below.

### Tool Use Behavior

The `tool_use_behavior` field controls whether the model gets another turn after a successful tool call. This is the primary lever for optimizing token usage in tool-calling agents.

| Value | Model calls | When to use |
|-------|------------|---------------|
| `run_llm_again` (default) | Tool call + follow-up model call to process the result | The agent needs to **interpret, format, or synthesize** the tool output before handing off. Most agents need this. |
| `stop_on_first_tool` | Tool call only -- tool output becomes the agent's final output directly | The agent is a **relay** that calls a tool and passes raw data to the next agent in the pipeline. No interpretation needed. |

**Example: `run_llm_again` (default)**

An analyst agent calls an API tool, then needs to produce labeled output from the raw response:

```yaml
kind: Agent
metadata:
  name: analyst-agent
spec:
  prompt: "Call the API, then return SUMMARY: and EVIDENCE: labels."
  tools:
    - external-api-tool
  # tool_use_behavior defaults to run_llm_again -- agent gets a
  # second model call to read the tool result and produce labels.
```

Step 1: model calls `external-api-tool` → Step 2: model reads tool result, produces labeled output → done (2 model calls).

**Example: `stop_on_first_tool`**

A fetcher agent's only job is to call a tool and pass the raw result downstream:

```yaml
kind: Agent
metadata:
  name: fetcher-agent
spec:
  prompt: "Fetch the latest data from the API."
  tools:
    - external-api-tool
  execution:
    tool_use_behavior: stop_on_first_tool
  # Agent exits immediately after the tool returns.
  # Raw tool output becomes the agent's output -- no extra model call.
```

Step 1: model calls `external-api-tool` → done (1 model call). The next agent in the pipeline receives the raw tool response as context.

**When NOT to use `stop_on_first_tool`:**
- The agent needs to produce structured/labeled output from the tool result.
- The agent has multiple tools and may need to call more than one.
- The agent needs to reason about the tool result before responding.

## Defaults and Validation

- `model_ref` is required.
- `roles` are trimmed and deduplicated (case-insensitive).
- `memory.allow` is trimmed, normalized, and deduplicated. It requires `memory.ref`.
- `limits.max_steps` defaults to `10` when `<= 0`.
- `execution.profile` defaults to `dynamic`.
- `execution.duplicate_tool_call_policy` defaults to `short_circuit`. Applies to all profiles.
- `execution.on_contract_violation` defaults to `non_retryable_error`. Set to `observe` for safe production rollout.
- `execution.tool_use_behavior` defaults to `run_llm_again`.
- `execution.tool_sequence` and `execution.required_output_markers` are trimmed and deduplicated.
- When `execution.profile=contract`, `execution.tool_sequence` is required.
- Tool sequence is tracked as a set: tools may be called in any order.
- When all tools in `tool_sequence` complete but `required_output_markers` are not satisfied at `max_steps`, the task completes with a `contract_warning` event instead of deadlettering.

**Structured tool protocol:** Tool results are sent to the model using the provider's native structured tool calling protocol (OpenAI `role: "tool"` with `tool_call_id`, Anthropic `tool_result` content blocks). This gives the model structured evidence that a tool was already called, preventing unnecessary repeat calls.

**Scaling ladder for cost control:**

1. `profile: dynamic` (default): structured tool protocol prevents repeat calls. Succeeded tools are filtered from the available tools list. No YAML changes needed.
2. `tool_use_behavior: stop_on_first_tool`: for pipeline stages that pass raw data, eliminates all extra model calls (1 model call + 1 tool call total).
3. `profile: contract` + `on_contract_violation: observe`: adds guaranteed early completion when all tools succeed plus telemetry for contract deviations.
4. `profile: contract` + `on_contract_violation: non_retryable_error`: hard enforcement for critical pipeline stages. Violations deadletter the task.

## status

- `phase`, `lastError`, `observedGeneration`

Example: [`examples/resources/agents/`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/agents)

See also: [Agent concept](../../concepts/agents/agent.md)
