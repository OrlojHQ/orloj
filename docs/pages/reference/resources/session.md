# Session

> **Stability: alpha** -- Sessions are a new `orloj.dev/v1` API-store resource. The event contract is durable, but fields and event types may evolve before beta.

A Session is a long-lived conversation with an `AgentSystem`. Each submitted user message creates a queued turn. Orloj serializes turns for that Session, executes each turn through the selected AgentSystem, and stores an ordered event history for replay and reconnect.

Sessions do not replace Tasks. A Session turn creates a Task internally so existing AgentSystem orchestration, tool governance, approvals, and worker retry behavior continue to apply.

## spec

- `system` (string, required): target `AgentSystem` name.
- `idle_ttl` (duration string): expiry window after the last activity. Defaults to `24h`.
- `max_turns` (int, >= 0): optional completed-turn limit. `0` means unlimited.
- `input` (map[string]string): values added to each turn's internal Task input.

## status

- `phase`: `WaitingInput`, `Running`, `Paused`, `WaitingApproval`, `Failed`, `Cancelled`, `Completed`, or `Expired`.
- `activeTurnID`, `queuedTurns`, `completedTurns`
- `lastEventSequence`: highest durable event sequence available for replay.
- `claimedBy`, `leaseUntil`, `lastHeartbeat`, `fence`: worker lease and stale-writer fencing.
- `startedAt`, `lastActivityAt`, `expiresAt`, `completedAt`
- `systemGeneration`: AgentSystem generation captured when the Session was created.

## Turn and event model

`POST /v1/sessions/{name}/turns` requires an `Idempotency-Key` header. Repeating the same key returns the original turn without adding a duplicate user message.

Session events have a strictly increasing per-Session `seq`. Important event types include:

- `message.created`, `message.delta`, `message.reset`, `message.completed`
- `turn.queued`, `turn.started`, `turn.retrying`, `turn.completed`, `turn.failed`, `turn.cancelled`
- `session.paused`, `session.resumed`, `session.cancelled`, `session.completed`, `session.expired`
- `approval.requested`, `approval.resolved`
- `tool.started`, `tool.completed`, `error`

Clients reconnect to `GET /v1/sessions/{name}/stream` with `Last-Event-ID` or `?after=<seq>`. The server replays every later event before following live updates. A `message.reset` tells clients to discard tentative assistant output for that message after interruption or worker takeover.

## Example

```yaml
apiVersion: orloj.dev/v1
kind: Session
metadata:
  name: support-chat
spec:
  system: support-system
  idle_ttl: 8h
  max_turns: 50
```

See [Interactive Sessions](../../guides/interactive-sessions.md) for complete API examples.
