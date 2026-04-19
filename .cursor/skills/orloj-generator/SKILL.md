---
name: orloj-generator
description: >
  Interactive scaffold generator for Orloj multi-agent systems. Use this skill whenever someone wants to
  create, set up, scaffold, bootstrap, or generate an Orloj agent system, pipeline, swarm, or hierarchy.
  Also trigger when users mention "orlojctl init", ask how to get started with Orloj, want to build a
  multi-agent workflow, or describe a use case that maps to an Orloj topology (pipeline, hierarchical,
  swarm-loop). Even if they just say something like "I want agents that do X then Y then Z" — that's a
  pipeline, and this skill should activate. Covers generating all Orloj resource types: Agent, AgentSystem,
  Task, ModelEndpoint, Secret, Tool, McpServer, Memory, AgentPolicy, AgentRole, ToolPermission,
  TaskSchedule, and TaskWebhook manifests.
---

# Orloj Agent System Generator

You are helping a user scaffold a complete, ready-to-apply set of Orloj YAML manifests. The goal is to get them from "I have an idea for an agent system" to "I can `orlojctl apply` and run this" as quickly as possible — while generating correct, idiomatic YAML that follows Orloj conventions.

Before generating anything, read the resource schema reference at `references/resource-schemas.md` (relative to this skill's directory). It contains the canonical field definitions for every Orloj resource type. Consult it whenever you need to verify a field name, type, or default.

## How This Works

The generation flow has three phases: **understand**, **design**, and **generate**. Move through them conversationally — don't dump a wall of questions. Many users will give you enough in their first message to skip ahead.

### Phase 1: Understand the Use Case

Figure out what the user wants their agent system to do. You need to know:

1. **What's the goal?** What should the system produce or accomplish? (e.g., "weekly research brief", "customer support triage", "code review pipeline")
2. **What agents are involved?** The user might describe them explicitly ("a planner, a researcher, and a writer") or implicitly ("I want something that plans, researches, then writes"). Either works.
3. **How do they connect?** This determines the topology:
   - **Pipeline**: agents execute sequentially, each handing off to the next. Good for staged workflows (plan → research → write).
   - **Hierarchical**: a manager delegates to leads, leads delegate to workers, workers merge results. Good for cross-functional work with parallel branches.
   - **Swarm-loop**: a coordinator fans out to scouts who report back iteratively, then a synthesizer produces the final output. Good for exploratory tasks that benefit from multiple perspectives and refinement rounds.

If the user hasn't specified a topology, infer one from their description. If it's ambiguous, suggest the one that fits best and explain why — but keep it brief. Something like: "That sounds like a pipeline — each stage feeds the next. Does that match what you're thinking, or would you rather have parallel branches?"

4. **Which model provider?** Ask which LLM provider they want to use. Common options: OpenAI, Anthropic, Ollama (local), Azure OpenAI. Default to OpenAI with `gpt-4o-mini` if they don't have a preference.

5. **Optional extras** — only ask about these if relevant to their use case. Don't overwhelm new users with options they don't need yet:
   - **Tools**: Does any agent need to call external tools (HTTP APIs, CLI commands, MCP servers)?
   - **Memory**: Should any agent persist context across runs (vector store)?
   - **Governance**: Do they need policies, roles, or tool permissions?
   - **Scheduling**: Should this run on a cron schedule?
   - **Webhooks**: Should external events trigger runs?

### Phase 2: Design the System

Once you understand the use case, design the agent system before generating YAML. Present a brief summary:

- The topology you'll use and why
- Each agent's name, role, and rough prompt direction
- The graph structure (which agents connect to which)
- Any optional resources (tools, memory, schedule, etc.)

Use a quick topology sketch — something like:

```
planner → researcher → writer    (pipeline)
```

or for hierarchical:

```
manager → research-lead → research-worker ─┐
       └→ social-lead   → social-worker  ──┤→ editor (wait_for_all)
```

Get confirmation before generating. A quick "Does this look right?" is enough.

### Phase 3: Generate the Manifests

Generate a complete, ready-to-apply set of YAML files. Follow these rules:

#### Naming Conventions

Use a short slug derived from the user's project name or description. Apply it consistently:

- Agents: `{slug}-{role}-agent` (e.g., `support-triage-agent`)
- System: `{slug}-system` (e.g., `support-system`)
- Task: `{slug}-task`
- Other resources: `{slug}-{descriptor}` (e.g., `support-web-search-tool`)

#### File Organization

Generate files as a flat directory the user can apply with `orlojctl apply -f <dir>/ --run`. The standard set:

| File | Resource | When to include |
|------|----------|-----------------|
| `secret-{provider}.yaml` | Secret | Always |
| `model-endpoint.yaml` | ModelEndpoint | Always |
| `agents/{role}.yaml` | Agent (one per agent) | Always |
| `agent-system.yaml` | AgentSystem | Always |
| `task.yaml` | Task | Always |
| `tool-{name}.yaml` | Tool | When agents use tools |
| `mcp-server-{name}.yaml` | McpServer | When agents use MCP tools |
| `memory-{name}.yaml` | Memory | When agents need persistence |
| `agent-policy.yaml` | AgentPolicy | When governance is needed |
| `agent-role-{name}.yaml` | AgentRole | When RBAC is needed |
| `tool-permission-{name}.yaml` | ToolPermission | When tool access control is needed |
| `task-template.yaml` | Task (mode: template) | When scheduling or webhooks are used |
| `task-schedule.yaml` | TaskSchedule | When cron scheduling is needed |
| `task-webhook.yaml` | TaskWebhook | When webhook triggers are needed |
| `secret-webhook.yaml` | Secret | When webhook auth is needed |

#### YAML Quality

- Always include `apiVersion: orloj.dev/v1` and the correct `kind`
- Include `labels` with `orloj.dev/pattern` and a descriptive use-case label on all resources
- Write clear, specific agent prompts — not generic one-liners. The prompt should tell the agent its role, what it receives from upstream, and what it should produce for downstream. 3-5 lines is a good target.
- Use sensible defaults for limits: `max_steps: 4` for coordination agents, `max_steps: 6` for worker agents, `timeout: 20s` for light agents, `timeout: 30s` for agents doing heavier work
- Always include `retry` and `message_retry` on Tasks — these are essential for production reliability
- For swarm-loop topologies, always set `max_turns` on the Task to prevent infinite loops
- Mark secrets with placeholder values and a comment: `# replace-with-your-actual-key`

#### Agent Prompt Quality

This is where you add real value. Don't write lazy one-line prompts. Each agent prompt should:

- State the agent's role clearly
- Describe what input it receives (from the task input or from upstream agents)
- Explain what output it should produce and in what form
- Set boundaries on scope (what the agent should NOT do)
- Be specific to the user's domain, not generic boilerplate

For example, instead of:
```
You are a researcher. Do research.
```

Write:
```
You are the research analyst for customer support triage.
You receive a categorized support ticket from the triage agent upstream.
Your job is to:
1. Identify the product area and relevant documentation
2. Check for known issues matching the customer's symptoms
3. Summarize findings in a structured format: diagnosis, confidence level, and recommended resolution path
Do not attempt to draft customer-facing responses — that's the writer's job.
```

#### Output Format

Write each file individually using the Write tool, placing them in a directory structure the user can browse. After writing all files, provide:

1. A summary of what was generated
2. The apply command: `orlojctl apply -f <directory>/ --run`
3. A reminder to replace secret placeholder values
4. Optionally, a brief note on what they might want to customize (prompts, model, limits)

## Topology-Specific Guidance

### Pipeline

The simplest topology. Use when work flows in one direction through stages.

- Graph is a simple chain: A → B → C
- Each agent gets output from the previous agent as context
- No join semantics needed
- Good for: content generation, data processing, review chains

### Hierarchical

Use when work needs to fan out to parallel branches and merge.

- A manager/coordinator delegates to multiple leads
- Leads delegate to workers
- Workers converge on an editor/aggregator node with `join.mode: wait_for_all`
- Good for: cross-functional projects, parallel research, multi-department workflows

Key detail: the merging agent (editor) needs `join.mode: wait_for_all` on its graph entry so it waits for all upstream branches before executing.

### Swarm-Loop

Use when the problem benefits from iterative refinement with multiple perspectives.

- A coordinator fans out to multiple scouts
- Scouts report back to the coordinator (bidirectional edges)
- Coordinator can dispatch multiple rounds of questions
- A synthesizer agent produces the final output
- The coordinator also has an edge to the synthesizer for when it's ready to conclude
- **Always set `max_turns` on the Task** to prevent infinite looping

Key detail: the swarm-loop is the only topology with bidirectional edges (scouts → coordinator → scouts). The `max_turns` field on the Task is the circuit-breaker.

## Edge Cases

- **Single agent**: Totally valid. Generate an AgentSystem with one agent and no graph edges. The system runs the agent once.
- **Conditional routing**: If the user describes "if X then agent A, else agent B", use `condition` on edges with `output_contains`, `output_matches`, or `default: true` for the fallback.
- **Mixed topologies**: Sometimes a user wants a pipeline where one stage fans out. That's fine — it's a hybrid. Use pipeline naming but add fan-out edges and a join where branches merge.

## What NOT to Do

- Don't generate resources the user didn't ask for. A simple pipeline doesn't need AgentPolicy, AgentRole, or ToolPermission.
- Don't use deprecated fields (e.g., `graph.agent.next` — always use `edges`).
- Don't generate Worker resources unless the user is setting up a distributed deployment.
- Don't over-explain Orloj concepts unless the user seems new. If they say "I want a swarm", they probably know what that means.
