import { useState, type FormEvent } from "react";
import { MessagesSquare, Plus, X } from "lucide-react";
import { useLocation, useNavigate } from "react-router-dom";
import { useCreateSession, useSessions } from "../api/hooks";
import type { Session } from "../api/types";
import { EmptyState } from "../components/EmptyState";
import { ListFetchError } from "../components/ListFetchError";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useAppStore } from "../store";

function sessionPhase(session: Session): string | undefined {
  return session.status?.phase;
}

export function Sessions() {
  const navigate = useNavigate();
  const location = useLocation();
  const namespace = useAppStore((state) => state.namespace);
  const sessionsQuery = useSessions();
  const createMutation = useCreateSession();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [system, setSystem] = useState("");

  const sessions = sessionsQuery.data ?? [];
  const columns: Column<Session>[] = [
    {
      key: "name",
      header: "Name",
      render: (session) => <span className="mono">{session.metadata.name}</span>,
    },
    {
      key: "system",
      header: "Agent system",
      render: (session) => session.spec.system ?? "—",
    },
    {
      key: "phase",
      header: "Phase",
      render: (session) => <StatusBadge phase={sessionPhase(session)} />,
      width: "140px",
    },
    {
      key: "turns",
      header: "Turns",
      render: (session) => session.status?.completedTurns ?? 0,
      width: "90px",
    },
    {
      key: "updated",
      header: "Updated",
      render: (session) => {
        const timestamp = session.status?.lastActivityAt ?? session.metadata.createdAt;
        return timestamp ? new Date(timestamp).toLocaleString() : "—";
      },
    },
  ];

  const closeCreate = () => {
    if (createMutation.isPending) return;
    setCreateOpen(false);
    createMutation.reset();
  };

  const handleCreate = async (event: FormEvent) => {
    event.preventDefault();
    const trimmedName = name.trim();
    const trimmedSystem = system.trim();
    if (!trimmedName || !trimmedSystem) return;

    try {
      const created = await createMutation.mutateAsync({
        apiVersion: "orloj.dev/v1",
        kind: "Session",
        metadata: { name: trimmedName, namespace },
        spec: { system: trimmedSystem },
      });
      setCreateOpen(false);
      setName("");
      setSystem("");
      navigate(`/sessions/${encodeURIComponent(created.metadata.name)}`);
    } catch {
      // The mutation error is shown in the dialog.
    }
  };

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Sessions</h1>
          <p className="page__subtitle">{sessions.length} interactive sessions</p>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setCreateOpen(true)}>
            <Plus size={14} /> New session
          </button>
        </div>
      </div>

      {sessionsQuery.isError && (
        <ListFetchError
          message={
            sessionsQuery.error instanceof Error
              ? sessionsQuery.error.message
              : "Failed to load sessions"
          }
          onRetry={() => void sessionsQuery.refetch()}
        />
      )}

      {sessions.length === 0 && !sessionsQuery.isLoading && !sessionsQuery.isError ? (
        <EmptyState
          icon={<MessagesSquare size={40} />}
          title="No sessions"
          description="Start an interactive session to exchange turns with an agent system."
          action={
            <button className="btn-primary" onClick={() => setCreateOpen(true)}>
              <Plus size={14} /> New session
            </button>
          }
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={sessions}
          rowKey={(session) => session.metadata.name}
          onRowClick={(session) =>
            navigate(
              `/sessions/${encodeURIComponent(session.metadata.name)}`,
              detailListNavState(location),
            )
          }
          loading={sessionsQuery.isLoading}
        />
      )}

      {createOpen && (
        <div className="confirm-overlay" role="presentation" onMouseDown={closeCreate}>
          <form
            className="session-create-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-session-title"
            onSubmit={handleCreate}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="session-create-dialog__header">
              <div>
                <h2 id="create-session-title">New session</h2>
                <p className="text-muted text-xs">Connect a conversation to an agent system.</p>
              </div>
              <button
                type="button"
                className="btn-ghost"
                aria-label="Close"
                onClick={closeCreate}
              >
                <X size={16} />
              </button>
            </div>
            <div className="session-create-dialog__body">
              <label>
                <span>Name</span>
                <input
                  autoFocus
                  required
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="support-session"
                />
              </label>
              <label>
                <span>Agent system</span>
                <input
                  required
                  value={system}
                  onChange={(event) => setSystem(event.target.value)}
                  placeholder="support-pipeline"
                />
              </label>
              {createMutation.isError && (
                <p className="text-red text-xs" role="alert">
                  {createMutation.error instanceof Error
                    ? createMutation.error.message
                    : "Failed to create session"}
                </p>
              )}
            </div>
            <div className="session-create-dialog__footer">
              <button type="button" className="btn-secondary" onClick={closeCreate}>
                Cancel
              </button>
              <button
                type="submit"
                className="btn-primary"
                disabled={!name.trim() || !system.trim() || createMutation.isPending}
              >
                {createMutation.isPending ? "Creating…" : "Create session"}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
