export const AI_TRANSPARENCY_DECISIONS = ["included", "nested", "excluded"] as const;

export type AiTransparencyDecision = (typeof AI_TRANSPARENCY_DECISIONS)[number];

export interface AiTransparencySurface {
  id: string;
  sourceFiles: readonly string[];
  decision: AiTransparencyDecision;
  rationale: string;
  parentId?: string;
  disclosure?: "ai-interaction" | "generated-output" | "ai-assisted-analysis" | "ai-translation";
  providerStrategy: "agent-endpoint" | "provider-agnostic" | "not-applicable";
}

/** Production inventory for Orloj console AI interactions and generated output. */
export const aiTransparencySurfaces = [
  {
    id: "orloj-session-interaction",
    sourceFiles: ["pages/SessionDetail.tsx"],
    decision: "included",
    rationale: "The live Session composer directly sends natural-person steering input to an agent system.",
    disclosure: "ai-interaction",
    providerStrategy: "provider-agnostic",
  },
  {
    id: "orloj-session-assistant-message",
    sourceFiles: ["components/SessionTimeline.tsx"],
    decision: "included",
    rationale: "Session timeline items with assistant role directly render streamed or persisted agent output.",
    disclosure: "generated-output",
    providerStrategy: "provider-agnostic",
  },
  {
    id: "orloj-session-user-system-activity",
    sourceFiles: ["components/SessionTimeline.tsx", "utils/sessionTimeline.ts"],
    decision: "excluded",
    rationale: "User messages, checkpoints, tool activity, lifecycle events, and errors are human or operational content.",
    providerStrategy: "not-applicable",
  },
  {
    id: "orloj-task-overview-output",
    sourceFiles: ["pages/TaskDetail.tsx"],
    decision: "included",
    rationale: "Task status output directly renders aggregate agent-system output.",
    disclosure: "generated-output",
    providerStrategy: "provider-agnostic",
  },
  {
    id: "orloj-task-agent-message",
    sourceFiles: ["pages/TaskDetail.tsx"],
    decision: "included",
    rationale: "Task messages with a non-system producing agent directly render agent-authored content.",
    disclosure: "generated-output",
    providerStrategy: "agent-endpoint",
  },
  {
    id: "orloj-task-system-message",
    sourceFiles: ["pages/TaskDetail.tsx"],
    decision: "excluded",
    rationale: "System/control messages and last_error are operational rather than model output.",
    providerStrategy: "not-applicable",
  },
  {
    id: "orloj-trace-model-output",
    sourceFiles: ["components/TraceView.tsx"],
    decision: "included",
    rationale: "model_output trace events expose generated model content.",
    disclosure: "generated-output",
    providerStrategy: "agent-endpoint",
  },
  {
    id: "orloj-trace-agent-message",
    sourceFiles: ["components/TraceView.tsx"],
    decision: "included",
    rationale: "agent_message trace events expose direct agent-generated message content.",
    disclosure: "generated-output",
    providerStrategy: "agent-endpoint",
  },
  {
    id: "orloj-trace-operational-events",
    sourceFiles: ["components/TraceView.tsx"],
    decision: "excluded",
    rationale: "Tool, error, timing, phase, branch, and token-usage events are operational evidence.",
    providerStrategy: "not-applicable",
  },
  {
    id: "orloj-eval-result-output",
    sourceFiles: ["pages/EvalRunDetail.tsx"],
    decision: "included",
    rationale: "Evaluation sample output directly renders aggregate generated system output.",
    disclosure: "ai-assisted-analysis",
    providerStrategy: "provider-agnostic",
  },
  {
    id: "orloj-task-yaml",
    sourceFiles: ["pages/TaskDetail.tsx"],
    decision: "nested",
    parentId: "orloj-task-overview-output",
    rationale: "The editable YAML inspector repeats mixed task config/input/output already classified at its primary surface.",
    providerStrategy: "not-applicable",
  },
  {
    id: "orloj-task-log-viewer",
    sourceFiles: ["components/LogViewer.tsx", "pages/TaskDetail.tsx"],
    decision: "excluded",
    rationale: "Raw operational logs have mixed per-line provenance and cannot be accurately blanket-labelled.",
    providerStrategy: "not-applicable",
  },
  {
    id: "orloj-eval-dataset-expected-output",
    sourceFiles: ["pages/EvalDatasetDetail.tsx"],
    decision: "excluded",
    rationale: "Expected outputs are human-authored evaluation fixtures, not live model output.",
    providerStrategy: "not-applicable",
  },
  {
    id: "orloj-task-approval-output",
    sourceFiles: ["pages/TaskApprovalDetail.tsx"],
    decision: "nested",
    parentId: "orloj-task-overview-output",
    rationale: "Approval detail repeats held task output for human governance rather than invoking a new model.",
    providerStrategy: "not-applicable",
  },
] as const satisfies readonly AiTransparencySurface[];

export const ORLOJ_AI_CENSUS_PATTERNS = [
  /status\?\.output|status\.output/,
  /msg\.content|message__content/,
  /model_output|agent_message/,
  /results\.map|\.output/,
  /SessionTimeline|sendTurn/,
] as const;

export function validateAiTransparencyRegistry(surfaces: readonly AiTransparencySurface[] = aiTransparencySurfaces): string[] {
  const errors: string[] = [];
  const ids = new Set<string>();
  for (const surface of surfaces) {
    if (!surface.id.trim()) errors.push("surface id is empty");
    if (ids.has(surface.id)) errors.push(`duplicate surface id: ${surface.id}`);
    ids.add(surface.id);
    if (surface.sourceFiles.length === 0) errors.push(`${surface.id}: sourceFiles is empty`);
    if (!surface.rationale.trim()) errors.push(`${surface.id}: rationale is empty`);
    if (surface.decision === "included" && !surface.disclosure) errors.push(`${surface.id}: included surface has no disclosure kind`);
    if (surface.decision === "nested" && !surface.parentId) errors.push(`${surface.id}: nested surface has no parentId`);
  }
  for (const surface of surfaces) {
    if (surface.parentId && !ids.has(surface.parentId)) errors.push(`${surface.id}: unknown parentId ${surface.parentId}`);
    if (surface.parentId === surface.id) errors.push(`${surface.id}: cannot be its own parent`);
  }
  return errors;
}
