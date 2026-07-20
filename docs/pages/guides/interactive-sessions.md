# Build an Interactive Agent Session

Sessions add a durable, multi-turn conversation around an existing AgentSystem. The AgentSystem still defines the agents, graph, tools, and governance. The Session remembers the conversation, queues user turns, and exposes a reconnectable event stream.

## Create a Session

```bash
curl -sS http://127.0.0.1:8080/v1/sessions \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "orloj.dev/v1",
    "kind": "Session",
    "metadata": {"name": "support-chat"},
    "spec": {
      "system": "support-system",
      "idle_ttl": "8h"
    }
  }'
```

`spec.system` is an AgentSystem name, not a foundation-model identifier. The AgentSystem must exist in the same namespace.

## Submit a user turn

```bash
curl -sS http://127.0.0.1:8080/v1/sessions/support-chat/turns \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: message-001" \
  -d '{"content":"Can you investigate order 4812?"}'
```

The response is `202 Accepted` for a new queued turn. Retrying with the same `Idempotency-Key` returns the original turn with `200 OK`; it does not add the message twice.

Turns submitted concurrently are queued in durable order. Orloj runs one turn at a time per Session so conversation history remains deterministic.

To replace an active turn with newer human input, submit the new turn with
`"interrupt": true`. Orloj fences the active worker, emits `message.reset` and
`turn.cancelled`, then runs the replacement turn.

## Stream and reconnect

```bash
curl -N http://127.0.0.1:8080/v1/sessions/support-chat/stream \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN"
```

Each SSE frame has:

- `id`: the durable per-Session sequence
- `event`: a typed event such as `message.delta` or `turn.completed`
- `data`: the complete SessionEvent JSON

Save the latest `id`. After a network disconnect, reconnect with:

```bash
curl -N http://127.0.0.1:8080/v1/sessions/support-chat/stream \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN" \
  -H "Last-Event-ID: 42"
```

Orloj first replays events `43` onward, then follows live events. You can also inspect a bounded page without opening SSE:

```bash
curl -sS "http://127.0.0.1:8080/v1/sessions/support-chat/events?after=42"
```

## Supervise a Session in the Console

Open **Sessions** in the Console and select a Session to follow its durable event
stream. The live timeline combines assistant output with turn lifecycle, tool,
approval, error, and Session checkpoint events. The connection badge shows when
the stream is live or reconnecting; after reconnecting, the Console resumes from
the latest event sequence without duplicating activity.

You can intervene while a turn is running:

- Send new steering input and select **Interrupt active turn** to replace the
  in-flight turn.
- Pause execution to fence the current worker and retain the turn for another
  attempt.
- Resume a paused Session or cancel it permanently.

Session checkpoint markers appear at safe execution boundaries. Select a marker
to:

- **Replay** the checkpoint read-only and verify its state hash and event
  history. Replay never calls a model or tool.
- **Rewind** the Session to that point, optionally interrupting the active turn
  and resuming immediately.
- **Fork** an independent Session from that point, leaving the source unchanged.

After a rewind, superseded events remain available in a collapsed **Abandoned
timeline** section for audit. The active checkpoint lineage and current live tail
remain visually distinct. This is different from an AgentSystem review
checkpoint, which pauses a Task for human approval rather than storing
time-travel state.

## Pause, resume, and cancel

```bash
curl -X POST http://127.0.0.1:8080/v1/sessions/support-chat/pause \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason":"Waiting for a customer decision"}'

curl -X POST http://127.0.0.1:8080/v1/sessions/support-chat/resume \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN"

curl -X POST http://127.0.0.1:8080/v1/sessions/support-chat/cancel \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN"
```

Pausing fences the current Session worker. If tentative assistant output was visible, the event stream emits `message.reset` before the turn can be attempted again. Cancelling is terminal and rejects later turns.

## Recover, replay, rewind, and fork

Orloj stores a checkpoint after each complete agent step. If a worker lease expires, another worker restores the latest compatible checkpoint and continues with a new fence instead of repeating the whole turn.

List checkpoints:

```bash
curl -sS http://127.0.0.1:8080/v1/sessions/support-chat/checkpoints \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN"
```

Replay validates recorded state and events without invoking a model or tool:

```bash
curl -sS \
  http://127.0.0.1:8080/v1/sessions/support-chat/checkpoints/CHECKPOINT_ID/replay \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN"
```

Rewind the current Session and resume from a safe point:

```bash
curl -X POST \
  http://127.0.0.1:8080/v1/sessions/support-chat/checkpoints/CHECKPOINT_ID/rewind \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"interrupt":true,"resume":true}'
```

Fork an alternate timeline without changing the original:

```bash
curl -X POST \
  http://127.0.0.1:8080/v1/sessions/support-chat/checkpoints/CHECKPOINT_ID/fork \
  -H "Authorization: Bearer $ORLOJ_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"support-chat-alternate"}'
```

Forks start paused by default. Set `"resume":true` to move the new Session to `WaitingInput`. The fork owns a copy of its checkpoint and inherited conversation events, so deleting or pruning the source does not break it.

## Session versus Task

Use a **Task** for one submitted job and one final result. Use a **Session** when a user will send follow-up messages, watch the answer arrive, reconnect, or intervene.

Each Session turn is still executed through a Task internally. This preserves the existing AgentSystem execution engine and governance rather than introducing a separate ungoverned chat path.
