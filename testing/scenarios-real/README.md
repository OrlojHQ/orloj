# Real-Model Runtime Test Scenarios

This directory is the live-validation matrix for Orloj before OSS launch. It is organized as a small set of realistic scenario folders plus `Makefile` targets that turn them into repeatable gates.

## Prerequisites

1. Run `go test ./...` as a baseline before starting a live session.
2. Start `orlojd` in message-driven mode:

```bash
go run ./cmd/orlojd --task-execution-mode=message-driven --agent-message-bus-backend=memory
```

3. Start the worker that matches your scenario:

Anthropic model-only scenarios:

```bash
go run ./cmd/orlojworker \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume \
  --model-gateway-provider=anthropic
```

Anthropic tool-backed scenarios:

```bash
go run ./cmd/orlojworker \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume \
  --model-gateway-provider=anthropic \
  --tool-isolation-backend=container \
  --tool-container-network=bridge
```

4. Start the deterministic local stub tool service for tool-backed scenarios:

```bash
make real-tool-stub
```

5. Replace every provider `Secret.spec.stringData.value: replace-me` with a real API key before applying a scenario.

## Scenario Matrix

### Wave 0: existing flow hardening

1. `01-pipeline`
- Real-model planner -> research -> writer handoff.
- Gate checks final labeled output plus trace/message coverage.

2. `02-hierarchical`
- Manager/lead/worker/editor fan-out and join.
- Gate checks both worker branches reach the editor and the merged output is labeled.

3. `03-loop-max-turns`
- Cyclical manager/research loop with bounded `max_turns`.
- Gate checks repeated agent messages and labeled loop output.

4. `04-tool-call-smoke`
- Anthropic model uses a deterministic local stub HTTP tool.
- Gate checks tool-call trace events and exact smoke markers.

5. `05-tool-decision`
- Anthropic-backed A/B decision test: tool required vs self-contained.
- Gate checks both the use-tool and no-tool branches.

### Wave 1: memory-first validation

6. `06-memory-shared-handoff`
- SaaS incident escalation triage with shared memory across planner, researcher, and writer.
- Gate checks memory entries plus output derived from retrieved facts.

7. `07-memory-persistent-reuse`
- Two-task runbook reuse flow in the same memory backend.
- Gate checks seed + query behavior and verifies cross-task recall.

### Wave 2: controllable tools and governance

8. `08-tool-auth-and-contract`
- Authenticated HTTP tool with deterministic contract response.
- Gate checks auth path, tool call trace, and exact evidence marker.

9. `09-governance-real-deny`
- Real model with a real tool available, but intentionally missing permission grants.
- Gate checks fail-closed deny semantics and zero successful tool calls.

10. `10-tool-retry-recovery`
- Stub tool fails once, then succeeds on retry.
- Gate checks retry/error trace plus recovered final output.

### Wave 3: trigger paths

11. `11-webhook-live-flow`
- Signed webhook delivery creates a run task and writes to memory.
- Gate checks delivery acceptance, downstream task success, and memory entry creation.

12. `12-schedule-live-flow`
- Minute-level schedule creates a run task that writes to memory.
- Gate checks schedule trigger status, downstream task success, and memory entry creation.

## Key Targets

Apply a single scenario:

```bash
make real-apply-pipeline
make real-apply-memory-shared
make real-apply-tool-auth
```

Run a single gate:

```bash
make real-gate-pipeline
make real-gate-memory-shared
make real-gate-governance-deny
make real-gate-webhook
```

Run grouped gates:

```bash
make real-gate-wave0
make real-gate-wave1
make real-gate-wave2
make real-gate-wave3
```

Repeat a gate for release-candidate confidence:

```bash
make real-repeat TARGET=real-gate-pipeline COUNT=3
make real-repeat TARGET=real-gate-governance-deny COUNT=5
```

## Artifact Capture

Every scenario gate writes artifacts under:

```text
testing/artifacts/real/<namespace>/<task>/<timestamp>/
```

Captured files include:

- `task.json`
- `messages.json`
- `metrics.json`
- `memory-<name>.json` when the gate tracks memory
- `verdict.txt`

## Notes

- Tool-backed scenarios use `http://host.docker.internal:18080/...` in the manifests because the tool call originates inside the container isolation runtime.
- `07-memory-persistent-reuse` is applied in two steps by the `Makefile`: base resources + seed task first, then the query task.
- `11-webhook-live-flow` and `12-schedule-live-flow` create run tasks dynamically, so their `real-check-*` targets resolve the latest triggered task from resource status.
