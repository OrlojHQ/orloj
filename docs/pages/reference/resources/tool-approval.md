# ToolApproval

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

Captures a pending human/system approval request for a tool invocation that was flagged by a `ToolPermission` `operation_rules` verdict of `approval_required`.

## spec

- `task_ref` (string, required): name of the Task resource waiting for approval.
- `tool` (string, required): tool name that triggered the approval request.
- `operation_class` (string): the operation class that requires approval.
- `agent` (string): agent that attempted the tool call.
- `input` (string): tool input payload (for audit context).
- `reason` (string): human-readable reason for the approval request.
- `ttl` (duration string): time-to-live before auto-expiry. Defaults to `10m`.

## Defaults and Validation

## status

- `phase` (string): `Pending`, `Approved`, `Denied`, `Expired`. Defaults to `Pending`.
- `decision` (string): `approved` or `denied`.
- `decided_by` (string): identity of the approver/denier.
- `decided_at` (string): RFC3339 timestamp of the decision.
- `expires_at` (string): RFC3339 timestamp when the approval expires.

## API Endpoints

- `POST /v1/tool-approvals` -- create an approval request.
- `GET /v1/tool-approvals` -- list approval requests (supports namespace and label filters).
- `GET /v1/tool-approvals/{name}` -- get a specific approval.
- `DELETE /v1/tool-approvals/{name}` -- delete an approval.
- `POST /v1/tool-approvals/{name}/approve` -- approve a pending request. Body: `{"decided_by": "..."}`.
- `POST /v1/tool-approvals/{name}/deny` -- deny a pending request. Body: `{"decided_by": "..."}`.

See also: [Tool approval concepts](../../concepts/governance/tool-approval.md).
