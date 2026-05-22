import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  Handle,
  Position,
  MarkerType,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
} from "@xyflow/react";

const mqMobile = typeof window !== "undefined" ? window.matchMedia("(max-width: 768px)") : null;
function useIsMobile() {
  return useSyncExternalStore(
    (cb) => { mqMobile?.addEventListener("change", cb); return () => mqMobile?.removeEventListener("change", cb); },
    () => mqMobile?.matches ?? false,
  );
}
import "@xyflow/react/dist/style.css";
import Dagre from "@dagrejs/dagre";
import type {
  AgentSystem,
  Agent,
  ModelEndpoint,
  Tool,
  Secret,
  Memory,
  AgentRole,
  Task,
  TaskSchedule,
  TaskWebhook,
  Worker,
  GraphEdge as GraphEdgeDef,
} from "../api/types";
import { StatusBadge } from "./StatusBadge";
import {
  Bot,
  Play,
  GitMerge,
  Network,
  Database,
  Wrench,
  Lock,
  Brain,
  UserCog,
  ListTodo,
  CalendarClock,
  Webhook,
  Cpu,
  CircleDot,
  Layers,
  Map as MapIcon,
  Shield,
} from "lucide-react";
import clsx from "clsx";

// ---------------------------------------------------------------------------
// Graph helpers
// ---------------------------------------------------------------------------

function getOutgoing(edge: GraphEdgeDef): string[] {
  const targets: string[] = [];
  const seen = new Set<string>();
  if (edge.next) {
    targets.push(edge.next);
    seen.add(edge.next.toLowerCase());
  }
  for (const r of edge.edges ?? []) {
    if (r.to && !seen.has(r.to.toLowerCase())) {
      targets.push(r.to);
      seen.add(r.to.toLowerCase());
    }
  }
  return targets;
}

function computeEntryPoints(agents: string[], graph: Record<string, GraphEdgeDef>): Set<string> {
  const allTargets = new Set<string>();
  for (const [, edge] of Object.entries(graph)) {
    for (const t of getOutgoing(edge)) allTargets.add(t.toLowerCase());
  }
  return new Set(agents.filter((a) => !allTargets.has(a.toLowerCase())));
}

function computeIncoming(agents: string[], graph: Record<string, GraphEdgeDef>): Record<string, string[]> {
  const incoming: Record<string, string[]> = {};
  for (const a of agents) incoming[a] = [];
  for (const [src, edge] of Object.entries(graph)) {
    for (const t of getOutgoing(edge)) {
      if (incoming[t]) incoming[t].push(src);
    }
  }
  return incoming;
}

function computeTerminals(agents: string[], graph: Record<string, GraphEdgeDef>): Set<string> {
  const hasOut = new Set<string>();
  for (const [src, edge] of Object.entries(graph)) {
    if (getOutgoing(edge).length > 0) hasOut.add(src);
  }
  return new Set(agents.filter((a) => !hasOut.has(a)));
}

function topoSortAgents(agents: string[], graph: Record<string, GraphEdgeDef>): string[] {
  const indegree: Record<string, number> = {};
  for (const a of agents) indegree[a] = 0;
  for (const [, edge] of Object.entries(graph)) {
    for (const t of getOutgoing(edge)) indegree[t] = (indegree[t] ?? 0) + 1;
  }
  const queue = agents.filter((a) => indegree[a] === 0);
  const result: string[] = [];
  while (queue.length) {
    const n = queue.shift()!;
    result.push(n);
    if (graph[n]) {
      for (const t of getOutgoing(graph[n])) {
        indegree[t]--;
        if (indegree[t] === 0) queue.push(t);
      }
    }
  }
  for (const a of agents) {
    if (!result.includes(a)) result.push(a);
  }
  return result;
}

function detectBlueprint(agents: string[], graph: Record<string, GraphEdgeDef>): string {
  for (const [src, edge] of Object.entries(graph)) {
    for (const t of getOutgoing(edge)) {
      if (graph[t]) {
        for (const t2 of getOutgoing(graph[t])) {
          if (t2 === src) return "swarm-loop";
        }
      }
    }
  }
  const maxFan = Math.max(...Object.values(graph).map((e) => getOutgoing(e).length), 0);
  const entries = computeEntryPoints(agents, graph);
  if (maxFan <= 1 && entries.size <= 1) return "pipeline";
  if (maxFan > 1) return "hierarchical";
  return "pipeline";
}

const BLUEPRINT_META: Record<string, { label: string; color: string }> = {
  pipeline: { label: "Pipeline", color: "var(--blue)" },
  hierarchical: { label: "Hierarchical", color: "var(--purple)" },
  "swarm-loop": { label: "Swarm Loop", color: "var(--orange)" },
};

// Config-type resources that default to "Pending" but are effectively ready
// if they exist -- no active controller manages their phase.
const CONFIG_KINDS = new Set(["model", "tool", "secret", "memory", "role"]);

