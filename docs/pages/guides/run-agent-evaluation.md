# Run Your First Agent Evaluation

This guide walks you through creating a golden dataset, running an evaluation against an agent system, and comparing results across different configurations. By the end, you will have a repeatable evaluation workflow for measuring agent quality.

## Prerequisites

- Orloj server (`orlojd`) running (sequential mode with `--embedded-worker` is fine for this guide)
- `orlojctl` available (or `go run ./cmd/orlojctl`)
- At least one [AgentSystem](../concepts/agents/agent-system.md) and [ModelEndpoint](../reference/resources/model-endpoint.md) already applied

If you have not set up Orloj yet, follow the [Install](../getting-started/install.md) and [Quickstart](../getting-started/quickstart.md) guides first.

## What You Will Build

A golden dataset that tests a support triage agent, two eval runs (one per model), and a comparison showing which model performs better.

```
Dataset ──► EvalRun (gpt-4o) ──►  ┐
                                   ├──► Compare
Dataset ──► EvalRun (claude)  ──►  ┘
```

## Step 1: Define a Golden Dataset

A dataset contains sample inputs and expected outputs. Create `triage-dataset.yaml`:

```yaml
apiVersion: orloj.dev/v1
kind: EvalDataset
metadata:
  name: triage-golden
spec:
  description: "Golden set for the support triage agent"
  samples:
    - name: billing-question
      input:
        prompt: "I was charged twice for my subscription last month"
      expected:
        output_contains: "billing"
    - name: password-reset
      input:
        prompt: "I can't log in, I forgot my password"
      expected:
        output_contains: "password"
        output_not_contains: "billing"
    - name: refund-request
      input:
        prompt: "I want a refund for order #12345, the item arrived broken"
      expected:
        output_contains: "refund"
    - name: feature-request
      input:
        prompt: "It would be great if you could add dark mode"
      expected:
        output_contains: "feature"
```

Apply it:

```bash
orlojctl apply -f triage-dataset.yaml
```

Verify:

```bash
orlojctl eval datasets
```

```
NAME             NAMESPACE   SAMPLES   DESCRIPTION
triage-golden    default     4         Golden set for the support triage agent
```

## Step 2: Run an Evaluation with exact_match

The simplest scoring strategy is `exact_match`, which checks expected output fields against the agent's actual output. Start a run:

```bash
orlojctl eval run \
  --dataset triage-golden \
  --system support-triage-system
```

The CLI creates an EvalRun resource and polls until completion. You will see output like:

```
EvalRun triage-golden-run-a1b2c3 created (Pending)
Phase: Running (2/4 samples completed)
Phase: Scoring...

EvalRun triage-golden-run-a1b2c3: Succeeded
Pass Rate: 75.0%   Mean Score: 0.750   Tokens: 2340   Latency: 1.8s

SAMPLE              PASS   SCORE   LATENCY
billing-question    ✓      1.000   1.2s
password-reset      ✓      1.000   1.5s
refund-request      ✓      1.000   2.1s
feature-request     ✗      0.000   2.4s
```

You can also create runs declaratively:

```yaml
apiVersion: orloj.dev/v1
kind: EvalRun
metadata:
  name: triage-eval-exact
spec:
  dataset_ref: triage-golden
  system: support-triage-system
  scoring:
    strategy: exact_match
  concurrency: 2
  timeout: 60s
```

```bash
orlojctl apply -f eval-run.yaml               # creates the run in suspended state
orlojctl eval start triage-eval-exact          # start it when ready
# or apply and start immediately:
orlojctl apply -f eval-run.yaml --run
```

## Step 3: Run with LLM-as-Judge

For subjective quality assessment, use `llm_judge`. The judge model evaluates each sample against a rubric:

```bash
orlojctl eval run \
  --dataset triage-golden \
  --system support-triage-system \
  --scoring llm_judge \
  --model-ref gpt-4o-judge \
  --rubric "Rate accuracy and helpfulness of the triage classification (0-1)."
```

Or declaratively:

