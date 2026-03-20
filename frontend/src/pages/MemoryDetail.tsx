import { useState } from "react";
import { useParams } from "react-router-dom";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useMemory, useMemoryEntries, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft, Search } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "entries" | "yaml";

export function MemoryDetail() {
  const { name } = useParams<{ name: string }>();
  const { goBack } = useDetailReturnNav("/memories");
  const { data: memory, isLoading } = useMemory(name ?? "");
  const deleteMutation = useDeleteResource("Memory");
  const updateMutation = useUpdateResource("Memory");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "entries", label: "Entries" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !memory) {
    return <div className="page"><div className="loading-placeholder">Loading memory...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete Memory ${memory.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(memory.metadata.name);
      toast("success", "Memory deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Memory");
    }
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{memory.metadata.name}</h1>
            <p className="page__subtitle">
              {memory.spec.type ?? "—"} · {memory.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={memory.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Memory"}
        </button>
      </div>

      <div className="tab-bar">
        {tabs.map((t) => (
          <button
            key={t.id}
            className={clsx("tab-bar__tab", tab === t.id && "tab-bar__tab--active")}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="tab-content">
        {tab === "overview" && (
          <div className="detail-grid">
            <div className="detail-field">
              <span className="detail-field__label">Phase</span>
              <StatusBadge phase={memory.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Type</span>
              <span className="detail-field__value">{memory.spec.type ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Provider</span>
              <span className="detail-field__value">{memory.spec.provider ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Embedding Model</span>
              <span className="detail-field__value mono">{memory.spec.embedding_model ?? "-"}</span>
            </div>
            {memory.spec.endpoint && (
              <div className="detail-field">
                <span className="detail-field__label">Endpoint</span>
                <span className="detail-field__value mono">{memory.spec.endpoint}</span>
              </div>
            )}
            {memory.spec.auth?.secretRef && (
              <div className="detail-field">
                <span className="detail-field__label">Auth Secret</span>
                <span className="detail-field__value mono">{memory.spec.auth.secretRef}</span>
              </div>
            )}
            {memory.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{memory.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "entries" && <MemoryEntriesTab name={name ?? ""} />}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(memory, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: memory.metadata.name, body, rv: memory.metadata.resourceVersion });
              toast("success", "Memory updated");
            }}
          />
        )}
      </div>
    </div>
  );
}

function MemoryEntriesTab({ name }: { name: string }) {
  const [search, setSearch] = useState("");
  const [submitted, setSubmitted] = useState("");
  const params = submitted ? { q: submitted, limit: 100 } : { limit: 100 };
  const { data, isLoading } = useMemoryEntries(name, params);
  const entries = data?.entries ?? [];

  return (
    <div className="memory-entries">
      <form
        className="memory-entries__search"
        onSubmit={(e) => {
          e.preventDefault();
          setSubmitted(search.trim());
        }}
      >
        <div className="memory-entries__search-field">
          <Search size={14} className="memory-entries__search-icon" />
          <input
            type="text"
            placeholder="Search memory entries..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="memory-entries__search-input"
          />
        </div>
        <button type="submit" className="btn-secondary">
          Search
        </button>
        {submitted && (
          <button
            type="button"
            className="btn-ghost"
            onClick={() => {
              setSearch("");
              setSubmitted("");
            }}
          >
            Clear
          </button>
        )}
      </form>

      {isLoading ? (
        <div className="loading-placeholder">Loading entries...</div>
      ) : entries.length === 0 ? (
        <div className="empty-state-inline">
          {submitted ? "No entries match your search." : "No entries stored yet."}
        </div>
      ) : (
        <>
          <div className="memory-entries__count">
            {data?.count ?? 0} {submitted ? "results" : "entries"}
          </div>
          <div className="memory-entries__table">
            <table>
              <thead>
                <tr>
                  <th style={{ width: "30%" }}>Key</th>
                  <th>Value</th>
                  {submitted && <th style={{ width: "80px" }}>Score</th>}
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.key}>
                    <td className="mono">{entry.key}</td>
                    <td className="memory-entries__value">
                      {entry.value.length > 300
                        ? entry.value.slice(0, 300) + "..."
                        : entry.value}
                    </td>
                    {submitted && (
                      <td className="mono">
                        {entry.score != null ? entry.score.toFixed(2) : "—"}
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
