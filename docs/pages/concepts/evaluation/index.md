# Agent Evaluation

Orloj includes a built-in evaluation framework for measuring and comparing agent system quality. Define golden datasets as declarative YAML, run them against your agent systems, score the results with multiple strategies (programmatic, LLM-as-judge, or human review), and compare runs side-by-side.

## Why Evaluate?

Model changes, prompt edits, tool updates, and graph topology changes can all silently degrade agent behavior. An evaluation framework lets you:

- **Detect regressions** before they reach production by running golden datasets against every change.
- **Compare configurations** (models, prompts, topologies) with objective metrics.
- **Involve humans** in scoring subjective output quality via an export/review/import workflow.
- **Track quality over time** with pass rates, mean scores, and latency trends.

## How It Works

The evaluation framework introduces two resource kinds:

| Resource | Purpose |
|---|---|
| **[EvalDataset](../../reference/resources/eval-dataset.md)** | A list of (input, expected output) pairs with optional per-sample scoring rubrics. |
| **[EvalRun](../../reference/resources/eval-run.md)** | A single evaluation execution: run a dataset against an agent system and collect scores. |

The workflow is:

```
1. Define a dataset       ──► orlojctl apply -f dataset.yaml
2. Start an eval run      ──► orlojctl eval run --dataset golden --system my-system
3. Controller creates     ──► one Task per sample, respecting concurrency limits
4. Workers execute tasks  ──► normal agent system execution
5. Scoring pipeline runs  ──► exact_match, llm_judge, manual, or custom
6. Results aggregated     ──► pass rate, mean score, latency, tokens
7. Compare runs           ──► orlojctl eval compare run-a run-b
```

## Scoring Strategies

Each sample can be scored with one of four strategies:

### exact_match

Compares agent output against expected fields using the same matching logic as graph edge conditions: `output_contains`, `output_matches` (regex), `output_json_path` with comparison operators. Binary 0/1 score.

```yaml
expected:
  output_contains: "billing"
  output_json_path: "$.category"
  equals: "billing"
```

### llm_judge

Sends the input, agent output, and a rubric to a judge model. The judge returns a score (0.0–1.0), pass/fail verdict, and reasoning. This is a single model call per sample, not a full agent execution.

```yaml
scoring:
  strategy: llm_judge
  model_ref: gpt-4o-judge
  rubric: "The response should correctly identify the customer's intent."
```

### manual

Tasks execute and outputs are collected, but no automated scoring occurs. The run transitions to **PendingReview** and results can be exported for human review (CSV or JSON), annotated via CLI or API, and then finalized.

```yaml
scoring:
  strategy: manual
```

### custom

Invokes an external [Tool](../tools/tool.md) with the sample input, expected output, and actual output. The tool returns a JSON score object. This lets you plug in any custom scoring logic.

```yaml
scoring:
  strategy: custom
  tool_ref: my-custom-scorer
```

## EvalRun Lifecycle

```
Pending ──► Running ──► Scoring ──► Succeeded
                                └──► PendingReview ──► Succeeded (after finalize)
                    └──► Failed
                    └──► Cancelled
```

| Phase | Meaning |
|---|---|
| `Pending` | Run is created, waiting for the controller to start task creation. |
| `Running` | Tasks are being created and executed. |
| `Scoring` | All tasks completed; the scoring pipeline is running. |
| `PendingReview` | Manual scoring: waiting for human annotations. |
| `Succeeded` | All scoring complete; summary metrics computed. |
| `Failed` | The eval run encountered a fatal error (e.g., missing dataset). |
| `Cancelled` | Cancelled by user; in-flight tasks are also cancelled. |

## Manual Review Workflow

When using `manual` scoring (or a mix where some samples use it):

1. **Tasks execute** normally and outputs are collected.
2. The run enters **PendingReview** once all tasks complete.
3. **Export** results for review:
   ```bash
   orlojctl eval export my-run --format csv > results.csv
   ```
4. **Annotate** individual samples:
   ```bash
   orlojctl eval annotate my-run --sample billing-q --score 0.8 --pass --comment "Good"
   ```
5. **Bulk import** from a reviewed CSV:
   ```bash
   orlojctl eval import my-run -f reviewed.csv
   ```
6. **Finalize** to compute aggregates and transition to Succeeded:
   ```bash
   orlojctl eval finalize my-run
   ```

## Comparing Runs

The comparison API and CLI show side-by-side metrics across multiple runs:

```bash
orlojctl eval compare run-gpt4o run-claude run-gemini
```

```
METRIC          run-gpt4o   run-claude   run-gemini
Pass Rate       88.0%       85.0%        82.0%
Mean Score      0.910       0.890        0.850
Tokens          13100       11800        12200
```

This is the primary tool for A/B testing model changes, prompt revisions, or topology experiments. Use `agent_overrides` in the EvalRun spec to swap prompts or models without modifying the base agent resources.

## Related

- [EvalDataset reference](../../reference/resources/eval-dataset.md) -- full spec documentation
- [EvalRun reference](../../reference/resources/eval-run.md) -- full spec documentation
- [Guide: Run Your First Agent Evaluation](../../guides/run-agent-evaluation.md) -- step-by-step tutorial
- [CLI: `orlojctl eval`](../../reference/cli.md) -- command reference
