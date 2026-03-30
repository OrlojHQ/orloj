# ToolApproval

A **ToolApproval** captures a pending human/system approval request for a tool invocation that was flagged by a [ToolPermission](./tool-permission.md) `operation_rules` verdict of `approval_required`.

## How the Approval Workflow Works

When a tool call is flagged as `approval_required`:

1. The `GovernedToolRuntime` returns an `ErrToolApprovalRequired` sentinel error.
2. The task controller transitions the task to the `WaitingApproval` phase.
3. A `ToolApproval` resource is created with details about the pending call.
4. An external actor approves or denies the request via the API.
5. The task controller reconciles the approval status:
   - **Approved**: task resumes to `Running`.
   - **Denied**: task transitions to `Failed` with `approval_denied`.
   - **Expired** (TTL elapsed): task transitions to `Failed` with `approval_timeout`.

Approval-related outcomes are non-retryable and do not consume retry budget.

## ToolApproval Fields

```yaml
apiVersion: orloj.dev/v1
kind: ToolApproval
metadata:
  name: db-write-approval-001
spec:
  task_ref: weekly-report
  tool: database_tool
  operation_class: write
  agent: research-agent
  input: '{"query": "INSERT INTO ..."}'
  reason: "Write operation requires human approval"
  ttl: 10m
```

| Field | Description |
|---|---|
| `task_ref` | Name of the Task waiting for approval. |
| `tool` | Tool name that triggered the approval request. |
| `operation_class` | The operation class that requires approval. |
| `agent` | Agent that attempted the tool call. |
| `input` | Tool input payload (for audit context). |
| `reason` | Human-readable reason for the approval request. |
| `ttl` | Time-to-live before auto-expiry. Defaults to `10m`. |

## Status

| Field | Description |
|---|---|
| `phase` | `Pending`, `Approved`, `Denied`, `Expired`. |
| `decision` | `approved` or `denied`. |
| `decided_by` | Identity of the approver/denier. |
| `decided_at` | Timestamp of the decision. |
| `expires_at` | Timestamp when the approval expires. |

## API Endpoints

- `POST /v1/tool-approvals` -- create an approval request.
- `GET /v1/tool-approvals` -- list approval requests.
- `GET /v1/tool-approvals/{name}` -- get a specific approval.
- `DELETE /v1/tool-approvals/{name}` -- delete an approval.
- `POST /v1/tool-approvals/{name}/approve` -- approve a pending request. Body: `{"decided_by": "...", "reason": "..."}` (`reason` optional).
- `POST /v1/tool-approvals/{name}/deny` -- deny a pending request. Body: `{"decided_by": "...", "reason": "..."}` (`reason` optional).

## Related

- [ToolPermission](./tool-permission.md) -- defines which operations require approval
- [Governance Overview](./) -- how the governance resources work together
- [Resource Reference: ToolApproval](../../reference/resources/tool-approval.md)
