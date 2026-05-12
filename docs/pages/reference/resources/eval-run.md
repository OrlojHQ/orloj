# EvalRun

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

An EvalRun executes all samples in an [EvalDataset](./eval-dataset.md) against an [AgentSystem](./agent-system.md), scores the results, and produces aggregate metrics. Runs track per-sample scores, latency, token usage, and pass/fail verdicts.

## Manifest

```yaml
apiVersion: orloj.dev/v1
kind: EvalRun
metadata:
  name: triage-gpt4o-20260510
  namespace: default
spec:
  dataset_ref: support-triage-golden
  system: support-triage-system
  scoring:
    strategy: llm_judge
    model_ref: gpt-4o-judge
    rubric: "Rate the accuracy and helpfulness of the triage response."
  concurrency: 5
  timeout: 120s
  agent_overrides:
    triage-agent:
      prompt: "You are a support triage bot. Classify and route."
      model_ref: gpt-4o-mini
```

## spec

| Field | Type | Description |
|---|---|---|
| `dataset_ref` | string, required | Name of the [EvalDataset](./eval-dataset.md) to evaluate against. |
| `system` | string, required | Name of the [AgentSystem](./agent-system.md) to evaluate. |
| `scoring` | EvalScoringConfig | Default scoring strategy for all samples. Per-sample overrides in the dataset take precedence. |
| `concurrency` | int | Maximum parallel tasks (samples) to execute. Defaults to 5. Minimum 1. |
| `timeout` | duration string | Per-sample task timeout (e.g. `120s`, `5m`). Must be a valid Go `time.Duration`. |
| `agent_overrides` | map[string]AgentOverride | Ephemeral overrides for agent configuration within this run. Keys are agent names. |
| `labels` | map[string]string | Arbitrary labels for filtering and comparison. |
| `suspended` | bool | When true, the controller will not execute this run. Defaults to `true` when created via `apply` (use `--run` to override or `orlojctl eval start` to trigger later). Defaults to `false` when created via `orlojctl eval run`. |

### AgentOverride

Used for A/B testing models, prompts, or parameters without modifying the base agent resource. The map key is the name of the agent to override.

| Field | Type | Description |
|---|---|---|
| `prompt` | string | Override the agent's system prompt. |
| `model_ref` | string | Override the agent's model endpoint. |

### EvalScoringConfig

