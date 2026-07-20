import {
  isPermanentSessionStreamStatus,
  mergeSessionEvents,
} from "./sessionStream";
import type { SessionEvent } from "./types";

function assert(condition: boolean, message: string) {
  if (!condition) throw new Error(message);
}

function event(seq: number, type = "message.delta"): SessionEvent {
  return { seq, type, payload: { delta: String(seq) } };
}

function testMergeOrdersAndDeduplicatesBySequence() {
  const merged = mergeSessionEvents(
    [event(2), event(1)],
    [event(3), event(2, "message.completed")],
  );

  assert(
    merged.map((item) => item.seq).join(",") === "1,2,3",
    "expected events to be sorted by sequence",
  );
  assert(
    merged[1]?.type === "message.completed",
    "expected the newest event with a duplicate sequence to win",
  );
}

testMergeOrdersAndDeduplicatesBySequence();

assert(isPermanentSessionStreamStatus(401), "expected auth failures to stop reconnecting");
assert(isPermanentSessionStreamStatus(404), "expected missing sessions to stop reconnecting");
assert(!isPermanentSessionStreamStatus(408), "expected request timeout to reconnect");
assert(!isPermanentSessionStreamStatus(429), "expected rate limiting to reconnect");
assert(!isPermanentSessionStreamStatus(503), "expected server failures to reconnect");
