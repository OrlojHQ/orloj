# Orloj Resource Schema Reference

This is the canonical reference for all Orloj resource types and their YAML schemas. When generating manifests, always consult this file for correct field names, types, and defaults.

All resources share: `apiVersion: orloj.dev/v1`, a `kind` field, and `metadata` with at least `name`.

## Table of Contents

1. [Agent](#agent)
2. [AgentSystem](#agentsystem)
3. [Task](#task)
4. [ModelEndpoint](#modelendpoint)
5. [Secret](#secret)
6. [Tool](#tool)
7. [McpServer](#mcpserver)
8. [Memory](#memory)
9. [AgentPolicy](#agentpolicy)
10. [AgentRole](#agentrole)
11. [ToolPermission](#toolpermission)
12. [TaskSchedule](#taskschedule)
13. [TaskWebhook](#taskwebhook)
14. [Worker](#worker)

---

## Common Metadata

```yaml
metadata:
  name: resource-name          # required, unique per namespace
  namespace: default           # optional, defaults to "default"
  labels:                      # optional
    orloj.dev/pattern: pipeline
    orloj.dev/use-case: my-use-case
  annotations: {}              # optional
```

---

## Agent

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: my-agent
  labels:
    orloj.dev/use-case: my-system
spec:
  model_ref: openai-default           # required - references a ModelEndpoint
  prompt: |                            # required - system prompt
    You are a research specialist.
  tools:                               # optional - tool names the agent can invoke
    - web-search
  allowed_tools:                       # optional - whitelist subset of tools
    - web-search
  roles:                               # optional - AgentRole references
    - researcher-role
  memory:                              # optional
    ref: my-memory                     # Memory resource name
    type: vector
    provider: in-memory
    allow: [read, write]               # allowed ops: read, write, delete, list, reset
  execution:                           # optional - runtime behavior
    profile: dynamic                   # "dynamic" | "contract"
    tool_sequence: []                  # ordered tool calls (contract mode)
    required_output_markers: []        # strings that must appear in output
    duplicate_tool_call_policy: short_circuit  # "short_circuit" | "deny"
    on_contract_violation: non_retryable_error # "observe" | "non_retryable_error"
    tool_use_behavior: run_llm_again   # "run_llm_again" | "stop_on_first_tool"
    output_schema: {}                  # JSON Schema for structured output
  limits:                              # optional - safety bounds
    max_steps: 10                      # default: 10
    timeout: 30s                       # Go duration syntax
```

---

## AgentSystem

Defines the multi-agent graph topology.

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: my-system
  labels:
    orloj.dev/pattern: pipeline        # pipeline | hierarchical | swarm-loop
spec:
  agents:                              # list of Agent resource names
    - planner-agent
    - researcher-agent
    - writer-agent
  graph:
    planner-agent:
      edges:
        - to: researcher-agent
          labels: {}                   # optional routing labels
          policy: {}                   # optional per-edge policy
          condition:                   # optional firing predicate
            output_contains: "..."
            output_not_contains: "..."
            output_matches: "regex"
            output_json_path: "$.route"
            equals: "value"
            not_equals: "value"
            contains: "value"
            greater_than: "5"
            less_than: "10"
            default: true              # fallback when no condition matches
      join:                            # convergence semantics (on incoming edges)
        mode: wait_for_all             # "wait_for_all" | "quorum"
        quorum_count: 2                # absolute min upstream branches
        quorum_percent: 50             # percentage-based min (0-100)
        on_failure: deadletter         # "deadletter" | "skip" | "continue_partial"
      delegates: []                    # sub-agents dispatched after execution
      delegate_join: {}                # join semantics for delegate returns
```

### Topology Patterns

**Pipeline** (A -> B -> C):
```yaml
graph:
  agent-a:
    edges:
      - to: agent-b
  agent-b:
    edges:
      - to: agent-c
```

**Hierarchical** (manager fans out to leads, leads to workers, workers merge):
```yaml
graph:
  manager:
    edges:
      - to: lead-a
      - to: lead-b
  lead-a:
    edges:
      - to: worker-a
  lead-b:
    edges:
      - to: worker-b
  worker-a:
    edges:
      - to: editor
  worker-b:
    edges:
      - to: editor
  editor:
    join:
      mode: wait_for_all
```

**Swarm-Loop** (coordinator fans out to scouts, scouts loop back, synthesizer at end):
```yaml
graph:
  coordinator:
    edges:
      - to: scout-alpha
      - to: scout-beta
      - to: scout-gamma
      - to: synthesizer
  scout-alpha:
    edges:
      - to: coordinator
  scout-beta:
    edges:
      - to: coordinator
  scout-gamma:
    edges:
      - to: coordinator
```

---

## Task

```yaml
apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: my-task
spec:
  system: my-system                    # required - AgentSystem name
  mode: run                            # "run" (default) | "template"
  input:                               # key-value input data
    topic: "some topic"
  priority: high                       # "normal" | "high" | "low"
  max_turns: 10                        # max agent turns (important for swarm-loop)
  retry:
    max_attempts: 2
    backoff: 2s
  message_retry:
    max_attempts: 2
    backoff: 250ms
    max_backoff: 2s
    jitter: full                       # "none" | "full" | "equal"
    non_retryable: []                  # error codes to skip
  requirements:
    region: us-west-2
    gpu: false
    model: claude-sonnet-4-20250514
```

---

## ModelEndpoint

```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: openai-default
spec:
  provider: openai                     # openai | anthropic | ollama | azure_openai
  base_url: https://api.openai.com/v1  # auto-set by provider if omitted
  default_model: gpt-4o-mini
  options: {}                          # provider-specific options
  auth:
    secretRef: openai-api-key          # Secret resource name
  allowPrivate: false                  # true for ollama by default
```

### Provider Examples

```yaml
# Anthropic
spec:
  provider: anthropic
  base_url: https://api.anthropic.com/v1
  default_model: claude-sonnet-4-20250514
  auth:
    secretRef: anthropic-api-key

# Ollama (local)
spec:
  provider: ollama
  base_url: http://localhost:11434
  default_model: llama3
  allowPrivate: true

# Azure OpenAI
spec:
  provider: azure_openai
  base_url: https://my-resource.openai.azure.com
  default_model: gpt-4o
  options:
    api_version: "2024-02-01"
  auth:
    secretRef: azure-openai-key
```

---

## Secret

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: my-secret
spec:
  stringData:                          # plaintext (auto-converted to base64)
    value: replace-with-your-key
  # OR
  data:                                # base64-encoded
    value: c2stMTIzNDU2Nzg5MA==
```

---

## Tool

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: web-search
spec:
  type: http                           # http | external | grpc | webhook-callback | mcp | wasm | cli
  endpoint: https://api.example.com/search
  description: Search the web
  input_schema:
    type: object
    properties:
      query: { type: string }
  capabilities: [web-search]           # capability tags
  operation_classes: [read]            # read | write | delete | admin
  risk_level: low                      # low | medium | high | critical
  runtime:
    timeout: 30s
    isolation_mode: none               # none | sandboxed | container | wasm | kubernetes
    retry:
      max_attempts: 1
      backoff: 0s
      max_backoff: 30s
      jitter: none
  auth:
    profile: bearer                    # bearer | api_key_header | basic | oauth2_client_credentials
    secretRef: api-key-secret
    headerName: X-API-Key              # for api_key_header
```

### CLI Tool

```yaml
spec:
  type: cli
  cli:
    command: bash
    args: ["-c", "{{ .input }}"]
    image: ubuntu:latest
    output: stdout                     # stdout | stderr | both
    working_dir: /app
    env:
      MY_VAR: value
    env_from:
      - name: API_KEY
        secretRef: my-secret
        key: value
```

### MCP Tool

```yaml
spec:
  type: mcp
  mcp_server_ref: my-mcp-server
  mcp_tool_name: get_weather
```

---

## McpServer

```yaml
apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: my-mcp-server
spec:
  transport: stdio                     # stdio | http
  command: npx                         # required for stdio
  args: ["-y", "@modelcontextprotocol/server-everything"]
  env:
    - name: API_KEY
      secretRef: my-secret
  image: node:20                       # optional container image for stdio
  endpoint: https://...                # required for http transport
  idle_timeout: "0"                    # eviction timeout, "0" = never
  tool_filter:
    include: [echo, get_weather]       # whitelist of tool names
  reconnect:
    max_attempts: 3
    backoff: 2s
  auth: {}                             # same structure as Tool auth (for http)
```

---

## Memory

```yaml
apiVersion: orloj.dev/v1
kind: Memory
metadata:
  name: my-memory
spec:
  type: vector                         # vector | kv | custom (informational)
  provider: in-memory                  # in-memory | pgvector | http
  embedding_model: text-embedding-3-small  # ModelEndpoint ref (required for pgvector)
  endpoint: https://...                # provider endpoint (mutually exclusive with endpoint_secret_ref)
  endpoint_secret_ref: pg-dsn-secret   # Secret ref for sensitive endpoints (mutually exclusive with endpoint)
  auth:
    secretRef: pinecone-key
```

---

## AgentPolicy

```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: safe-policy
spec:
  max_tokens_per_run: 10000
  allowed_models: [gpt-4o-mini, claude-sonnet-4-20250514]
  blocked_tools: [dangerous-tool]
  apply_mode: scoped                   # "scoped" | "global"
  target_systems: [my-system]          # for scoped mode
  target_tasks: [my-task]
  target_agents: [agent-a, agent-b]    # optional - per-agent targeting within matched systems/tasks
  max_child_depth: 5
  max_child_tasks: 20
```

---

## AgentRole

```yaml
apiVersion: orloj.dev/v1
kind: AgentRole
metadata:
  name: researcher-role
spec:
  description: Permissions for research agents
  permissions:
    - tool:web-search:invoke
    - tool:summarize:invoke
    - capability:read
```

---

## ToolPermission

```yaml
apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: web-search-permission
spec:
  tool_ref: web-search
  action: invoke
  required_permissions: [tool:web-search:invoke]
  match_mode: all                      # "all" | "any"
  apply_mode: global                   # "global" | "scoped"
  target_agents: [research-agent]      # required for scoped
  operation_rules:
    - operation_class: read            # read | write | delete | admin | *
      verdict: allow                   # allow | deny | approval_required
    - operation_class: write
      verdict: deny
```

---

## TaskSchedule

```yaml
apiVersion: orloj.dev/v1
kind: TaskSchedule
metadata:
  name: daily-digest
spec:
  task_ref: digest-template-task       # must reference a mode:template Task
  schedule: "0 9 * * *"               # cron expression
  time_zone: America/New_York          # IANA timezone
  suspend: false
  starting_deadline_seconds: 300
  concurrency_policy: forbid
  successful_history_limit: 10
  failed_history_limit: 3
```

---

## TaskWebhook

```yaml
apiVersion: orloj.dev/v1
kind: TaskWebhook
metadata:
  name: github-webhook
spec:
  task_ref: webhook-template-task      # must reference a mode:template Task
  suspend: false
  auth:
    profile: github                    # generic | github | hmac | shared_token
    secret_ref: github-webhook-secret
    signature_header: X-Hub-Signature-256  # auto-set for github profile
  idempotency:
    event_id_header: X-GitHub-Delivery
    dedupe_window_seconds: 259200
  payload:
    mode: raw
    input_key: webhook_payload
```

---

## Worker

```yaml
apiVersion: orloj.dev/v1
kind: Worker
metadata:
  name: gpu-worker
spec:
  region: us-west-2
  capabilities:
    gpu: true
    supported_models: [claude-sonnet-4-20250514]
  max_concurrent_tasks: 4
```

---

## Naming Conventions

Orloj examples follow consistent naming patterns:

- **Blueprints**: `bp-{pattern}-{role}-agent` (e.g., `bp-pipeline-planner-agent`)
- **Use cases**: `uc-{slug}-{role}-agent` (e.g., `uc-weekly-brief-planner-agent`)
- **Systems**: `{prefix}-{slug}-system` (e.g., `uc-weekly-brief-system`)
- **Tasks**: `{prefix}-{slug}-task` (e.g., `uc-weekly-brief-task`)

When generating for users, use a short slug derived from their project name or description.
