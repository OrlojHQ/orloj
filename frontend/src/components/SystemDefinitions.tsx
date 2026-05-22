import type { Agent, ModelEndpoint, Tool, Worker } from "../api/types";

interface SystemDefinitionsProps {
  agents: Agent[];
  modelEndpoints: ModelEndpoint[];
  tools: Tool[];
  workers: Worker[];
  systemAgentNames: string[];
  apiReachable: boolean;
}

export function SystemDefinitions({
  agents,
  modelEndpoints,
  tools,
  workers,
  systemAgentNames,
  apiReachable,
}: SystemDefinitionsProps) {
  const systemAgents = agents.filter((a) =>
    systemAgentNames.includes(a.metadata.name),
  );
  const modelsUsed = new Set(
    systemAgents.map((a) => a.spec.model_ref).filter(Boolean),
  );
  const toolsUsed = new Set(systemAgents.flatMap((a) => a.spec.tools ?? []));

  const modelCount = modelEndpoints.filter((m) =>
    modelsUsed.has(m.metadata.name),
  ).length;
  const toolCount = tools.filter((t) => toolsUsed.has(t.metadata.name)).length;

  const workersOnline = workers.filter((w) => {
    const p = (w.status?.phase ?? "").toLowerCase();
    return p === "healthy" || p === "ready";
  }).length;

  const readyCount = modelEndpoints.filter(
    (m) =>
      modelsUsed.has(m.metadata.name) &&
      (m.status?.phase ?? "").toLowerCase() === "ready",
  ).length;

  return (
    <div className="sys-definitions">
      <h3 className="sys-definitions__title">SYSTEM DEFINITIONS</h3>
      <div className="sys-definitions__grid">
        <div className="sys-definitions__item">
          <span className="sys-definitions__item-label">Agents</span>
          <span className="sys-definitions__item-value">
            {systemAgentNames.length}
          </span>
        </div>
        <div className="sys-definitions__item">
          <span className="sys-definitions__item-label">Models</span>
          <span className="sys-definitions__item-value">{modelCount}</span>
        </div>
        <div className="sys-definitions__item">
          <span className="sys-definitions__item-label">Tools</span>
          <span className="sys-definitions__item-value">{toolCount}</span>
        </div>
        <div className="sys-definitions__item">
          <span className="sys-definitions__item-label">API reachability</span>
          <span className="sys-definitions__item-value">
            {apiReachable ? 1 : 0}
          </span>
        </div>
        <div className="sys-definitions__item">
          <span className="sys-definitions__item-label">Workers</span>
          <span className="sys-definitions__item-value">{workersOnline}</span>
        </div>
        <div className="sys-definitions__item">
          <span className="sys-definitions__item-label">Workers</span>
          <span className="sys-definitions__item-value">{workers.length}</span>
          <span className="sys-definitions__item-sub">Last 24 hours</span>
        </div>
      </div>
      <div className="sys-definitions__footer">
        <span className="sys-definitions__footer-item">Total on</span>
        <span className="sys-definitions__footer-item">{readyCount} ready</span>
      </div>
    </div>
  );
}
