import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import type { TaskTraceEvent } from "../src/api/types";
import type { SessionTimelineItem } from "../src/utils/sessionTimeline";
import { SessionTimeline } from "../src/components/SessionTimeline";
import { TraceView } from "../src/components/TraceView";

function messageItem(role: string, content: string): SessionTimelineItem {
  return {
    key: `${role}-1`,
    kind: "message",
    role,
    title: role,
    content,
    eventType: `${role}.message`,
    seq: 1,
    startSeq: 1,
    endSeq: 1,
    abandoned: false,
  };
}

describe("Orloj disclosure surface integration", () => {
  test("labels only assistant Session timeline messages", () => {
    const html = renderToStaticMarkup(
      <SessionTimeline
        items={[messageItem("user", "human input"), messageItem("assistant", "agent output")]}
        onSelectCheckpoint={() => {}}
      />,
    );
    expect((html.match(/data-ai-disclosure="generated-output"/g) ?? [])).toHaveLength(1);
    expect(html).toContain("agent output");
    expect(html).toContain("human input");
  });

  test("labels model output trace rows with resolved attribution but not tool events", () => {
    const trace: TaskTraceEvent[] = [
      { timestamp: "2026-08-26T00:00:00Z", type: "model_output", agent: "writer", message: "generated trace output" },
      { timestamp: "2026-08-26T00:00:01Z", type: "tool_call", agent: "writer", tool: "read_file", message: "operational tool event" },
    ];
    const html = renderToStaticMarkup(
      <TraceView trace={trace} resolveAttribution={() => ({ provider: "anthropic", modelId: "claude-opus-4-1" })} />,
    );
    expect((html.match(/data-ai-disclosure="generated-output"/g) ?? [])).toHaveLength(1);
    expect(html).toContain('data-ai-provider="anthropic"');
    expect(html).toContain('data-ai-model="claude-opus-4-1"');
  });
});
