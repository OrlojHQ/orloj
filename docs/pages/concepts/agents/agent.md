# Agent

An **Agent** is a declarative unit of work backed by a language model. It defines what the agent does (its prompt), what model powers it, what tools it can call, and what constraints bound its execution.

## Defining an Agent

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a research assistant.
    Produce concise evidence-backed answers.
  tools:
    - web_search
    - vector_db
  memory:
    ref: research-memory
  roles:
    - analyst-role
  limits:
    max_steps: 6
    timeout: 30s
```

### Key Fields

| Field | Description |
|---|---|
| `model_ref` | Required reference to a [ModelEndpoint](../tools/model-endpoint.md) resource for provider-aware routing. |
| `prompt` | The system instruction that defines the agent's behavior. |
| `tools` | List of [Tool](../tools/tool.md) names this agent may call. Tool calls are subject to governance checks. |
| `roles` | Bound [AgentRole](../governance/agent-role.md) names. Roles carry permissions that authorize tool usage. |
| `memory.ref` | Reference to a [Memory](../memory/) resource. This attaches the memory backend to the agent. |
| `memory.allow` | Explicit list of built-in memory operations the agent may use: `read`, `write`, `search`, `list`, `ingest`. |
| `limits.max_steps` | Maximum execution steps per task turn. Defaults to `10`. |
| `limits.timeout` | Maximum wall-clock time per task turn. |

## How an Agent Executes

When the runtime activates an agent during a task, it:

1. Initializes the agent's conversation history with the system prompt and current task context.
2. If `memory.ref` is set, wires the backing memory store into the runtime. If `memory.allow` is also set, the runtime exposes only those built-in memory operations as available tools.
3. Routes the request to the configured model via the model gateway, sending the full conversation history.
4. If the model selects tool calls, the runtime checks governance (AgentPolicy, AgentRole, ToolPermission) and executes authorized tools. Memory tool calls are handled internally without network calls. Tool results are sent back using the provider's native structured tool protocol (`role: "tool"` with `tool_call_id` for OpenAI, `tool_result` content blocks for Anthropic).
5. Results are appended to the conversation history and sent back to the model for the next step. The agent completes when the model produces text output without requesting further tools, or when `max_steps` / `timeout` is reached. Already-called tools are removed from the available list to prevent duplicate calls.

Conversation history is maintained for the full duration of the agent's activation, giving the model continuity across reasoning and tool-use steps. See [Memory](../memory/) for details on memory layers and built-in tools.

## Related

- [AgentSystem](./agent-system.md) -- compose agents into directed graphs
- [Resource Reference: Agent](../../reference/resources/agent.md)
- [Memory](../memory/)
- [Guide: Deploy Your First Pipeline](../../guides/deploy-pipeline.md)
