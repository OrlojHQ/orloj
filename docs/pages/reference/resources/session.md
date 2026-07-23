# Session

> **Stability: alpha** -- Sessions are a new `orloj.dev/v1` API-store resource. The event contract is durable, but fields and event types may evolve before beta.

A Session is a long-lived conversation with an `AgentSystem`. Each submitted user message creates a queued turn. Orloj serializes turns for that Session, executes each turn through the selected AgentSystem, and stores an ordered event history for replay and reconnect.

Sessions do not replace Tasks. A Session turn creates a Task internally so existing AgentSystem orchestration, tool governance, approvals, and worker retry behavior continue to apply.

## spec

- `system` (string, required): target `AgentSystem` name.
- `idle_ttl` (duration string): expiry window after the last activity. Defaults to `24h`.
- `max_turns` (int, >= 0): optional completed-turn limit. `0` means unlimited.
- `input` (map[string]string): values added to each turn's internal Task input.
- `checkpoint_retention.max_count` (int, `0`–`100`): maximum retained safe-point snapshots. Defaults to `100`.
- `checkpoint_retention.max_age` (positive duration string, at most `720h`): maximum checkpoint age. Defaults to `168h`.

Checkpoint runtime state is limited to 4 MiB and remains server-private. Checkpoint APIs expose metadata and integrity hashes for replay, rewind, and fork operations, but never return raw runtime or tool-result state.

## status

- `phase`: `WaitingInput`, `Running`, `Paused`, `WaitingApproval`, `Failed`, `Cancelled`, `Completed`, or `Expired`.
- `activeTurnID`, `queuedTurns`, `completedTurns`
- `lastEventSequence`: highest durable event sequence available for replay.
- `lastCheckpointID`, `restoredCheckpoint`: current durable execution head and the checkpoint selected by recovery or rewind.
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
- `checkpoint.created`, `checkpoint.pruned`, `session.recovered`, `session.rewound`, `session.forked`

Clients reconnect to `GET /v1/sessions/{name}/stream` with `Last-Event-ID` or `?after=<seq>`. The server replays every later event before following live updates. On lease recovery, `message.reset` discards tentative output emitted after the safe point, then `session.recovered` identifies the checkpoint used to continue the agent loop.

## Checkpoints and time travel

Orloj snapshots the serializable agent loop after a complete ReAct step and at agent completion. Partial model streams are never treated as safe points. Checkpoint persistence is synchronous: execution does not advance if the snapshot cannot be stored.

- `GET /v1/sessions/{name}/checkpoints` lists snapshots newest first.
- `GET /v1/sessions/{name}/checkpoints/{id}` returns one snapshot.
- `GET .../{id}/replay` verifies the stored state hash and replays recorded events without model or tool calls.
- `POST .../{id}/rewind` fences current execution and restores the Session to a paused checkpoint. Pass `{"interrupt":true,"resume":true}` to replace an active turn and continue.
- `POST .../{id}/fork` with `{"name":"alternate"}` creates an independent paused Session with materialized conversation history. Forks continue with a new user turn; they do not resume the source's partially executing turn.

Replaying recorded data is deterministic. Continuing live execution after rewind or fork is not guaranteed to produce identical output because models and external systems may change. Tool calls receive stable Session/turn/step request IDs, but tools that do not honor idempotency remain at-least-once across a crash.

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
  checkpoint_retention:
    max_count: 100
    max_age: 168h
```

See [Interactive Sessions](../../guides/interactive-sessions.md) for complete API examples.
