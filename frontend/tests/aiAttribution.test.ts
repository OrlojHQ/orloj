import { describe, expect, test } from "bun:test";
import type { Agent, ModelEndpoint } from "../src/api/types";
import { resolveAgentAiAttribution } from "../src/compliance/aiAttribution";

const agent = {
  apiVersion: "orloj.dev/v1",
  kind: "Agent",
  metadata: { name: "writer", namespace: "default" },
  spec: { model_ref: "primary-model", prompt: "synthetic fixture" },
} as Agent;

const endpoint = {
  apiVersion: "orloj.dev/v1",
  kind: "ModelEndpoint",
  metadata: { name: "primary-model", namespace: "default" },
  spec: {
    provider: "anthropic",
    default_model: "claude-opus-4-1",
    base_url: "https://private-provider.example/v1",
    auth: { secretRef: "do-not-expose" },
  },
} as ModelEndpoint;

describe("resolveAgentAiAttribution", () => {
  test("returns only public provider/model fields for one exact agent-endpoint chain", () => {
    const attribution = resolveAgentAiAttribution("writer", [agent], [endpoint]);
    expect(attribution).toEqual({ provider: "anthropic", modelId: "claude-opus-4-1" });
    const serialized = JSON.stringify(attribution);
    expect(serialized).not.toContain("do-not-expose");
    expect(serialized).not.toContain("private-provider.example");
  });

  test("falls back when the agent, endpoint, or mapping is ambiguous", () => {
    expect(resolveAgentAiAttribution("system", [agent], [endpoint])).toBeUndefined();
    expect(resolveAgentAiAttribution("missing", [agent], [endpoint])).toBeUndefined();
    expect(resolveAgentAiAttribution("writer", [agent, agent], [endpoint])).toBeUndefined();
    expect(resolveAgentAiAttribution("writer", [agent], [endpoint, endpoint])).toBeUndefined();
  });
});
