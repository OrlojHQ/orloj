# Human Review Checkpoints

This guide shows how to pause an Orloj workflow for human review of agent output or final task output.

Example manifests in the repo:

- [`examples/resources/agent-systems/review_sequential_system.yaml`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/agent-systems/review_sequential_system.yaml)
- [`examples/resources/agent-systems/review_message_driven_system.yaml`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/agent-systems/review_message_driven_system.yaml)
- [`examples/resources/tasks/review_sequential_task.yaml`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/tasks/review_sequential_task.yaml)
- [`examples/resources/tasks/review_message_driven_task.yaml`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/tasks/review_message_driven_task.yaml)

## Sequential Example

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: seq-review-system
spec:
  agents:
    - draft-agent
    - publish-agent
  graph:
    draft-agent:
      review:
        checkpoint_id: draft-review
        reason: Editor must approve the draft before publication.
      next: publish-agent
  completion_review:
    checkpoint_id: final-review
    reason: Final signoff before success.
```

When `draft-agent` finishes, the task pauses in `WaitingApproval` and a `TaskApproval` is created. If the reviewer requests changes, Orloj reruns `draft-agent` with `review.*` fields in its input.

Use `allow_request_changes: false` on a checkpoint when reviewers should only approve or deny. Use `max_review_cycles` to cap how many revision loops a single checkpoint can trigger.

## Message-Driven Example

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: msg-review-system
spec:
  agents:
    - intake-agent
    - decision-agent
  graph:
    intake-agent:
      review:
        checkpoint_id: intake-review
        reason: Human reviewer must approve intake output before routing.
      edges:
        - to: decision-agent
```

In message-driven mode, Orloj freezes the downstream messages, creates a `TaskApproval`, and only publishes those messages after approval.

## Reviewer Commands

```bash
orlojctl get task-approvals
orlojctl approve task-approval my-approval --decided-by reviewer@example.com --comment "Looks good"
orlojctl deny task-approval my-approval --decided-by reviewer@example.com --comment "Do not send this"
orlojctl request-changes task-approval my-approval --decided-by reviewer@example.com --comment "Add a compliance disclaimer"
```

`request-changes` requires reviewer feedback via `--comment` or the legacy `--reason` alias. The API and CLI return a conflict if that checkpoint disables `request_changes` or if it has already reached `max_review_cycles`.

## What To Watch

- `Task.status.phase = WaitingApproval`
- `Task.status.blocked_on` for the exact approval resource
- `TaskApproval.spec.review_cycle` and `spec.supersedes` for re-review loops

See also:

- [TaskApproval concept](../concepts/governance/task-approval.md)
- [TaskApproval resource reference](../reference/resources/task-approval.md)