See [EvalDataset](./eval-dataset.md#evalscoringconfig) for the full field list.

## Defaults and Validation

- `apiVersion` defaults to `orloj.dev/v1`.
- `kind` defaults to `EvalRun`.
- `metadata.namespace` defaults to `default`.
- `status.phase` defaults to `Pending`.
- `spec.suspended` defaults to `true` when created via `POST /v1/eval-runs` (unless `?run=true` is set). `orlojctl eval run` sets `?run=true` automatically.
- `spec.concurrency` defaults to 5; must be >= 1.
- `spec.dataset_ref` and `spec.system` are required.
- `spec.timeout` must be a valid Go duration string when set.
- `llm_judge` scoring requires `model_ref`.
- `custom` scoring requires `tool_ref`.
- Agent override names must be unique.

## status

| Field | Type | Description |
|---|---|---|
| `phase` | string | Current lifecycle phase (see below). |
| `message` | string | Human-readable status message. |
| `results` | []EvalSampleResult | Per-sample results. |
| `summary` | EvalSummary | Aggregate metrics computed after scoring. |
| `total_samples` | int | Total number of samples in the dataset. |
| `completed_samples` | int | Number of samples with completed tasks. |
| `scored_samples` | int | Number of samples that have been scored. |
| `errored_samples` | int | Number of samples that encountered errors. |

### EvalSampleResult

| Field | Type | Description |
|---|---|---|
| `sample_name` | string | Name matching the dataset sample. |
| `task_name` | string | Name of the task created for this sample. |
| `output` | string | Raw output from the agent system. |
| `score` | *float64 | Numeric score (0.0–1.0). Nil for unscored/manual. |
| `pass` | *bool | Pass/fail verdict. Nil for unscored. |
| `reasoning` | string | Explanation from the scorer (e.g. LLM judge reasoning). |
| `latency_ms` | int64 | Execution time in milliseconds. |
| `tokens_used` | int | Total tokens consumed by the task. |
| `error` | string | Non-empty if the sample errored. |

### EvalSummary

| Field | Type | Description |
|---|---|---|
| `pass_rate` | float64 | Fraction of scored samples that passed (0.0–1.0). |
| `mean_score` | float64 | Average score across all scored samples. |
| `total_tokens` | int | Sum of tokens across all samples. |
| `mean_latency_ms` | int64 | Average latency across all completed samples. |

## Lifecycle Phases

```
Pending ──► Running ──► Scoring ──► Succeeded
                                └──► PendingReview ──► Succeeded
                    └──► Failed
                    └──► Cancelled
```

| Phase | Meaning |
|---|---|
| `Pending` | Created, waiting for the controller. If `spec.suspended` is true, the controller skips the run until it is started. |
| `Running` | Tasks are being created and executed (up to `concurrency` in parallel). |
| `Scoring` | All tasks completed; scoring pipeline is evaluating results. |
| `PendingReview` | Manual scoring; awaiting human annotations before finalization. |
| `Succeeded` | Scoring complete; `status.summary` is final. |
| `Failed` | Fatal error during the run (e.g. missing dataset or system). |
| `Cancelled` | User-initiated cancellation. In-flight tasks are also cancelled. |

## API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/eval-runs` | List all runs. |
| `POST` | `/v1/eval-runs` | Create a new eval run. |
| `GET` | `/v1/eval-runs/{name}` | Get a run by name. |
| `PUT` | `/v1/eval-runs/{name}` | Update a run. |
| `DELETE` | `/v1/eval-runs/{name}` | Delete a run. |
| `GET` | `/v1/eval-runs/{name}/export` | Export results as JSON (or CSV with `?format=csv`). |
| `PUT` | `/v1/eval-runs/{name}/results/{sample}` | Annotate a single sample (manual review). |
| `POST` | `/v1/eval-runs/{name}/results` | Bulk import sample annotations. |
| `POST` | `/v1/eval-runs/{name}/start` | Start a suspended eval run. |
| `POST` | `/v1/eval-runs/{name}/finalize` | Finalize a PendingReview run (computes summary). |
| `POST` | `/v1/eval-runs/{name}/cancel` | Cancel a running eval. |
| `GET` | `/v1/eval-runs/compare?names=a,b,c` | Compare multiple runs side-by-side. |

## CLI Quick Reference

```bash
orlojctl eval run --dataset golden --system my-system      # Create and start a run
orlojctl eval start my-run                                  # Start a suspended run
orlojctl eval list                                          # List all runs
orlojctl eval get my-run                                    # Get run detail
orlojctl eval export my-run --format csv                    # Export for review
orlojctl eval annotate my-run --sample s1 --score 0.9      # Annotate sample
orlojctl eval import my-run -f reviewed.csv                 # Bulk import
orlojctl eval finalize my-run                               # Finalize manual run
orlojctl eval compare run-a run-b                           # Compare runs
orlojctl eval datasets                                      # List datasets
orlojctl apply -f eval-run.yaml                             # Apply (suspended by default)
orlojctl apply -f eval-run.yaml --run                       # Apply and start immediately
```

## Related

- [EvalDataset](./eval-dataset.md) -- define golden test data
- [Agent Evaluation (concept)](../../concepts/evaluation/) -- overview and workflow
- [Guide: Run Your First Agent Evaluation](../../guides/run-agent-evaluation.md)
- [AgentSystem](./agent-system.md) -- the systems being evaluated
