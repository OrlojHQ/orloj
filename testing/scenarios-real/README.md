# Real-Model Runtime Test Scenarios

This directory is for live runtime validation with real model providers.

Each scenario is self-contained and includes:

1. provider secret manifest
2. `ModelEndpoint` manifest
3. agent/system/task manifests

## Before You Run

1. Set worker/controller to message-driven mode with inbox consumers.
2. Use a real provider in runtime (for example `ORLOJ_MODEL_GATEWAY_PROVIDER=openai` or `anthropic`).
3. Replace `spec.stringData.value` in each scenario's `secret.yaml`.
4. For tool-call smoke, run workers with container isolation:
   `--tool-isolation-backend=container`

## Apply Pattern

```bash
find testing/scenarios-real/01-openai-pipeline -name '*.yaml' -print | sort | xargs -I{} go run ./cmd/orlojctl apply -f {}
```

Swap the folder name for other scenarios.

## Makefile Shortcuts

From repo root:

```bash
make real-apply-pipeline
make real-check-pipeline
```

Other scenario shortcuts:

- `make real-apply-hier`
- `make real-apply-loop`
- `make real-apply-tool`
- `make real-apply-anthropic-tool-decision`
- `make real-check-hier`
- `make real-check-loop`
- `make real-check-tool`
- `make real-check-anthropic-tool-use`
- `make real-check-anthropic-tool-no-use`
- `make real-gate-anthropic-tool-decision`
- `make real-check-all`

Generic forms:

```bash
make real-apply SCENARIO=01-openai-pipeline
make real-check NS=rr-real-pipeline TASK=rr-real-pipeline-task
```

## Scenarios

1. `01-openai-pipeline`
- Real-model pipeline handoffs (`planner -> research -> writer`).
- Goal: validate stable handoff quality and end-to-end success.

2. `02-openai-hierarchical`
- Manager/lead/worker/editor pattern with fan-in join.
- Goal: validate multi-branch completion and join behavior with real responses.

3. `03-openai-loop-max-turns`
- Cyclical manager/research dialogue bounded by `max_turns`.
- Goal: validate loop termination and output quality across turns.

4. `04-openai-tool-call-smoke`
- Real model instructed to call an HTTP tool.
- Goal: validate model tool-selection + tool invocation path.
- Uses public echo endpoint `https://postman-echo.com/post` for the smoke call.

5. `05-anthropic-tool-decision`
- Anthropic Sonnet A/B tool-decision scenario with paired tasks.
- Goal: validate "use tool when needed, skip tool when self-contained" behavior.
- Uses public echo endpoint `https://postman-echo.com/post` and container tool isolation.

## Suggested Checks

```bash
go run ./cmd/orlojctl get task rr-real-pipeline-task --namespace rr-real-pipeline
curl -s "http://localhost:8080/v1/tasks/rr-real-pipeline-task/messages?namespace=rr-real-pipeline" | jq .
curl -s "http://localhost:8080/v1/tasks/rr-real-pipeline-task/metrics?namespace=rr-real-pipeline" | jq .
```

Repeat with task/namespace for each scenario.

## Scenario Task/Namespace Pairs

1. `rr-real-pipeline-task` / `rr-real-pipeline`
2. `rr-real-hier-task` / `rr-real-hier`
3. `rr-real-loop-task` / `rr-real-loop`
4. `rr-real-tool-task` / `rr-real-tool`
5. `rr-anthropic-tool-use-task` / `rr-real-anthropic-tool-decision`
6. `rr-anthropic-tool-no-use-task` / `rr-real-anthropic-tool-decision`
