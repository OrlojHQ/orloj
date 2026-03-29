# Contributing Use-Case Templates

This guide defines the contract for new scenario templates under `examples/use-cases/`.

## Required Baseline Structure

Each new scenario directory must include:

- `README.md` based on [`TEMPLATE.md`](./TEMPLATE.md)
- agent definitions (`agents/*.yaml`) for the scenario
- `agent-system.yaml`
- at least one runnable task (`task.yaml` or `task-template.yaml` with trigger resources)
- `model-endpoint.yaml` and `secret-*.yaml` if the scenario is intended to be copy-paste complete

## Optional Resource Extensions

Use-case bundles are not limited to baseline resources. Add other kinds whenever they make the scenario more realistic:

- Governance: `AgentPolicy`, `AgentRole`, `ToolPermission`, `ToolApproval`
- Tooling and memory: `Tool`, `Memory`, `McpServer`
- Runtime routing and triggers: `Worker`, `TaskSchedule`, `TaskWebhook`
- Additional scenario tasks and templates for multi-step flows

Use a unique prefix for all resource names (for example `uc-<scenario>-*`) to avoid collisions with other examples.

## README Contract

Scenario README files must include:

1. Problem statement (`What this is for`)
2. Audience (`Who it is for`)
3. Decision guidance (`When to use something else`)
4. Topology diagram (Mermaid)
5. Apply command(s), including optional governance/integration resources when present
6. Expected output markers
7. Cleanup instructions when resources are long-lived

## Validation Before PR

From repository root:

```bash
go run ./cmd/orlojctl validate -f examples/use-cases/<scenario>/
go run ./cmd/orlojctl validate -f examples/
```

If you add docs links, verify they resolve locally and do not break the markdown link CI workflow.

## Labels and Issue Intake

Use the `Good first task` issue form when proposing onboarding-friendly scenario work.

Recommended labels for scenario work:

- `examples`
- `docs`
- `good first issue` or `help wanted`

## Scenario Contribution Track Cadence

Maintainers run one scenario contribution track every 2 weeks:

- 2 beginner docs/example tasks
- 1 intermediate scenario task

When opening track tasks, include acceptance criteria and expected output markers so contributors can self-verify before review.