function effectivePhase(kind: string, rawPhase?: string): string | undefined {
  if (!rawPhase) return undefined;
  if (CONFIG_KINDS.has(kind) && rawPhase === "Pending") return "Ready";
  return rawPhase;
}

/** Schedule/webhook controllers stamp these labels on each spawned run; hide those from the graph so high-frequency triggers do not overwhelm the layout. */
function isAutomationSpawnedRunTask(task: Task): boolean {
  const labels = task.metadata.labels ?? {};
  return Boolean(labels["orloj.dev/task-schedule"] || labels["orloj.dev/task-webhook"]);
}

// Task phases treated as "live" (collapse representative prefers these).
const ACTIVE_TASK_PHASES = new Set(["pending", "running", "waitingapproval"]);
// Task phases treated as "done" (fallback representative when no active run exists).
const TERMINAL_TASK_PHASES = new Set(["succeeded", "failed", "deadletter"]);

/** Reruns carry an `orloj.dev/source-task` label pointing at the original task name; treat all instances sharing that source (plus the source itself) as one lineage. */
function taskLineageKey(task: Task): string {
  return task.metadata.labels?.["orloj.dev/source-task"] || task.metadata.name;
}

/** Pick a single node to represent a task lineage: any active instance wins; otherwise the most recent terminal instance by creationTimestamp. Null if neither exists. */
function pickRepresentativeTask(tasks: Task[]): Task | null {
  const active = tasks.find((t) => ACTIVE_TASK_PHASES.has((t.status?.phase ?? "").toLowerCase()));
  if (active) return active;
  const terminal = tasks
    .filter((t) => TERMINAL_TASK_PHASES.has((t.status?.phase ?? "").toLowerCase()))
    .sort((a, b) => (b.metadata.createdAt ?? "").localeCompare(a.metadata.createdAt ?? ""));
  return terminal[0] ?? null;
}

// ---------------------------------------------------------------------------
// Node kind config
// ---------------------------------------------------------------------------

type NodeKind = "system" | "agent" | "model" | "tool" | "secret" | "memory" | "role" | "task" | "schedule" | "webhook" | "worker" | "adapter";

const KIND_CONFIG: Record<NodeKind, { icon: React.ReactNode; colorVar: string }> = {
  system:  { icon: <Network   size={14} />, colorVar: "var(--accent)" },
  adapter: { icon: <Shield     size={14} />, colorVar: "var(--yellow)" },
  agent:   { icon: <Bot        size={14} />, colorVar: "var(--green)" },
  model:   { icon: <Database   size={14} />, colorVar: "var(--accent-blue)" },
  tool:    { icon: <Wrench     size={14} />, colorVar: "var(--yellow)" },
  secret:  { icon: <Lock       size={14} />, colorVar: "var(--orange)" },
  memory:  { icon: <Brain      size={14} />, colorVar: "var(--purple)" },
  role:    { icon: <UserCog   size={14} />, colorVar: "var(--purple)" },
  task:    { icon: <ListTodo   size={14} />, colorVar: "var(--accent-blue)" },
  schedule:{ icon: <CalendarClock size={14} />, colorVar: "var(--orange)" },
  webhook: { icon: <Webhook    size={14} />, colorVar: "var(--orange)" },
  worker:  { icon: <Cpu        size={14} />, colorVar: "var(--green)" },
};

// ---------------------------------------------------------------------------
// Custom node
// ---------------------------------------------------------------------------

interface CrdNodeData {
  label: string;
  kind: NodeKind;
  phase?: string;
  subtitle?: string;
  isEntry?: boolean;
  isTerminal?: boolean;
  hasJoin?: boolean;
  joinMode?: string;
  depCounts?: { tools: number; models: number; memories: number; roles: number };
  [key: string]: unknown;
}

function ResourceNode({ data }: { data: CrdNodeData }) {
  const cfg = KIND_CONFIG[data.kind];
  const isAgent = data.kind === "agent";
  const isRunning = data.phase?.toLowerCase() === "running";
  const isSecondary = SECONDARY_KINDS.has(data.kind);
  const iconSize = isSecondary ? 12 : 14;

  return (
    <div
      className={clsx(
        "gnode",
        `gnode--${data.kind}`,
        isSecondary && "gnode--secondary",
        isRunning && "gnode--running",
        isAgent && data.isEntry && "gnode--entry",
        isAgent && data.isTerminal && "gnode--terminal",
        isAgent && data.hasJoin && "gnode--join",
      )}
    >
      <Handle id="left" type="target" position={Position.Left} className="gnode__handle" />
      <Handle id="top" type="target" position={Position.Top} className="gnode__handle" />
      <div className="gnode__icon-ring">
        {isAgent && data.isEntry ? <Play size={iconSize} /> : isAgent && data.hasJoin ? <GitMerge size={iconSize} /> : cfg.icon}
        {isRunning && <span className="gnode__pulse" />}
      </div>
      <div className="gnode__body">
        <div className="gnode__name">{data.label}</div>
        <div className="gnode__meta">
          {data.phase ? <StatusBadge phase={data.phase} pulse={isRunning} /> : data.subtitle ? <span className="gnode__pos">{data.subtitle}</span> : <span className="gnode__pos">{data.kind}</span>}
          {data.hasJoin && <span className="gnode__join-tag">{data.joinMode ?? "wait_for_all"}</span>}
        </div>
        {isAgent && data.depCounts && (
          <div className="gnode__deps">
            {data.depCounts.models > 0 && <span className="gnode__dep-tag">model</span>}
            {data.depCounts.tools > 0 && <span className="gnode__dep-tag">{data.depCounts.tools} tool{data.depCounts.tools > 1 ? "s" : ""}</span>}
            {data.depCounts.memories > 0 && <span className="gnode__dep-tag">memory</span>}
            {data.depCounts.roles > 0 && <span className="gnode__dep-tag">{data.depCounts.roles} role{data.depCounts.roles > 1 ? "s" : ""}</span>}
          </div>
        )}
      </div>
      <Handle id="right" type="source" position={Position.Right} className="gnode__handle" />
      <Handle id="bottom" type="source" position={Position.Bottom} className="gnode__handle" />
    </div>
  );
}

