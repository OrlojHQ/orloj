import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useMcpServers } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Plug, Plus } from "lucide-react";
import type { McpServer } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function McpServers() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading } = useMcpServers();
  const [showCreate, setShowCreate] = useState(false);
  const servers = data ?? [];

  const columns: Column<McpServer>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "transport", header: "Transport", render: (r) => r.spec.transport ?? "—", width: "100px" },
    {
      key: "target",
      header: "Command / Endpoint",
      render: (r) => (
        <span className="text-muted mono text-ellipsis">
          {r.spec.transport === "http" ? r.spec.endpoint ?? "—" : r.spec.command ?? "—"}
        </span>
      ),
    },
    {
      key: "tools",
      header: "Generated tools",
      render: (r) => (
        <span className="text-muted">{(r.status?.generatedTools ?? []).length || "—"}</span>
      ),
      width: "120px",
    },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">MCP Servers</h1>
            <p className="page__subtitle">{servers.length} servers</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New MCP Server
          </button>
        </div>
      </div>
      {servers.length === 0 && !isLoading ? (
        <EmptyState
          icon={<Plug size={40} />}
          title="No MCP Servers"
          description="Connect stdio or HTTP MCP servers; discovered tools can be synced as Tool resources."
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={servers}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) =>
            navigate(`/mcp-servers/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))
          }
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="McpServer" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
