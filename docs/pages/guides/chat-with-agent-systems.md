# Chat with an AgentSystem

`orlojctl chat` opens a durable, multi-turn conversation with an AgentSystem.
It is intended for interactive investigation, iterative work, and supervised
tool use. It does not turn every pipeline into a continuously running chatbot.

## Mental model

Four resources participate in a chat:

- An **AgentSystem** is the workflow definition: its agents, graph, tools, and
  governance.
- A **Session** binds a durable conversation to one AgentSystem. It stores
  ordered turns and events and survives terminal disconnects.
- A **turn** is one user message within the Session.
- A **Task** executes the AgentSystem for one turn.

Every message entered in `orlojctl chat` creates a new Session-owned Task. The
Task receives the message as `topic` and receives the prior Session transcript,
so later turns can refer to earlier answers. After the Task finishes, the
Session waits for another message without consuming model compute.

```text
Session
  turn 1 -> Task 1 -> AgentSystem execution -> assistant response
  turn 2 -> Task 2 -> AgentSystem execution -> assistant response
  turn 3 -> Task 3 -> AgentSystem execution -> assistant response
```

## When to use chat

Use chat when a person will collaborate with or supervise the system:

- Investigating an incident and asking follow-up questions
- Iteratively drafting, reviewing, or refining an artifact
- Letting a coordinator route requests to specialist agents
- Reviewing ToolApprovals while work is in progress
- Disconnecting and later resuming the same conversation

Prefer another trigger when the work is not conversational:

- Use a `TaskSchedule` for a fixed pipeline that runs periodically.
- Use `orlojctl run` for one submitted job and one final result.
- Use a `TaskWebhook` for work triggered by an external event.
- Use the OpenAI-compatible API when integrating an existing OpenAI client
  that needs one completion rather than native Session control.

Each chat turn may execute the AgentSystem graph. Do not put unconditional
side effects in a chat-oriented graph unless running them for every user
message is intentional.

## Who receives the message?

You chat with the AgentSystem as a whole, not an arbitrarily selected agent.

For a graph system, Orloj starts with its entry agent or agents: nodes with no
incoming graph edges. Without a graph, it starts with the first agent in
`spec.agents`. Execution then follows normal graph routing, and the final
system output becomes the assistant response.

A chat-friendly system usually has:

1. One coordinator as the front door
2. Conditional routes for action requests
3. No matching route for answer-only conversations
4. Specialist agents behind the coordinator
5. A final synthesizer for multi-agent results
6. ToolApprovals around write or destructive operations

For example:

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: incident-response
spec:
  agents:
    - incident-coordinator
    - cluster-investigator
    - response-writer
  graph:
    incident-coordinator:
      edges:
        - to: cluster-investigator
          condition:
            output_contains: ACTION_INVESTIGATE
    cluster-investigator:
      next: response-writer
```

Prompt the coordinator to include `ACTION_INVESTIGATE` only when the user
explicitly requests an investigation. A question such as "What can you help
with?" has no matching edge, so the coordinator's answer ends that turn. A
request such as "Investigate checkout failures" follows the route through the
investigator and response writer.

Conditional routing is useful for intent, but it is not a security boundary.
Protect sensitive tool calls with AgentPolicy, ToolPermission, and
ToolApproval.

## Start a conversation

The AgentSystem must already exist, and `orlojd` must have an available worker:

```bash
orlojctl chat incident-response
```

The CLI creates an unlimited-turn Session and prints its name and exact resume
command:

```text
chat: created session/chat-incident-response-a8f32c for agentsystem/incident-response
chat: resume with `orlojctl chat incident-response --session chat-incident-response-a8f32c ...`
chat: enter /help for commands
you>
```

Use `--decided-by` to record an identity on inline approval decisions:

```bash
orlojctl chat incident-response --decided-by operator@example.com
```

The REPL supports:

- `/help` to list commands
- `/session` to show the active Session, system, and namespace
- `/exit` or `/quit` to leave without deleting the Session

## Review a tool action inline

When a ToolPermission requires approval, the current Task and Session pause.
The CLI fetches the exact ToolApproval and shows the tool, agent, operation
class, reason, expiry, and input:

```text
you> Fix the payment-cache Service selector.

approval: tool action requires review
  name: ta-73db120a
  tool: kubernetes.patch_service
  agent: remediation-agent
  operation: write
  reason: production write
  input:
    {
      "namespace": "production",
      "service": "payment-cache",
      "version": "v43"
    }
approval> [a]pprove, [d]eny, or [q]uit: a
comment> Confirmed against the active deployment
approval: approved tool-approval/ta-73db120a
assistant> The selector was updated and checkout recovered.
```

Approval resumes the same Task and stream. Denial fails the blocked execution.
Quitting leaves the Session and pending approval intact.

Inline decisions are available only on an interactive terminal. If stdin is
piped, the CLI stops and prints explicit `orlojctl approve` and
`orlojctl deny` commands rather than consuming the next scripted message as an
approval decision.

## Leave and resume

Exiting the REPL or pressing Ctrl-C disconnects the local client; it does not
delete or cancel the Session. Resume it by name:

```bash
orlojctl chat incident-response \
  --session chat-incident-response-a8f32c
```

If a turn is still running, the CLI reattaches to it. If it is waiting on a
ToolApproval, the approval prompt is restored. Session events use durable
sequence numbers, so transient stream disconnects reconnect without duplicating
events.

`--session` means "reconnect to this conversation." It is different from the
Session API's `POST /resume` control action, which unpauses a paused Session.

## Important boundaries

- Chat does not attach to an unrelated Task previously started with
  `orlojctl run`; every chat turn owns a separate Task.
- A Session preserves conversational history, but it is not itself an agent or
  a permanently running AgentSystem instance.
- Resuming requires the same AgentSystem and namespace used to create the
  Session.
- Terminal Sessions (`Completed`, `Failed`, `Cancelled`, or `Expired`) cannot
  accept more turns. Start a new chat instead.
- TaskApproval review checkpoints are reported with their existing
  `orlojctl approve`, `deny`, or `request-changes` workflow. Inline REPL
  decisions currently cover ToolApproval.

For direct Session API integration, event replay, interruption, pause/resume,
and checkpoint operations, see [Build an Interactive Agent Session](./interactive-sessions.md).
