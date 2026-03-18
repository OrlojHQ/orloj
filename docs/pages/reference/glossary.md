# Glossary

Canonical definitions for terms used throughout Orloj documentation.

## A

**Agent**
A declarative unit of work backed by a language model. Defined as a resource with a prompt, model configuration, tool bindings, role assignments, and execution limits. See [Agents and Agent Systems](../concepts/agents-and-systems.md).

**Agent System**
A composition of multiple agents wired into a directed graph. The graph defines how messages flow between agents during task execution. Supports pipeline, hierarchical, and swarm-loop topologies. See [Agents and Agent Systems](../concepts/agents-and-systems.md).

**Agent Policy**
A governance resource that constrains agent execution. Can restrict allowed models, block specific tools, and cap token usage. Policies may be scoped to specific systems/tasks or applied globally. See [Governance and Policies](../concepts/governance.md).

**Agent Role**
A named set of permission strings that can be bound to agents. Agents accumulate the union of permissions from all bound roles. See [Governance and Policies](../concepts/governance.md).

## B

**Blueprint**
A ready-to-use template combining agents, an agent system, and a task for a specific orchestration pattern (pipeline, hierarchical, or swarm-loop). Available in `examples/blueprints/`. See [Starter Blueprints](../architecture/starter-blueprints.md).

## C

**Server**
The management layer of Orloj, running as `orlojd`. Includes the API server, resource store, background services, and task scheduler. See [Architecture Overview](../architecture/overview.md).

**Resource Definition**
A typed, declarative schema. Orloj defines 13 resource types with standard `apiVersion`, `kind`, `metadata`, `spec`, and `status` fields. See [Resource Reference](./resources.md).

## D

**Dead Letter**
A terminal state for tasks or messages that have exhausted all retry attempts. Dead-lettered items require manual investigation. Tasks transition `Failed -> DeadLetter` after all retries are consumed.

## E

**Edge**
A directional connection between two agents in an AgentSystem graph. Edges define message routing. The `edges[]` field supports fan-out (multiple targets) and metadata annotations via labels and policy.

## F

**Fan-in**
A graph pattern where multiple upstream branches converge on a single downstream node. Controlled by join gates with `wait_for_all` or `quorum` modes. See [Execution and Messaging](../architecture/execution-model.md).

**Fan-out**
A graph pattern where a single node routes messages to multiple downstream targets simultaneously.

## G

**Governance**
The authorization and policy enforcement layer. Composed of AgentPolicy, AgentRole, and ToolPermission resources. Governance is fail-closed: unauthorized actions are denied, not silently ignored. See [Governance and Policies](../concepts/governance.md).

## J

**Join Gate**
A fan-in mechanism on an AgentSystem graph node. Modes: `wait_for_all` (wait for every upstream branch) or `quorum` (wait for a count/percentage). Configurable failure policy: `deadletter`, `skip`, or `continue_partial`.

## L

**Lease**
A time-bounded claim on a task held by a worker. Workers renew leases via heartbeats during execution. If a lease expires (worker crash, network partition), another worker may safely take over the task.

## M

**Memory**
A resource that configures a persistent memory backend for agents. Agents that reference a Memory resource via `spec.memory.ref` gain access to built-in memory tools (`memory.read`, `memory.write`, `memory.search`, `memory.list`, `memory.ingest`). Configured with a type, provider (e.g. `in-memory`, `pgvector`), and optional embedding model. Memory operates in three layers: conversation history (per-activation), task-scoped shared store (per-task), and persistent backends (cross-task). See [Memory](../concepts/memory/index.md).

**Memory Tool**
One of five built-in tools automatically injected when an agent references a Memory resource: `memory.read`, `memory.write`, `memory.search`, `memory.list`, and `memory.ingest`. These are handled internally by the runtime without network calls. See [Memory](../concepts/memory/index.md).

**Message Bus**
The transport layer for agent-to-agent communication within a task. Implementations: `memory` (in-process) and `nats-jetstream` (durable). Messages carry lifecycle phase, retry state, and routing metadata.

**Model Endpoint**
A resource that configures a connection to a model provider. Declares the provider type, base URL, default model, provider-specific options, and auth credentials. Agents reference endpoints by name via `model_ref`. See [Model Routing](../concepts/model-routing.md).

**Model Gateway**
The worker component that routes model requests to the appropriate provider based on agent configuration. Handles provider-specific request formatting and response parsing.

## N

**Namespace**
A scope for resource names. Defaults to `default`. Resources can reference cross-namespace targets using `namespace/name` syntax.

## R

**Reconciliation**
The process by which background services observe the current state of a resource and take actions to move it toward the desired state declared in `spec`.

## S

**Secret**
A resource for storing sensitive values (API keys, tokens). `stringData` values are base64-encoded into `data` during normalization and then cleared. The runtime reads from `data` at execution time.

## T

**Task**
A request to execute an AgentSystem with specific input. Tasks move through phases: `Pending -> Running -> Succeeded | Failed | DeadLetter`. See [Tasks and Scheduling](../concepts/tasks-and-scheduling.md).

**Task Schedule**
A resource that creates tasks on a cron-based schedule from a template task. Supports timezone configuration, concurrency policy, and history limits. See [Tasks and Scheduling](../concepts/tasks-and-scheduling.md).

**Task Webhook**
A resource that creates tasks in response to external HTTP events. Supports signature verification (generic and GitHub profiles) and idempotency-based deduplication. See [Tasks and Scheduling](../concepts/tasks-and-scheduling.md).

**Tool**
An external capability that agents can invoke during execution. Defined as a resource with endpoint, auth, risk level, and runtime configuration (isolation, timeout, retry). See [Tools and Isolation](../concepts/tools-and-isolation.md).

**Tool Contract v1**
The standardized JSON request/response envelope that all tools must implement. Defines the error taxonomy (`tool_code`, `tool_reason`, `retryable`) used by the runtime for retry decisions. See [Tool Contract v1](./tool-contract-v1.md).

**Tool Permission**
A governance resource that defines what permissions are required to invoke a specific tool. Checked against the agent's accumulated role permissions at execution time. See [Governance and Policies](../concepts/governance.md).

## W

**Worker**
An execution unit that claims and runs tasks. Workers register capabilities (region, GPU, supported models) and the scheduler uses these for task matching. Runs as `orlojworker`. See [Tasks and Scheduling](../concepts/tasks-and-scheduling.md) and [Architecture Overview](../architecture/overview.md).
