import { ContextBackButton } from "../components/ContextBackButton";
import { useCapabilities } from "../api/hooks";
import { EmptyState } from "../components/EmptyState";
import { DetailSkeleton } from "../components/DetailSkeleton";
import { ListFetchError } from "../components/ListFetchError";
import { Sparkles } from "lucide-react";

export function Capabilities() {
  const { data, isLoading, isError, error, refetch } = useCapabilities();

  if (isLoading) {
    return <DetailSkeleton />;
  }

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Server capabilities</h1>
            <p className="page__subtitle">
              Extension and feature flags reported by <code className="mono">GET /v1/capabilities</code>
            </p>
          </div>
        </div>
      </div>

      {isError && (
        <ListFetchError
          message={error instanceof Error ? error.message : "Failed to load capabilities"}
          onRetry={() => void refetch()}
        />
      )}

      {!isError && data && (
        <>
          <p className="text-muted mb-md">
            Generated at: {data.generated_at ? new Date(data.generated_at).toLocaleString() : "—"}
          </p>
          {(data.capabilities ?? []).length === 0 ? (
            <EmptyState
              icon={<Sparkles size={40} />}
              title="No capabilities"
              description="The server returned an empty capability list."
            />
          ) : (
            <div className="table-wrapper">
              <table>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Enabled</th>
                    <th>Description</th>
                    <th>Source</th>
                  </tr>
                </thead>
                <tbody>
                  {(data.capabilities ?? []).map((c) => (
                    <tr key={c.id}>
                      <td className="mono">{c.id}</td>
                      <td>{c.enabled ? "Yes" : "No"}</td>
                      <td className="text-muted">{c.description ?? "—"}</td>
                      <td className="text-muted mono text-xs">{c.source ?? "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}