const nodeTypes = { resource: ResourceNode };

// ---------------------------------------------------------------------------
// Layout: Argo-style left-to-right tree with all nodes in dagre
// ---------------------------------------------------------------------------
//
// Strategy:
//   1. ALL nodes go through dagre with rankdir "LR" for clean auto-layout
//   2. System is the root; agents are its children (pipeline edges)
//   3. Shared resources (e.g. model used by multiple agents) get edges from
//      EVERY agent that references them, so dagre positions them naturally
//   4. Model endpoints have child secret nodes via auth.secretRef
//   5. Tasks connect to system; workers connect to tasks

const NODE_W = 220;
const NODE_H = 58;
const NODE_W_SM = 180;
const NODE_H_SM = 46;
const SECONDARY_KINDS = new Set<NodeKind>(["model", "tool", "secret", "memory", "role"]);

interface RelatedResources {
  agents?: Agent[];
  modelEndpoints?: ModelEndpoint[];
  tools?: Tool[];
  secrets?: Secret[];
  memories?: Memory[];
  roles?: AgentRole[];
  tasks?: Task[];
  taskSchedules?: TaskSchedule[];
  taskWebhooks?: TaskWebhook[];
  workers?: Worker[];
}

function nid(kind: string, name: string) {
  return `${kind}:${name}`;
}

function resolveTaskRef(taskRef?: string, defaultNamespace?: string): { namespace: string; name: string } | null {
  if (!taskRef) return null;
  const ref = taskRef.trim();
  if (!ref) return null;
  const slash = ref.indexOf("/");
  if (slash > 0 && slash < ref.length - 1) {
    return { namespace: ref.slice(0, slash), name: ref.slice(slash + 1) };
  }
  return { namespace: defaultNamespace ?? "default", name: ref };
}

