import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
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
  KeyRound,
  ListTodo,
  CalendarClock,
  Webhook,
  Cpu,
  CircleDot,
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

// ---------------------------------------------------------------------------
// Node kind config
// ---------------------------------------------------------------------------

type NodeKind = "system" | "agent" | "model" | "tool" | "secret" | "memory" | "role" | "task" | "schedule" | "webhook" | "worker";

const KIND_CONFIG: Record<NodeKind, { icon: React.ReactNode; colorVar: string }> = {
  system:  { icon: <Network   size={14} />, colorVar: "var(--accent)" },
  agent:   { icon: <Bot        size={14} />, colorVar: "var(--green)" },
  model:   { icon: <Database   size={14} />, colorVar: "var(--blue)" },
  tool:    { icon: <Wrench     size={14} />, colorVar: "var(--yellow)" },
  secret:  { icon: <Lock       size={14} />, colorVar: "var(--orange)" },
  memory:  { icon: <Brain      size={14} />, colorVar: "var(--purple)" },
  role:    { icon: <KeyRound   size={14} />, colorVar: "var(--purple)" },
  task:    { icon: <ListTodo   size={14} />, colorVar: "var(--blue)" },
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
  [key: string]: unknown;
}

function ResourceNode({ data }: { data: CrdNodeData }) {
  const cfg = KIND_CONFIG[data.kind];
  const isAgent = data.kind === "agent";

  return (
    <div className={clsx("gnode", `gnode--${data.kind}`, isAgent && data.isEntry && "gnode--entry", isAgent && data.isTerminal && "gnode--terminal", isAgent && data.hasJoin && "gnode--join")}>
      <Handle id="left" type="target" position={Position.Left} className="gnode__handle gnode__handle--target" />
      <Handle id="top" type="target" position={Position.Top} className="gnode__handle gnode__handle--target" />
      <div className="gnode__icon-ring" style={{ background: `color-mix(in srgb, ${cfg.colorVar} 18%, transparent)`, color: cfg.colorVar }}>
        {isAgent && data.isEntry ? <Play size={14} /> : isAgent && data.hasJoin ? <GitMerge size={14} /> : cfg.icon}
      </div>
      <div className="gnode__body">
        <div className="gnode__name">{data.label}</div>
        <div className="gnode__meta">
          {data.phase ? <StatusBadge phase={data.phase} /> : data.subtitle ? <span className="gnode__pos">{data.subtitle}</span> : <span className="gnode__pos">{data.kind}</span>}
          {data.hasJoin && <span className="gnode__join-tag">{data.joinMode ?? "wait_for_all"}</span>}
        </div>
      </div>
      <Handle id="right" type="source" position={Position.Right} className="gnode__handle gnode__handle--source" />
      <Handle id="bottom" type="source" position={Position.Bottom} className="gnode__handle gnode__handle--source" />
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
  g.setGraph({ rankdir: "LR", nodesep: 80, ranksep: 140, marginx: 50, marginy: 50 });

  const sysId = nid("system", system.metadata.name);
  g.setNode(sysId, { width: NODE_W + 20, height: NODE_H + 8 });

  // Track node metadata for React Flow construction
  interface NodeMeta { kind: NodeKind; label: string; phase?: string; subtitle?: string; extra?: Partial<CrdNodeData> }
  const nodeMeta: Record<string, NodeMeta> = {};
  const edgeList: { src: string; tgt: string; routing: boolean; agentToAgent: boolean }[] = [];
  const registeredNodes = new Set<string>();

  function regNode(id: string, kind: NodeKind, label: string, phase?: string, subtitle?: string, extra?: Partial<CrdNodeData>) {
    if (registeredNodes.has(id)) return;
    registeredNodes.add(id);
    g.setNode(id, { width: NODE_W, height: NODE_H });
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

  // System → entry agents (if no natural entry exists, e.g. a loop, use the first agent)
  if (entries.size > 0) {
    for (const aName of ordered) {
      if (entries.has(aName)) regEdge(sysId, nid("agent", aName), true);
    }
  } else if (ordered.length > 0) {
    regEdge(sysId, nid("agent", ordered[0]), true);
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

      // Secret under model endpoint (via auth.secretRef)
      const secRef = me.spec.auth?.secretRef;
      if (secRef && secretMap.has(secRef) && !addedSecretForModel.has(mid)) {
        addedSecretForModel.add(mid);
        const sec = secretMap.get(secRef)!;
        const keyCount = Object.keys(sec.spec.data ?? sec.spec.stringData ?? {}).length;
        const sid = nid("secret", secRef);
        regNode(sid, "secret", secRef, effectivePhase("secret", sec.status?.phase), `${keyCount} key${keyCount !== 1 ? "s" : ""}`);
        regEdge(mid, sid, false);
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

      // Secret under tool
      const secRef = tool.spec.auth?.secretRef;
      if (secRef && secretMap.has(secRef)) {
        const sec = secretMap.get(secRef)!;
        const keyCount = Object.keys(sec.spec.data ?? sec.spec.stringData ?? {}).length;
        const sid = nid("secret", secRef);
        regNode(sid, "secret", secRef, effectivePhase("secret", sec.status?.phase), `${keyCount} key${keyCount !== 1 ? "s" : ""}`);
        regEdge(tid, sid, false);
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
    }
  }

  // -- Tasks and workers connected to system -----------------------------------

  const systemTasks = (related.tasks ?? [])
    .filter((t) => t.spec.system === system.metadata.name)
    .filter((t) => !isAutomationSpawnedRunTask(t));
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
    if (!ref) continue;

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
    nodePos[id] = { x: pos.x, y: pos.y };
    nodes.push({
      id,
      type: "resource",
      position: { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 },
      data: { label: meta.label, kind: meta.kind, phase: meta.phase, subtitle: meta.subtitle, ...meta.extra },
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

    edges.push({
      id: eid,
      source: src,
      target: tgt,
      sourceHandle,
      targetHandle,
      animated: routing && animated,
      type: agentToAgent ? "default" : "smoothstep",
      markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14, color: routing ? "var(--accent)" : "var(--edge-stroke)" },
      style: { stroke: routing ? "var(--accent)" : "var(--edge-stroke)", strokeWidth: routing ? 2 : 1.5, strokeDasharray: routing ? undefined : "4 3" },
    });
  }

  return { nodes, edges };
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
  const agentNames = system.spec.agents ?? [];
  const graph = system.spec.graph ?? {};
  const blueprint = useMemo(() => detectBlueprint(agentNames, graph), [agentNames, graph]);
  const bpInfo = BLUEPRINT_META[blueprint];

  // Serialize runningAgents to a stable string so useMemo doesn't refire on identical Sets
  const runningKey = useMemo(() => runningAgents ? [...runningAgents].sort().join(",") : "", [runningAgents]);

  const { builtNodes, builtEdges } = useMemo(() => {
    const r = buildTree(system, related ?? {}, animated ?? false, runningAgents);
    return { builtNodes: r.nodes, builtEdges: r.edges };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [system, related, animated, runningKey]);

  const [nodes, setNodes, onNodesChange] = useNodesState(builtNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(builtEdges);

  useEffect(() => { setNodes(builtNodes); }, [builtNodes, setNodes]);
  useEffect(() => { setEdges(builtEdges); }, [builtEdges, setEdges]);

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      const sep = node.id.indexOf(":");
      if (sep > 0) onNodeClick?.(node.id.slice(0, sep), node.id.slice(sep + 1));
    },
    [onNodeClick],
  );

  const legendKinds: NodeKind[] = ["system", "agent", "model", "tool", "secret", "memory", "role", "task", "schedule", "webhook", "worker"];

  return (
    <div className="graph-view">
      <div className="graph-view__badge" style={{ color: bpInfo.color }}>{bpInfo.label}</div>
      <div className="graph-view__legend">
        {legendKinds.map((k) => (
          <span key={k} className="graph-view__legend-item">
            <CircleDot size={10} style={{ color: KIND_CONFIG[k].colorVar }} />
            <span>{k}</span>
          </span>
        ))}
      </div>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={handleNodeClick}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.15}
        maxZoom={2}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={24} size={1} color="var(--graph-grid)" />
        <Controls position="bottom-right" showInteractive={false} />
        {!isMobile && (
          <MiniMap
            nodeColor="var(--accent)"
            maskColor="var(--bg-overlay)"
            style={{ background: "var(--bg-secondary)", borderRadius: 8, border: "1px solid var(--card-border)" }}
          />
        )}
      </ReactFlow>
    </div>
  );
}
