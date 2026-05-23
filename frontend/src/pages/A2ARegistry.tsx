import { useState } from "react";
import { useA2AAgents } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { AgentCardPreview } from "../components/AgentCardPreview";
import { EmptyState } from "../components/EmptyState";
import { Radio } from "lucide-react";
import type { AgentCard } from "../api/types";

export function A2ARegistry() {
  const { data, isLoading, isError, error } = useA2AAgents();
  const [selectedCard, setSelectedCard] = useState<AgentCard | null>(null);

  const localAgents = data?.localAgents ?? [];
  const remoteAgents = data?.remoteAgents ?? [];

  if (isLoading) {
    return (
      <div className="page">
        <div className="page__header">
          <h1 className="page__title">A2A Registry</h1>
        </div>
        <div className="loading-placeholder">Loading A2A agents...</div>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="page">
        <div className="page__header">
          <h1 className="page__title">A2A Registry</h1>
        </div>
        <p className="text-red">
          {error instanceof Error ? error.message : "Failed to load A2A agents"}
        </p>
      </div>
    );
  }

  const totalCount = localAgents.length + remoteAgents.length;

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">A2A Registry</h1>
          <p className="page__subtitle">
            {totalCount} agent{totalCount !== 1 ? "s" : ""} registered
          </p>
        </div>
      </div>

      {totalCount === 0 && (
        <EmptyState
          icon={<Radio size={40} />}
          title="No A2A Agents"
          description="No agents are registered in the A2A protocol registry."
        />
      )}

      {localAgents.length > 0 && (
        <section style={{ marginBottom: "2rem" }}>
          <h2 className="dash-section-heading">Local Agents</h2>
          <div className="resource-table__wrapper">
            <table className="resource-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>URL</th>
                  <th>Protocol</th>
                  <th>Capabilities</th>
                  <th>Skills</th>
                </tr>
              </thead>
              <tbody>
                {localAgents.map((agent) => (
                  <tr
                    key={agent.name}
                    className="resource-table__row resource-table__row--clickable"
                    onClick={() => setSelectedCard(agent)}
                  >
                    <td><span className="mono">{agent.name}</span></td>
                    <td><span className="text-muted mono text-ellipsis">{agent.url}</span></td>
                    <td>{agent.protocolVersion ?? "—"}</td>
                    <td>{formatCapabilities(agent)}</td>
                    <td className="text-muted">{agent.skills?.length ?? 0}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {remoteAgents.length > 0 && (
        <section style={{ marginBottom: "2rem" }}>
          <h2 className="dash-section-heading">Remote Agents</h2>
          <div className="resource-table__wrapper">
            <table className="resource-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>URL</th>
                  <th>Protocol</th>
                  <th>Cache Status</th>
                  <th>Last Refreshed</th>
                  <th>Health</th>
                </tr>
              </thead>
              <tbody>
                {remoteAgents.map((agent) => (
                  <tr
                    key={agent.name}
                    className="resource-table__row resource-table__row--clickable"
                    onClick={() => agent.card && setSelectedCard(agent.card)}
                  >
                    <td><span className="mono">{agent.name}</span></td>
                    <td><span className="text-muted mono text-ellipsis">{agent.url}</span></td>
                    <td>{agent.protocolVersion ?? "—"}</td>
                    <td><StatusBadge phase={agent.cacheStatus ?? "unknown"} /></td>
                    <td className="text-muted">
                      {agent.lastRefreshed ? new Date(agent.lastRefreshed).toLocaleString() : "—"}
                    </td>
                    <td>
                      {agent.error ? (
                        <span className="text-red" title={agent.error}>Error</span>
                      ) : (
                        <span className="text-green">Healthy</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {selectedCard && (
        <AgentCardPreview card={selectedCard} />
      )}
    </div>
  );
}

function formatCapabilities(agent: AgentCard): string {
  const caps: string[] = [];
  if (agent.capabilities?.streaming) caps.push("streaming");
  if (agent.capabilities?.pushNotifications) caps.push("push");
  if (agent.capabilities?.stateTransitionHistory) caps.push("history");
  return caps.length > 0 ? caps.join(", ") : "—";
}
