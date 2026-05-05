import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useToolApproval, useDeleteResource, useApproveToolApproval, useDenyToolApproval, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft, CheckCircle, XCircle } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { ToolApproval } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

type Tab = "overview" | "yaml";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "—";
}

function formatInput(raw?: string): string {
  if (!raw) return "";
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

export function ToolApprovalDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const routeName = nameParam ?? "";
  const { data: approval, isLoading, isError, error } = useToolApproval(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("ToolApproval");
  const updateMutation = useUpdateResource("ToolApproval");
  const approveMutation = useApproveToolApproval();
  const denyMutation = useDenyToolApproval();
  const [tab, setTab] = useState<Tab>("overview");
  const [comment, setComment] = useState("");

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Tool approval"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={() => navigate("/approvals")}
      />
    );
  }

  if (isLoading || !approval) {
    return <div className="page"><div className="loading-placeholder">Loading approval...</div></div>;
  }

  const isPending = (approval.status?.phase ?? "Pending").toLowerCase() === "pending";

  const handleApprove = async () => {
    try {
      await approveMutation.mutateAsync({ name: routeName, body: comment.trim() ? { comment: comment.trim() } : undefined });
      toast("success", "Approval granted");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to approve");
    }
  };

  const handleDeny = async () => {
    try {
      await denyMutation.mutateAsync({ name: routeName, body: comment.trim() ? { comment: comment.trim() } : undefined });
      toast("success", "Approval denied");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to deny");
    }
  };

  const handleDelete = async () => {
    if (!window.confirm(`Delete ToolApproval ${approval.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
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
            {isPending && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Comment</span>
                <textarea
                  className="yaml-editor"
                  style={{ minHeight: 88 }}
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder="Optional reviewer comment"
                />
              </div>
            )}
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
            {approval.spec.input && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Tool Input</span>
                <pre className="yaml-editor" style={{ margin: 0, padding: 12, whiteSpace: "pre-wrap", wordBreak: "break-word", maxHeight: 320, overflow: "auto" }}>
                  {formatInput(approval.spec.input)}
                </pre>
              </div>
            )}
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
              <span className="detail-field__label">Comment</span>
              <span className="detail-field__value">{approval.status?.comment ?? "—"}</span>
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
              const updated = await saveNamespacedResourceYaml<ToolApproval>(
                queryClient,
                "ToolApproval",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<ToolApproval>,
              );
              toast("success", "Approval updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.ToolApproval}/${encodeURIComponent(updated.metadata.name)}`,
                  { replace: true },
                );
              }
            }}
          />
        )}
      </div>
    </div>
  );
}
