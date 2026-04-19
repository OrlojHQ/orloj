# TaskApproval

`TaskApproval` extends Orloj approvals from "may this tool call happen?" to "is this output safe and acceptable to continue?"

Use it when you want a human to review:

- a sensitive agent handoff before downstream agents continue
- a regulated draft before publication or external delivery
- a final task result before the task is marked `Succeeded`

## How It Works

1. An `AgentSystem` checkpoint is configured on a node (`spec.graph.<node>.review`) or on final completion (`spec.completion_review`).
2. The agent runs normally.
3. Orloj pauses the task in `WaitingApproval`, stores the exact blocker in `Task.status.blocked_on`, and creates a `TaskApproval`.
4. A reviewer chooses:
   - `approve`: resume the workflow
   - `deny`: fail the task
   - `request_changes`: rerun the same producing agent with reviewer feedback injected as `review.*` runtime input
5. If a rerun hits the same checkpoint again, Orloj creates a new `TaskApproval` with an incremented `review_cycle` and a `supersedes` link to the prior review.

`request_changes` is optional per checkpoint. If `allow_request_changes` is set to `false`, reviewers can only approve or deny. When `max_review_cycles` is reached, Orloj rejects further `request_changes` decisions with `409 Conflict`.

## Checkpoint Configuration

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: regulated-writer
spec:
  agents:
    - writer-agent
    - compliance-agent
  graph:
    writer-agent:
      review:
        checkpoint_id: writer-review
        display_name: Writer Review
        reason: Human reviewer must approve the draft before compliance continues.
        ttl: 30m
        allow_request_changes: true
        max_review_cycles: 3
      next: compliance-agent
  completion_review:
    checkpoint_id: publish-review
    reason: Final human signoff is required before the task is marked succeeded.
```

## Review Context

For `request_changes`, Orloj injects:

- `review.feedback`
- `review.previous_output`
- `review.checkpoint_id`
- `review.cycle`
- `review.requested_by`

That lets the same agent revise its output with concrete human guidance instead of starting from scratch.

Reviewers send that feedback through `POST /v1/task-approvals/{name}/request-changes` or `orlojctl request-changes task-approval <name> ...`. The request must include `comment` or the legacy `reason` field.

## When To Use It

- Healthcare: review a triage note or patient-facing summary before delivery.
- Finance: review a risk classification or client communication before release.
- Insurance: review a claim denial rationale or settlement summary before sending.

See also:

- [ToolApproval](./tool-approval.md)
- [Human Review Checkpoints guide](../../guides/human-review-checkpoints.md)
- [TaskApproval resource reference](../../reference/resources/task-approval.md)