function buildTree(
  system: AgentSystem,
  related: RelatedResources,
  animated: boolean,
  runningAgents?: Set<string>,
) {
  const agentNames = system.spec.agents ?? [];
  const graphDef = system.spec.graph ?? {};
  const entries = computeEntryPoints(agentNames, graphDef);
  const terminals = computeTerminals(agentNames, graphDef);
  const incoming = computeIncoming(agentNames, graphDef);
  const ordered = topoSortAgents(agentNames, graphDef);

  const agentMap = new Map((related.agents ?? []).map((a) => [a.metadata.name, a]));
  const modelMap = new Map((related.modelEndpoints ?? []).map((m) => [m.metadata.name, m]));
  const toolMap = new Map((related.tools ?? []).map((t) => [t.metadata.name, t]));
  const secretMap = new Map((related.secrets ?? []).map((s) => [s.metadata.name, s]));
  const memMap = new Map((related.memories ?? []).map((m) => [m.metadata.name, m]));
  const roleMap = new Map((related.roles ?? []).map((r) => [r.metadata.name, r]));

  // -- Dagre graph: TB layout, all nodes included -----------------------------

  const g = new Dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: "LR", nodesep: 120, ranksep: 200, marginx: 60, marginy: 60 });

  const sysId = nid("system", system.metadata.name);
  g.setNode(sysId, { width: NODE_W + 20, height: NODE_H + 8 });

  // Track node metadata for React Flow construction
  interface NodeMeta { kind: NodeKind; label: string; phase?: string; subtitle?: string; extra?: Partial<CrdNodeData> }
  const nodeMeta: Record<string, NodeMeta> = {};
  const edgeList: { src: string; tgt: string; routing: boolean; agentToAgent: boolean }[] = [];
  const registeredNodes = new Set<string>();

  // Symmetric map of semantic neighbours that can bypass the rendered graph edges.
  // Used for hover highlighting so that a shared model (edged to the system) or a
  // transitively-used secret (edged to its tool/model) still light up when you hover
  // the agent that references them.
  const semanticNeighbours = new Map<string, Set<string>>();
  function linkSemantic(a: string, b: string) {
    if (a === b) return;
    if (!semanticNeighbours.has(a)) semanticNeighbours.set(a, new Set());
    if (!semanticNeighbours.has(b)) semanticNeighbours.set(b, new Set());
    semanticNeighbours.get(a)!.add(b);
    semanticNeighbours.get(b)!.add(a);
  }

  function regNode(id: string, kind: NodeKind, label: string, phase?: string, subtitle?: string, extra?: Partial<CrdNodeData>) {
    if (registeredNodes.has(id)) return;
    registeredNodes.add(id);
    const small = SECONDARY_KINDS.has(kind);
    const baseW = small ? NODE_W_SM : NODE_W;
    const textW = Math.ceil(label.length * 7.2) + (small ? 56 : 64);
    const w = Math.min(Math.max(baseW, textW), 340);
    g.setNode(id, { width: w, height: small ? NODE_H_SM : NODE_H });
    nodeMeta[id] = { kind, label, phase, subtitle, extra };
  }

  function regEdge(src: string, tgt: string, routing: boolean, agentToAgent = false) {
    g.setEdge(src, tgt);
    edgeList.push({ src, tgt, routing, agentToAgent });
  }

  // System node
  nodeMeta[sysId] = { kind: "system", label: system.metadata.name, phase: system.status?.phase, subtitle: `${agentNames.length} agents` };
  registeredNodes.add(sysId);

  // Agent nodes -- override phase to "Running" when task messages indicate activity
  for (const aName of ordered) {
    const aid = nid("agent", aName);
    const agent = agentMap.get(aName);
    const hasJoin = (incoming[aName]?.length ?? 0) > 1;
    const agentPhase = runningAgents?.has(aName) ? "Running" : agent?.status?.phase;
    regNode(aid, "agent", aName, agentPhase, agent?.spec.model_ref, {
      isEntry: entries.has(aName),
      isTerminal: terminals.has(aName),
      hasJoin,
      joinMode: hasJoin ? (graphDef[aName]?.join?.mode ?? "wait_for_all") : undefined,
    });
  }

  // Context adapter node: sits between system and entry agents
  const adapterRef = system.spec.context_adapter?.trim();
  const adapterId = adapterRef ? nid("adapter", adapterRef) : null;
  if (adapterRef && adapterId) {
    regNode(adapterId, "adapter", adapterRef, "Ready", "context adapter");
    regEdge(sysId, adapterId, true);
  }

  // System (or adapter) → entry agents
  const entrySource = adapterId ?? sysId;
  if (entries.size > 0) {
    for (const aName of ordered) {
      if (entries.has(aName)) regEdge(entrySource, nid("agent", aName), true);
    }
  } else if (ordered.length > 0) {
    regEdge(entrySource, nid("agent", ordered[0]), true);
  }

  // Agent → Agent routing edges (pipeline)
  for (const [src, edge] of Object.entries(graphDef)) {
    for (const target of getOutgoing(edge)) {
      regEdge(nid("agent", src), nid("agent", target), true, true);
    }
  }

  // -- Count how many agents reference each dep to detect shared resources -----

  const refCount: Record<string, number> = {};
  function incRef(id: string) { refCount[id] = (refCount[id] ?? 0) + 1; }

  for (const aName of ordered) {
    const agent = agentMap.get(aName);
    if (!agent) continue;
    if (agent.spec.model_ref && modelMap.has(agent.spec.model_ref)) incRef(nid("model", agent.spec.model_ref));
    for (const tName of agent.spec.tools ?? []) { if (toolMap.has(tName)) incRef(nid("tool", tName)); }
    for (const rName of agent.spec.roles ?? []) { if (roleMap.has(rName)) incRef(nid("role", rName)); }
    if (agent.spec.memory?.ref && memMap.has(agent.spec.memory.ref)) incRef(nid("memory", agent.spec.memory.ref));
  }

  const isShared = (id: string) => (refCount[id] ?? 0) > 1;

  // -- Agent dependency nodes: model, tool, secret, role, memory ---------------
  // Shared resources (used by 2+ agents) connect to system as peers.
  // Unique resources connect to their owning agent.

  const addedSecretForModel = new Set<string>();

  for (const aName of ordered) {
    const agent = agentMap.get(aName);
    if (!agent) continue;
    const aid = nid("agent", aName);

    // Model endpoint
    const modelRef = agent.spec.model_ref;
    if (modelRef && modelMap.has(modelRef)) {
      const me = modelMap.get(modelRef)!;
      const mid = nid("model", modelRef);
      regNode(mid, "model", modelRef, effectivePhase("model", me.status?.phase), me.spec.provider ?? me.spec.default_model);
      // Shared → connect to system; unique → connect to agent
      if (isShared(mid)) {
        if (!g.hasEdge(sysId, mid)) regEdge(sysId, mid, false);
      } else {
        regEdge(aid, mid, false);
      }
      linkSemantic(aid, mid);

      // Secret under model endpoint (via auth.secretRef)
      const secRef = me.spec.auth?.secretRef;
      if (secRef && secretMap.has(secRef)) {
        const sid = nid("secret", secRef);
        if (!addedSecretForModel.has(mid)) {
          addedSecretForModel.add(mid);
          const sec = secretMap.get(secRef)!;
          const keyCount = Object.keys(sec.spec.data ?? sec.spec.stringData ?? {}).length;
          regNode(sid, "secret", secRef, effectivePhase("secret", sec.status?.phase), `${keyCount} key${keyCount !== 1 ? "s" : ""}`);
          regEdge(mid, sid, false);
        }
        linkSemantic(aid, sid);
      }
    }

    // Tools
    for (const tName of agent.spec.tools ?? []) {
      if (!toolMap.has(tName)) continue;
      const tool = toolMap.get(tName)!;
      const tid = nid("tool", tName);
      regNode(tid, "tool", tName, effectivePhase("tool", tool.status?.phase), `${tool.spec.type ?? "http"} · ${tool.spec.risk_level ?? "low"}`);
      if (isShared(tid)) {
        if (!g.hasEdge(sysId, tid)) regEdge(sysId, tid, false);
      } else {
        regEdge(aid, tid, false);
      }
      linkSemantic(aid, tid);

      // Secret under tool
      const secRef = tool.spec.auth?.secretRef;
      if (secRef && secretMap.has(secRef)) {
        const sec = secretMap.get(secRef)!;
        const keyCount = Object.keys(sec.spec.data ?? sec.spec.stringData ?? {}).length;
        const sid = nid("secret", secRef);
        regNode(sid, "secret", secRef, effectivePhase("secret", sec.status?.phase), `${keyCount} key${keyCount !== 1 ? "s" : ""}`);
        regEdge(tid, sid, false);
        linkSemantic(aid, sid);
      }
    }

    // Roles
    for (const rName of agent.spec.roles ?? []) {
      if (!roleMap.has(rName)) continue;
      const role = roleMap.get(rName)!;
      const rid = nid("role", rName);
      regNode(rid, "role", rName, effectivePhase("role", role.status?.phase), role.spec.description);
      if (isShared(rid)) {
        if (!g.hasEdge(sysId, rid)) regEdge(sysId, rid, false);
      } else {
        regEdge(aid, rid, false);
      }
      linkSemantic(aid, rid);
    }

    // Memory
    const memRef = agent.spec.memory?.ref;
    if (memRef && memMap.has(memRef)) {
      const mem = memMap.get(memRef)!;
      const memId = nid("memory", memRef);
      regNode(memId, "memory", memRef, effectivePhase("memory", mem.status?.phase), mem.spec.type ?? mem.spec.provider);
      if (isShared(memId)) {
        if (!g.hasEdge(sysId, memId)) regEdge(sysId, memId, false);
      } else {
        regEdge(aid, memId, false);
      }
      linkSemantic(aid, memId);
    }
  }

  // -- Compute dependency counts per agent for inline badges -------------------

  const agentDepCounts: Record<string, { tools: number; models: number; memories: number; roles: number }> = {};
  for (const aName of ordered) {
    const agent = agentMap.get(aName);
    if (!agent) continue;
    const counts = { tools: 0, models: 0, memories: 0, roles: 0 };
    if (agent.spec.model_ref && modelMap.has(agent.spec.model_ref)) counts.models++;
    counts.tools = (agent.spec.tools ?? []).filter((t) => toolMap.has(t)).length;
    counts.roles = (agent.spec.roles ?? []).filter((r) => roleMap.has(r)).length;
    if (agent.spec.memory?.ref && memMap.has(agent.spec.memory.ref)) counts.memories++;
    agentDepCounts[aName] = counts;
  }

  // -- Tasks and workers connected to system -----------------------------------

  // Tasks for this system, minus high-frequency schedule/webhook spawns.
  const candidateTasks = (related.tasks ?? [])
    .filter((t) => t.spec.system === system.metadata.name)
    .filter((t) => !isAutomationSpawnedRunTask(t));

  // Templates are definitional and must always render (schedules/webhooks edge into them).
  const templateTasks = candidateTasks.filter((t) => t.spec.mode === "template");

  // Runnable tasks collapse by lineage to one representative: active run if any, else latest terminal.
  const tasksByLineage = new Map<string, Task[]>();
  for (const t of candidateTasks) {
    if (t.spec.mode === "template") continue;
    const key = taskLineageKey(t);
    const list = tasksByLineage.get(key) ?? [];
    list.push(t);
    tasksByLineage.set(key, list);
  }
  const representativeTasks: Task[] = [];
  for (const group of tasksByLineage.values()) {
    const rep = pickRepresentativeTask(group);
    if (rep) representativeTasks.push(rep);
  }

  const systemTasks = [...templateTasks, ...representativeTasks];
  for (const task of systemTasks) {
    const tid = nid("task", task.metadata.name);
    regNode(tid, "task", task.metadata.name, task.status?.phase, task.spec.priority ?? "normal");
    regEdge(sysId, tid, false);

    const wName = task.status?.assignedWorker;
    if (wName) {
      const worker = (related.workers ?? []).find((w) => w.metadata.name === wName);
      const wid = nid("worker", wName);
      regNode(wid, "worker", wName, worker?.status?.phase, worker?.spec.region);
      regEdge(tid, wid, false);
    }
  }

  // -- Task schedules connected to template task --------------------------------

  const systemNamespace = system.metadata.namespace ?? "default";
  const taskMap = new Map<string, Task>();
  for (const task of related.tasks ?? []) {
    const ns = task.metadata.namespace ?? systemNamespace;
    taskMap.set(`${ns}/${task.metadata.name}`, task);
  }

  for (const schedule of related.taskSchedules ?? []) {
    const scheduleNamespace = schedule.metadata.namespace ?? systemNamespace;
    const ref = resolveTaskRef(schedule.spec.task_ref, scheduleNamespace);
    if (!ref) continue;

    const template = taskMap.get(`${ref.namespace}/${ref.name}`);
    if (!template || template.spec.system !== system.metadata.name) continue;

    const scheduleId = nid("schedule", schedule.metadata.name);
    const templateTaskId = nid("task", template.metadata.name);

    regNode(
      scheduleId,
      "schedule",
      schedule.metadata.name,
      schedule.status?.phase,
      `${schedule.spec.schedule ?? ""} ${schedule.spec.time_zone ? `(${schedule.spec.time_zone})` : ""}`.trim(),
    );
    regEdge(scheduleId, templateTaskId, false);
  }

  for (const webhook of related.taskWebhooks ?? []) {
    const webhookNamespace = webhook.metadata.namespace ?? systemNamespace;
    const ref = resolveTaskRef(webhook.spec.task_ref, webhookNamespace);

    if (ref) {
      const template = taskMap.get(`${ref.namespace}/${ref.name}`);
      if (!template || template.spec.system !== system.metadata.name) continue;

      const webhookId = nid("webhook", webhook.metadata.name);
      const templateTaskId = nid("task", template.metadata.name);
      regNode(
        webhookId,
        "webhook",
        webhook.metadata.name,
        webhook.status?.phase,
        webhook.status?.endpointID ?? webhook.spec.auth?.profile ?? "generic",
      );
      regEdge(webhookId, templateTaskId, false);
    } else if (webhook.spec.task_template) {
      if (webhook.spec.task_template.system !== system.metadata.name) continue;

      const webhookId = nid("webhook", webhook.metadata.name);
      regNode(
        webhookId,
        "webhook",
        webhook.metadata.name,
        webhook.status?.phase,
        webhook.status?.endpointID ?? webhook.spec.auth?.profile ?? "generic",
      );

      // Create a synthetic task node for the inline template so the graph
      // shows webhook → task → worker (same visual as the task_ref path)
      // rather than an orphan webhook hanging off the system node.
      // Edge direction matches normal tasks: system → task, webhook → task.
      const inlineTaskId = nid("task", `${webhook.metadata.name}-inline`);
      const tmpl = webhook.spec.task_template;
      regNode(inlineTaskId, "task", `${webhook.metadata.name} (inline)`, undefined, tmpl.priority ?? "normal");
      regEdge(webhookId, inlineTaskId, false);
      regEdge(sysId, inlineTaskId, false);
    }
  }

  // -- Run dagre layout -------------------------------------------------------

  Dagre.layout(g);

  // -- Build React Flow nodes and edges from dagre results ---------------------

  const nodes: Node<CrdNodeData>[] = [];
  const edges: Edge[] = [];
  const nodePos: Record<string, { x: number; y: number }> = {};

  for (const id of registeredNodes) {
    const pos = g.node(id);
    const meta = nodeMeta[id];
    const w = pos.width as number;
    const h = pos.height as number;
    nodePos[id] = { x: pos.x, y: pos.y };
    const data: CrdNodeData = { label: meta.label, kind: meta.kind, phase: meta.phase, subtitle: meta.subtitle, ...meta.extra };
    if (meta.kind === "agent" && agentDepCounts[meta.label]) {
      data.depCounts = agentDepCounts[meta.label];
    }
    nodes.push({
      id,
      type: "resource",
      position: { x: pos.x - w / 2, y: pos.y - h / 2 },
      data,
    });
  }

  const addedEdges = new Set<string>();
  for (const { src, tgt, routing, agentToAgent } of edgeList) {
    const eid = `e-${src}-${tgt}`;
    if (addedEdges.has(eid)) continue;
    addedEdges.add(eid);

    // For agent-to-agent edges, use different handle pairs for forward vs back
    // so both directions are visually distinct (not overlapping).
    // Forward (target is to the right): right → left (straight across)
    // Back (target is to the left): bottom → top (curves underneath)
    let sourceHandle: string | undefined;
    let targetHandle: string | undefined;
    if (agentToAgent && nodePos[src] && nodePos[tgt]) {
      const dx = nodePos[tgt].x - nodePos[src].x;
      if (dx >= 0) {
        sourceHandle = "right";
        targetHandle = "left";
      } else {
        sourceHandle = "bottom";
        targetHandle = "top";
      }
    }

    // Per-edge animation: animate only routing edges that land on a currently-
    // running agent. When the caller doesn't pass `runningAgents` (e.g. task
    // detail, which only knows the task's phase), fall back to animating every
    // routing edge so the pipeline still reads as "live".
    let edgeAnimated = routing && animated;
    if (edgeAnimated && routing && runningAgents) {
      const tgtAgent = tgt.startsWith("agent:") ? tgt.slice("agent:".length) : null;
      edgeAnimated = tgtAgent ? runningAgents.has(tgtAgent) : false;
    }

    const strokeColor = edgeAnimated ? "var(--accent)" : "var(--edge-stroke)";
    const arrowColor = edgeAnimated ? "var(--accent)" : "var(--edge-stroke)";

    edges.push({
      id: eid,
      source: src,
      target: tgt,
      sourceHandle,
      targetHandle,
      animated: edgeAnimated,
      className: routing ? "edge--routing" : "edge--dep",
      type: "default",
      markerEnd: { type: MarkerType.ArrowClosed, width: routing ? 12 : 10, height: routing ? 12 : 10, color: arrowColor },
      style: {
        stroke: strokeColor,
        strokeWidth: routing ? 2.5 : 1.5,
        strokeDasharray: routing ? undefined : "3 4",
        // Dep edges: single-source opacity via CSS (.edge--dep { stroke-opacity }).
        // Routing edges keep a slight inline opacity for a softer line at rest.
        ...(routing ? { opacity: 0.75 } : {}),
      },
    });
  }

  return { nodes, edges, semanticNeighbours };
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

