import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useToolApproval, useDeleteResource, useApproveToolApproval, useDenyToolApproval, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft, CheckCircle, XCircle } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "—";
}

export function ToolApprovalDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: approval, isLoading } = useToolApproval(name ?? "");
  const deleteMutation = useDeleteResource("ToolApproval");
  const updateMutation = useUpdateResource("ToolApproval");
  const approveMutation = useApproveToolApproval();
  const denyMutation = useDenyToolApproval();
  const [tab, setTab] = useState<Tab>("overview");

  if (isLoading || !approval) {
    return <div className="page"><div className="loading-placeholder">Loading approval...</div></div>;
  }

  const isPending = (approval.status?.phase ?? "Pending").toLowerCase() === "pending";

  const handleApprove = async () => {
    try {
      await approveMutation.mutateAsync(approval.metadata.name);
      toast("success", "Approval granted");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to approve");
    }
  };

  const handleDeny = async () => {
    try {
      await denyMutation.mutateAsync(approval.metadata.name);
      toast("success", "Approval denied");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to deny");
    }
  };

  const handleDelete = async () => {
    if (!window.confirm(`Delete ToolApproval ${approval.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(approval.metadata.name);
      toast("success", "ToolApproval deleted");
      navigate("/approvals");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete");
    }
  };

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/approvals")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{approval.metadata.name}</h1>
            <p className="page__subtitle">{approval.spec.tool ?? "—"} · {approval.metadata.namespace}</p>
          </div>
          <StatusBadge phase={approval.status?.phase} size="md" />
        </div>
        <div className="page__header-actions">
          {isPending && (
            <>
              <button
                className="btn-primary"
                onClick={handleApprove}
                disabled={approveMutation.isPending}
              >
                <CheckCircle size={14} /> {approveMutation.isPending ? "Approving..." : "Approve"}
              </button>
              <button
                className="btn-secondary text-red"
                onClick={handleDeny}
                disabled={denyMutation.isPending}
              >
                <XCircle size={14} /> {denyMutation.isPending ? "Denying..." : "Deny"}
              </button>
            </>
          )}
          <button
            className="btn-secondary text-red"
            onClick={handleDelete}
            disabled={deleteMutation.isPending}
          >
            {deleteMutation.isPending ? "Deleting..." : "Delete"}
          </button>
        </div>
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
              <StatusBadge phase={approval.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Tool</span>
              <span className="detail-field__value mono">{approval.spec.tool ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Operation Class</span>
              <span className="detail-field__value">{approval.spec.operation_class ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Agent</span>
              <span className="detail-field__value mono">{approval.spec.agent ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Task Ref</span>
              <span
                className={clsx("detail-field__value mono", approval.spec.task_ref && "detail-field__link")}
                onClick={() => { if (approval.spec.task_ref) navigate(`/tasks/${approval.spec.task_ref}`); }}
              >
                {approval.spec.task_ref ?? "—"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">TTL</span>
              <span className="detail-field__value">{approval.spec.ttl ?? "10m"}</span>
            </div>
            {approval.spec.reason && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Reason</span>
                <span className="detail-field__value">{approval.spec.reason}</span>
              </div>
            )}
            <div className="detail-field">
              <span className="detail-field__label">Decision</span>
              <span className="detail-field__value">{approval.status?.decision ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Decided By</span>
              <span className="detail-field__value">{approval.status?.decided_by ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Decided At</span>
              <span className="detail-field__value">{formatDateTime(approval.status?.decided_at)}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Expires At</span>
              <span className="detail-field__value">{formatDateTime(approval.status?.expires_at)}</span>
            </div>
          </div>
        )}
        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(approval, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: approval.metadata.name, body, rv: approval.metadata.resourceVersion });
              toast("success", "Approval updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
