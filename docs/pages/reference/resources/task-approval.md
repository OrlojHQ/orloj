# TaskApproval

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

Captures a pending human review checkpoint for agent output or final task output.

## spec

- `task_ref` (string, required): task waiting for review.
- `checkpoint_id` (string, required): stable checkpoint identifier from the `AgentSystem`.
- `checkpoint_type` (string): `agent_output` or `task_output`.
- `agent` (string): producing agent.
- `reason` (string): reviewer-facing instructions.
- `ttl` (duration string): time-to-live before expiry. Defaults to `10m`.
- `allow_request_changes` (bool): whether reviewers may send the output back for revision. Defaults to `true`.
- `max_review_cycles` (int): maximum number of review loops permitted for this checkpoint. Defaults to `3`.
- `review_cycle` (int): review iteration number. Defaults to `1`.
- `supersedes` (string): previous `TaskApproval` name when this is a re-review cycle.
- `output` (string or object): frozen output snapshot presented to the reviewer.
- `output_format` (string): `text` or `json`.
- `resume_context` (object): runtime-owned context used to resume deterministically after review.

## status

- `phase` (string): `Pending`, `Approved`, `Denied`, `ChangesRequested`, `Expired`.
- `decision` (string): `approved`, `denied`, or `request_changes`.
- `decided_by` (string): reviewer identity.
- `decided_at` (string): RFC3339 timestamp of the decision.
- `comment` (string): optional reviewer comment. Required by the API for `request_changes`.
- `expires_at` (string): RFC3339 expiry timestamp.

## API Endpoints

- `POST /v1/task-approvals`
- `GET /v1/task-approvals`
- `GET /v1/task-approvals/{name}`
- `DELETE /v1/task-approvals/{name}`
- `POST /v1/task-approvals/{name}/approve`
- `POST /v1/task-approvals/{name}/deny`
- `POST /v1/task-approvals/{name}/request-changes`

Decision body:

```json
{
  "decided_by": "reviewer@example.com",
  "comment": "Tighten the medical disclaimer and regenerate."
}
```

`POST /v1/task-approvals/{name}/request-changes` requires `comment` or the legacy `reason` alias. It returns `409 Conflict` when `allow_request_changes` is `false` or the approval has already reached `max_review_cycles`.

See also:

- [TaskApproval concept](../../concepts/governance/task-approval.md)
- [Task](./task.md)
- [AgentSystem](./agent-system.md)
- [Human Review Checkpoints guide](../../guides/human-review-checkpoints.md)