interface GraphViewProps {
  system: AgentSystem;
  related?: RelatedResources;
  onNodeClick?: (kind: string, name: string) => void;
  animated?: boolean;
  runningAgents?: Set<string>;
}

export function GraphView({ system, related, onNodeClick, animated, runningAgents }: GraphViewProps) {
  const isMobile = useIsMobile();
  const [showLegend, setShowLegend] = useState(false);
  const [showMinimap, setShowMinimap] = useState(true);
  const [focusedNode, setFocusedNode] = useState<string | null>(null);
  const agentNames = system.spec.agents ?? [];
  const graph = system.spec.graph ?? {};
  const blueprint = useMemo(() => detectBlueprint(agentNames, graph), [agentNames, graph]);
  const bpInfo = BLUEPRINT_META[blueprint];

  // Serialize runningAgents to a stable string so useMemo doesn't refire on identical Sets
  const runningKey = useMemo(() => runningAgents ? [...runningAgents].sort().join(",") : "", [runningAgents]);

  const { builtNodes, builtEdges, semanticNeighbours } = useMemo(() => {
    const r = buildTree(system, related ?? {}, animated ?? false, runningAgents);
    return { builtNodes: r.nodes, builtEdges: r.edges, semanticNeighbours: r.semanticNeighbours };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [system, related, animated, runningKey]);

  const [nodes, setNodes, onNodesChange] = useNodesState(builtNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(builtEdges);

  useEffect(() => { setNodes(builtNodes); }, [builtNodes, setNodes]);
  useEffect(() => { setEdges(builtEdges); }, [builtEdges, setEdges]);

  // ---------------------------------------------------------------------------
  // Hover-based sub-tree highlighting
  // When hovering a node, light up its direct neighbours + their edges and dim
  // everything else.  This makes resource ownership (which model / tools /
  // secrets belong to which agent) immediately obvious.
  // ---------------------------------------------------------------------------

  // Nodes related to the currently-hovered node: direct graph neighbours plus
  // semantic neighbours (dependencies that route through the system node or
  // through intermediate resources like a tool's secret).
  const focusRelated = useMemo(() => {
    if (!focusedNode) return null;
    const relatedIds = new Set<string>([focusedNode]);
    for (const e of edges) {
      if (e.source === focusedNode) relatedIds.add(e.target);
      if (e.target === focusedNode) relatedIds.add(e.source);
    }
    const sem = semanticNeighbours?.get(focusedNode);
    if (sem) for (const id of sem) relatedIds.add(id);
    return relatedIds;
  }, [edges, focusedNode, semanticNeighbours]);

  const displayNodes = useMemo(() => {
    if (!focusRelated) return nodes;
    return nodes.map((n) => ({
      ...n,
      className: focusRelated.has(n.id) ? "gnode-focus--active" : "gnode-focus--dimmed",
    }));
  }, [nodes, focusRelated]);

  const displayEdges = useMemo(() => {
    if (!focusedNode || !focusRelated) return edges;
    return edges.map((e) => {
      // Touches the hovered node directly, or connects two nodes in the related
      // set (so system↔shared-model lights up when hovering an agent that uses it).
      const isRelated =
        e.source === focusedNode ||
        e.target === focusedNode ||
        (focusRelated.has(e.source) && focusRelated.has(e.target));
      return {
        ...e,
        className: `${e.className ?? ""} ${isRelated ? "edge-focus--active" : "edge-focus--dimmed"}`.trim(),
        style: {
          ...e.style,
          opacity: isRelated ? 1 : 0.06,
          strokeWidth: isRelated ? 2 : e.style?.strokeWidth,
        },
      };
    });
  }, [edges, focusedNode, focusRelated]);

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      const sep = node.id.indexOf(":");
      if (sep > 0) onNodeClick?.(node.id.slice(0, sep), node.id.slice(sep + 1));
    },
    [onNodeClick],
  );

  const handleNodeMouseEnter = useCallback((_: React.MouseEvent, node: Node) => {
    setFocusedNode(node.id);
  }, []);

  const handleNodeMouseLeave = useCallback(() => {
    setFocusedNode(null);
  }, []);

  // Status summary counts
  const statusCounts = useMemo(() => {
    const counts = { running: 0, ready: 0, failed: 0, total: agentNames.length };
    const agentList = related?.agents ?? [];
    for (const aName of agentNames) {
      const phase = runningAgents?.has(aName) ? "running" : agentList.find((a) => a.metadata.name === aName)?.status?.phase?.toLowerCase();
      if (phase === "running") counts.running++;
      else if (phase === "ready" || phase === "healthy") counts.ready++;
      else if (phase === "failed" || phase === "error") counts.failed++;
    }
    return counts;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentNames, related?.agents, runningKey]);

  // Minimap node color by status
  const minimapNodeColor = useCallback(
    (node: Node) => {
      const d = node.data as CrdNodeData | undefined;
      if (!d?.phase) return "var(--edge-stroke)";
      const p = d.phase.toLowerCase();
      if (p === "running") return "var(--accent)";
      if (p === "failed" || p === "error") return "var(--red)";
      if (p === "ready" || p === "healthy" || p === "succeeded") return "var(--green)";
      return "var(--edge-stroke)";
    },
    [],
  );

  const legendKinds: NodeKind[] = ["system", "adapter", "agent", "model", "tool", "secret", "memory", "role", "task", "schedule", "webhook", "worker"];

  return (
    <div className="graph-view">
      <div className="graph-view__topbar">
        <div className="graph-view__badge" style={{ color: bpInfo.color }}>{bpInfo.label}</div>
        <div className="graph-view__status-bar">
          {statusCounts.ready > 0 && (
            <span className="graph-view__status-item">
              <span className="graph-view__status-dot" style={{ background: "var(--green)" }} />
              {statusCounts.ready} ready
            </span>
          )}
          {statusCounts.running > 0 && (
            <span className="graph-view__status-item">
              <span className="graph-view__status-dot" style={{ background: "var(--blue)" }} />
              {statusCounts.running} running
            </span>
          )}
          {statusCounts.failed > 0 && (
            <span className="graph-view__status-item">
              <span className="graph-view__status-dot" style={{ background: "var(--red)" }} />
              {statusCounts.failed} failed
            </span>
          )}
          <span className="graph-view__status-total">{statusCounts.total} agents</span>
        </div>
        {!isMobile && (
          <button
            className={clsx("graph-view__legend-toggle", showMinimap && "graph-view__legend-toggle--active")}
            onClick={() => setShowMinimap(!showMinimap)}
            title="Toggle minimap"
            aria-pressed={showMinimap}
          >
            <MapIcon size={14} />
          </button>
        )}
        <button
          className={clsx("graph-view__legend-toggle", showLegend && "graph-view__legend-toggle--active")}
          onClick={() => setShowLegend(!showLegend)}
          title="Toggle legend"
          aria-pressed={showLegend}
        >
          <Layers size={14} />
        </button>
      </div>
      {showLegend && (
        <div className="graph-view__legend">
          {legendKinds.map((k) => (
            <span key={k} className="graph-view__legend-item">
              <CircleDot size={8} style={{ color: KIND_CONFIG[k].colorVar }} />
              <span>{k}</span>
            </span>
          ))}
        </div>
      )}
      <ReactFlow
        nodes={displayNodes}
        edges={displayEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={handleNodeClick}
        onNodeMouseEnter={handleNodeMouseEnter}
        onNodeMouseLeave={handleNodeMouseLeave}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.25, minZoom: 0.45, maxZoom: 1.0 }}
        minZoom={0.15}
        maxZoom={2}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={32} size={0.5} color="var(--graph-grid)" />
        <Controls position="bottom-right" showInteractive={false} />
        {!isMobile && showMinimap && (
          <MiniMap
            nodeColor={minimapNodeColor}
            maskColor="var(--bg-overlay)"
            pannable
            zoomable
            style={{ background: "var(--bg-secondary)", borderRadius: 10, border: "1px solid var(--border-subtle)" }}
          />
        )}
      </ReactFlow>
    </div>
  );
}
