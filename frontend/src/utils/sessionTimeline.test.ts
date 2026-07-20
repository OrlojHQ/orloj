import type { SessionCheckpoint, SessionEvent } from "../api/types";
import {
  activeSessionCheckpointIDs,
  buildSessionTimeline,
} from "./sessionTimeline";

function assert(condition: boolean, message: string) {
  if (!condition) throw new Error(message);
}

function event(
  seq: number,
  type: string,
  payload: Record<string, unknown> = {},
  extra: Partial<SessionEvent> = {},
): SessionEvent {
  return {
    seq,
    type,
    payload,
    timestamp: `2026-07-20T12:00:${String(seq).padStart(2, "0")}Z`,
    ...extra,
  };
}

function checkpoint(
  id: string,
  eventSequence: number,
  parentCheckpointID = "",
): SessionCheckpoint {
  return {
    id,
    session_name: "timeline",
    namespace: "default",
    event_sequence: eventSequence,
    parent_checkpoint_id: parentCheckpointID || undefined,
    safe_point: "step.completed",
    state_version: 1,
    state_hash: `${id}-hash`,
    state: { version: 1 },
    created_at: `2026-07-20T12:00:${String(eventSequence).padStart(2, "0")}Z`,
    agent: "researcher",
  };
}

function testMessageCoalescingAndReset() {
  const timeline = buildSessionTimeline(
    [
      event(1, "message.delta", { role: "assistant", delta: "Hel" }, {
        message_id: "answer",
      }),
      event(2, "message.delta", { role: "assistant", delta: "lo" }, {
        message_id: "answer",
      }),
      event(3, "message.reset", { reason: "session paused" }, {
        message_id: "answer",
      }),
      event(4, "message.delta", { role: "assistant", delta: "Replacement" }, {
        message_id: "answer",
      }),
    ],
    [],
  );
  const messages = timeline.filter((item) => item.kind === "message");
  const message = messages[0];
  assert(message?.content === "Hello", "expected deltas to coalesce");
  assert(message?.reset === true, "expected reset output to be marked");
  assert(message?.abandoned === true, "expected reset output to be abandoned");
  assert(messages.length === 2, "expected post-reset output to start a new message");
  assert(
    messages[1]?.content === "Replacement" && messages[1]?.abandoned === false,
    "expected replacement output to remain on the active timeline",
  );
  assert(
    timeline.some((item) => item.title === "Tentative output reset"),
    "expected a visible reset activity",
  );
}

function testActiveAndAbandonedLineage() {
  const cp1 = checkpoint("cp-1", 2);
  const cp2 = checkpoint("cp-2", 4, "cp-1");
  const timeline = buildSessionTimeline(
    [
      event(1, "session.created"),
      event(2, "checkpoint.created", {
        checkpoint_id: "cp-1",
        safe_point: "step.completed",
      }),
      event(3, "message.created", { role: "assistant", content: "old branch" }),
      event(4, "checkpoint.created", {
        checkpoint_id: "cp-2",
        safe_point: "step.completed",
      }),
      event(5, "tool.completed", { tool: "lookup", result: "old result" }),
      event(6, "session.rewound", {
        checkpoint_id: "cp-1",
        checkpoint_sequence: 2,
      }),
      event(7, "message.created", { role: "assistant", content: "active branch" }),
    ],
    [cp2, cp1],
    "cp-1",
  );

  const cp1Item = timeline.find((item) => item.checkpointID === "cp-1");
  const cp2Item = timeline.find((item) => item.checkpointID === "cp-2");
  const oldBranch = timeline.find((item) => item.content === "old branch");
  const activeBranch = timeline.find((item) => item.content === "active branch");
  const tool = timeline.find((item) => item.eventType === "tool.completed");
  assert(cp1Item?.abandoned === false, "expected restored checkpoint to be active");
  assert(cp2Item?.abandoned === true, "expected superseded checkpoint to be abandoned");
  assert(oldBranch?.abandoned === true, "expected rewound events to be abandoned");
  assert(activeBranch?.abandoned === false, "expected post-rewind events to be active");
  assert(tool?.title === "lookup completed", "expected typed tool activity");
}

function testRepeatedRewindsAndForkOrigin() {
  const cp1 = checkpoint("cp-1", 2);
  const cp3 = checkpoint("cp-3", 8, "cp-1");
  const timeline = buildSessionTimeline(
    [
      event(2, "checkpoint.created", { checkpoint_id: "cp-1" }),
      event(6, "session.rewound", {
        checkpoint_id: "cp-1",
        checkpoint_sequence: 2,
      }),
      event(8, "checkpoint.created", { checkpoint_id: "cp-3" }),
      event(9, "message.created", { content: "second abandoned branch" }),
      event(10, "session.rewound", {
        checkpoint_id: "cp-1",
        checkpoint_sequence: 2,
      }),
      event(11, "session.forked", {
        source_session: "source-session",
        source_checkpoint_id: "source-cp",
      }),
    ],
    [cp3, cp1],
    "cp-1",
  );

  assert(
    timeline.find((item) => item.checkpointID === "cp-3")?.abandoned === true,
    "expected a checkpoint superseded by a later rewind to be abandoned",
  );
  const fork = timeline.find((item) => item.eventType === "session.forked");
  assert(
    fork?.content?.includes("source-session") === true,
    "expected fork origin to be visible",
  );
}

function testActiveCheckpointAncestry() {
  const cp1 = checkpoint("cp-1", 2);
  const cp2 = checkpoint("cp-2", 4, "cp-1");
  const cp3 = checkpoint("cp-3", 6, "cp-2");
  const active = activeSessionCheckpointIDs([cp1, cp2, cp3], "cp-3");
  assert(active.size === 3, "expected the complete checkpoint ancestry");
  assert(active.has("cp-1") && active.has("cp-3"), "expected root and head active");
}

function testRewindCanReactivateAnEarlierBranch() {
  const root = checkpoint("root", 2);
  const old = checkpoint("old", 4, "root");
  const timeline = buildSessionTimeline(
    [
      event(2, "checkpoint.created", { checkpoint_id: "root" }),
      event(3, "message.created", { content: "reactivated history" }),
      event(4, "checkpoint.created", { checkpoint_id: "old" }),
      event(5, "message.created", { content: "superseded tail" }),
      event(6, "session.rewound", {
        checkpoint_id: "root",
        checkpoint_sequence: 2,
      }),
      event(7, "message.created", { content: "newer branch" }),
      event(10, "session.rewound", {
        checkpoint_id: "old",
        checkpoint_sequence: 4,
      }),
    ],
    [old, root],
    "old",
  );
  assert(
    timeline.find((item) => item.content === "reactivated history")?.abandoned ===
      false,
    "expected history through the latest restored checkpoint to reactivate",
  );
  assert(
    timeline.find((item) => item.content === "superseded tail")?.abandoned === true,
    "expected the latest superseded tail to remain abandoned",
  );
}

testMessageCoalescingAndReset();
testActiveAndAbandonedLineage();
testRepeatedRewindsAndForkOrigin();
testActiveCheckpointAncestry();
testRewindCanReactivateAnEarlierBranch();