```yaml
apiVersion: orloj.dev/v1
kind: EvalRun
metadata:
  name: triage-eval-llm
spec:
  dataset_ref: triage-golden
  system: support-triage-system
  scoring:
    strategy: llm_judge
    model_ref: gpt-4o-judge
    rubric: "Rate accuracy and helpfulness of the triage classification (0-1)."
  concurrency: 4
  timeout: 120s
```

The judge model must be configured as a [ModelEndpoint](../reference/resources/model-endpoint.md) like any other model.

## Step 4: A/B Test with Agent Overrides

Compare two models without modifying your agents. Use `agent_overrides` to swap the model for a specific agent:

```yaml
apiVersion: orloj.dev/v1
kind: EvalRun
metadata:
  name: triage-eval-claude
spec:
  dataset_ref: triage-golden
  system: support-triage-system
  scoring:
    strategy: llm_judge
    model_ref: gpt-4o-judge
    rubric: "Rate accuracy and helpfulness of the triage classification (0-1)."
  concurrency: 4
  agent_overrides:
    triage-agent:
      model_ref: claude-sonnet
```

Apply and start:

```bash
orlojctl apply -f eval-claude.yaml --run
orlojctl eval get triage-eval-claude -w
```

## Step 5: Compare Runs

Compare any number of completed runs side-by-side:

```bash
orlojctl eval compare triage-eval-llm triage-eval-claude
```

```
METRIC          triage-eval-llm   triage-eval-claude
Pass Rate       100.0%            75.0%
Mean Score      0.920             0.850
Tokens          4200              3800
Mean Latency    2.1s              1.8s
```

The comparison API also returns per-sample deltas accessible via the REST API:

```bash
curl "http://localhost:8080/v1/eval-runs/compare?names=triage-eval-llm,triage-eval-claude"
```

## Step 6: Manual Review (Optional)

For subjective tasks where automated scoring is not appropriate, use `manual` scoring:

```bash
orlojctl eval run \
  --dataset triage-golden \
  --system support-triage-system \
  --scoring manual
```

Once the tasks complete, the run transitions to **PendingReview**. Export the results:

```bash
orlojctl eval export triage-golden-run-xyz --format csv > review.csv
```

The CSV contains columns: `sample_name`, `input`, `output`, `score`, `pass`, `reasoning`. Fill in the `score`, `pass`, and `reasoning` columns, then import:

```bash
orlojctl eval import triage-golden-run-xyz -f review.csv
```

Finalize to compute aggregate metrics:

```bash
orlojctl eval finalize triage-golden-run-xyz
```

## Step 7: Inspect Results

Get full details for any run:

```bash
orlojctl eval get triage-eval-llm -o yaml
```

List all runs:

```bash
orlojctl eval list
```

```
NAME                   DATASET          SYSTEM                  PHASE       PASS RATE   SAMPLES
triage-eval-exact      triage-golden    support-triage-system   Succeeded   75.0%       4
triage-eval-llm        triage-golden    support-triage-system   Succeeded   100.0%      4
triage-eval-claude     triage-golden    support-triage-system   Succeeded   75.0%       4
```

## Tips

- **Start small:** begin with `exact_match` on 5–10 samples to validate the workflow before scaling up.
- **Use concurrency wisely:** higher concurrency runs faster but consumes more model quota. Start with 2–4.
- **Version your datasets:** use descriptive names like `triage-golden-v2` to track dataset evolution.
- **Automate with TaskSchedule:** create a [TaskSchedule](../concepts/tasks/task-schedule.md) or webhook to run evaluations on every deployment.
- **Export for dashboards:** pipe `orlojctl eval export` output to your observability stack for trend tracking.

## Related

- [Agent Evaluation (concept)](../concepts/evaluation/) -- framework overview
- [EvalDataset reference](../reference/resources/eval-dataset.md) -- full spec documentation
- [EvalRun reference](../reference/resources/eval-run.md) -- full spec documentation
- [CLI Reference: `orlojctl eval`](../reference/cli.md) -- all eval subcommands
