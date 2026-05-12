# EvalDataset

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

An EvalDataset is a declarative list of (input, expected output) pairs, optionally with per-sample scoring rubrics. Datasets are applied like any other resource and referenced by [EvalRun](./eval-run.md) resources to drive evaluations.

## Manifest

```yaml
apiVersion: orloj.dev/v1
kind: EvalDataset
metadata:
  name: support-triage-golden
  namespace: default
spec:
  description: "Golden set for the support triage agent"
  samples:
    - name: billing-question
      input:
        prompt: "I was charged twice for my subscription"
      expected:
        output_contains: "billing"
        output_json_path: "$.category"
        equals: "billing"
    - name: refund-request
      input:
        prompt: "I want a refund for order #12345"
      expected:
        output_contains: "refund"
      scoring:
        strategy: llm_judge
        model_ref: gpt-4o-judge
        rubric: "The response should identify this as a refund request and extract the order number."
```

## spec

- `description` (string, optional): human-readable description of the dataset.
- `samples` ([]EvalSample, required, min 1): the evaluation cases.

### EvalSample

| Field | Type | Description |
|---|---|---|
| `name` | string, required | Unique name within the dataset (case-insensitive uniqueness). |
| `input` | map[string]string, required | Input passed to the agent system as `task.spec.input`. Must be non-empty. |
| `expected` | EvalExpected, optional | Expected output criteria for `exact_match` scoring. |
| `scoring` | EvalScoringConfig, optional | Per-sample scoring override. When set, takes precedence over the run-level default. |

### EvalExpected

Mirrors [EdgeCondition](./agent-system.md) semantics. All non-empty fields must match (logical AND).

| Field | Type | Description |
|---|---|---|
| `output_contains` | string | Output must contain this substring (case-insensitive). |
| `output_not_contains` | string | Output must NOT contain this substring (case-insensitive). |
| `output_matches` | string | Output must match this regex. Validated during normalization. |
| `output_json_path` | string | Dot-notation JSON path (e.g. `$.category`). Requires a comparison operator. |
| `equals` | string | JSON path value must equal this string. |
| `not_equals` | string | JSON path value must NOT equal this string. |
| `contains` | string | JSON path value (string or array) must contain this value. |
| `greater_than` | string | JSON path numeric value must be greater than this threshold. |
| `less_than` | string | JSON path numeric value must be less than this threshold. |

### EvalScoringConfig

| Field | Type | Description |
|---|---|---|
| `strategy` | string | One of: `exact_match`, `llm_judge`, `manual`, `custom`. |
| `model_ref` | string | Required for `llm_judge`. References a [ModelEndpoint](./model-endpoint.md). |
| `rubric` | string | Evaluation rubric for `llm_judge`. |
| `tool_ref` | string | Required for `custom`. References a [Tool](./tool.md). |

## Defaults and Validation

- `apiVersion` defaults to `orloj.dev/v1`.
- `kind` defaults to `EvalDataset`.
- `metadata.namespace` defaults to `default`.
- `status.phase` defaults to `Ready`.
- Sample names must be unique within a dataset (case-insensitive).
- Each sample must have at least one input entry.
- `output_matches` is validated as a valid regex during normalization.
- `llm_judge` strategy requires `model_ref`.
- `custom` strategy requires `tool_ref`.

## status

- `phase`: `Ready` (datasets are config-only; no controller moves them through phases).

## API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/eval-datasets` | List all datasets (supports `namespace`, `limit`, `continue` query params). |
| `POST` | `/v1/eval-datasets` | Create or update a dataset. |
| `GET` | `/v1/eval-datasets/{name}` | Get a dataset by name. |
| `PUT` | `/v1/eval-datasets/{name}` | Update a dataset. |
| `DELETE` | `/v1/eval-datasets/{name}` | Delete a dataset. |

## Related

- [EvalRun](./eval-run.md) -- runs a dataset against an agent system
- [Agent Evaluation (concept)](../../concepts/evaluation/) -- overview of the evaluation framework
- [Guide: Run Your First Agent Evaluation](../../guides/run-agent-evaluation.md)
