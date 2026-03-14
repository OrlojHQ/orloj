# Starter Blueprints

These blueprints are copy/paste starting points for common multi-agent topologies.

All blueprints are designed for `task-execution-mode=message-driven` with worker inbox consumers enabled (`--agent-message-consume`).

## 1) Pipeline

Linear handoffs with explicit stage ownership.

- Directory: `examples/blueprints/pipeline/`
- Shape: `planner -> research -> writer`

Apply:

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/agents/planner_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/agents/research_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/agents/writer_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/agent-system.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/task.yaml
```

## 2) Hierarchical

Manager delegates to sub-managers; workers execute specialized tasks; editor joins outputs.

- Directory: `examples/blueprints/hierarchical/`
- Shape: `manager -> leads -> workers -> editor`
- Access control example: no direct `manager -> social-worker` edge

Apply:

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/agents/manager_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/agents/research_lead_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/agents/research_worker_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/agents/social_lead_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/agents/social_worker_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/agents/editor_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/agent-system.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/hierarchical/task.yaml
```

## 3) Swarm + Loop

Coordinator fans out to scouts and accepts iterative scout feedback, bounded by `max_turns`.

- Directory: `examples/blueprints/swarm-loop/`
- Shape: `coordinator <-> scouts`, plus `coordinator -> synthesizer`
- Safety: `Task.spec.max_turns` prevents unbounded loops

Apply:

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/swarm-loop/agents/coordinator_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/swarm-loop/agents/scout_alpha_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/swarm-loop/agents/scout_beta_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/swarm-loop/agents/scout_gamma_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/swarm-loop/agents/synthesizer_agent.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/swarm-loop/agent-system.yaml
go run ./cmd/orlojctl apply -f examples/blueprints/swarm-loop/task.yaml
```
