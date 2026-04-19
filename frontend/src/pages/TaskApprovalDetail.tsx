import { useMemo, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import {
  useTaskApproval,
  useDeleteResource,
  useApproveTaskApproval,
  useDenyTaskApproval,
  useRequestChangesTaskApproval,
  useUpdateResource,
} from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft, CheckCircle, MessageSquareWarning, XCircle } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { TaskApproval } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

type Tab = "overview" | "output" | "yaml";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "—";
}

function formatOutput(value: unknown): string {
  if (typeof value === "string") return value;
  if (value == null) return "—";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function TaskApprovalDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const routeName = nameParam ?? "";
  const { data: approval, isLoading, isError, error } = useTaskApproval(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("TaskApproval");
  const updateMutation = useUpdateResource("TaskApproval");
  const approveMutation = useApproveTaskApproval();
  const denyMutation = useDenyTaskApproval();
  const requestChangesMutation = useRequestChangesTaskApproval();
  const [tab, setTab] = useState<Tab>("overview");
  const [comment, setComment] = useState("");
  const outputText = useMemo(() => formatOutput(approval?.spec.output), [approval?.spec.output]);

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Task approval"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={() => navigate("/approvals")}
      />
    );
  }

  if (isLoading || !approval) {
    return <div className="page"><div className="loading-placeholder">Loading approval...</div></div>;
  }

  const isPending = (approval.status?.phase ?? "Pending").toLowerCase() === "pending";
  const allowRequestChanges = approval.spec.allow_request_changes ?? true;
  const maxReviewCycles = approval.spec.max_review_cycles ?? 3;
  const reviewCycle = approval.spec.review_cycle ?? 1;
  const canRequestChanges = isPending && allowRequestChanges && reviewCycle < maxReviewCycles;
  const body = comment.trim() ? { comment: comment.trim() } : undefined;

  const handleApprove = async () => {
    try {
      await approveMutation.mutateAsync({ name: routeName, body });
      toast("success", "Approval granted");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to approve");
    }
  };

  const handleDeny = async () => {
    try {
      await denyMutation.mutateAsync({ name: routeName, body });
      toast("success", "Approval denied");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to deny");
    }
  };

  const handleRequestChanges = async () => {
    if (!comment.trim()) {
      toast("error", "Comment is required when requesting changes");
      return;
    }
    try {
      await requestChangesMutation.mutateAsync({ name: routeName, body: { comment: comment.trim() } });
      toast("success", "Requested changes");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to request changes");
    }
  };

  const handleDelete = async () => {
    if (!window.confirm(`Delete TaskApproval ${approval.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "TaskApproval deleted");
      navigate("/approvals");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete");
    }
  };

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "output", label: "Output" },
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
            <p className="page__subtitle">{approval.spec.checkpoint_id ?? "checkpoint"} · {approval.metadata.namespace}</p>
          </div>
          <StatusBadge phase={approval.status?.phase} size="md" />
        </div>
        <div className="page__header-actions">
          {isPending && (
            <>
              <button className="btn-primary" onClick={handleApprove} disabled={approveMutation.isPending}>
                <CheckCircle size={14} /> {approveMutation.isPending ? "Approving..." : "Approve"}
              </button>
              <button className="btn-secondary" onClick={handleRequestChanges} disabled={!canRequestChanges || requestChangesMutation.isPending}>
                <MessageSquareWarning size={14} /> {requestChangesMutation.isPending ? "Requesting..." : "Request Changes"}
              </button>
              <button className="btn-secondary text-red" onClick={handleDeny} disabled={denyMutation.isPending}>
                <XCircle size={14} /> {denyMutation.isPending ? "Denying..." : "Deny"}
              </button>
            </>
          )}
          <button className="btn-secondary text-red" onClick={handleDelete} disabled={deleteMutation.isPending}>
            {deleteMutation.isPending ? "Deleting..." : "Delete"}
          </button>
        </div>
      </div>

      {isPending && (
        <div className="card" style={{ marginBottom: 16 }}>
          <label className="detail-field__label" htmlFor="approval-comment">Reviewer Comment</label>
          <textarea
            id="approval-comment"
            className="yaml-editor"
            style={{ minHeight: 96 }}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder="Optional for approve/deny, required for request changes"
          />
        </div>
      )}

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
              <span className="detail-field__label">Checkpoint</span>
              <span className="detail-field__value mono">{approval.spec.checkpoint_id ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Checkpoint Type</span>
              <span className="detail-field__value">{approval.spec.checkpoint_type ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Agent</span>
              <span className="detail-field__value mono">{approval.spec.agent ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Task Ref</span>
              <span className="detail-field__value mono">{approval.spec.task_ref ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Review Cycle</span>
              <span className="detail-field__value">{reviewCycle}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Allow Request Changes</span>
              <span className="detail-field__value">{allowRequestChanges ? "true" : "false"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Max Review Cycles</span>
              <span className="detail-field__value">{maxReviewCycles}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Supersedes</span>
              <span className="detail-field__value mono">{approval.spec.supersedes ?? "—"}</span>
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
              <span className="detail-field__label">Comment</span>
              <span className="detail-field__value">{approval.status?.comment ?? "—"}</span>
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
        {tab === "output" && (
          <pre className="yaml-editor" style={{ whiteSpace: "pre-wrap" }}>{outputText}</pre>
        )}
        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(approval, null, 2)}
            editable
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<TaskApproval>(
                queryClient,
                "TaskApproval",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<TaskApproval>,
              );
              toast("success", "Approval updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.TaskApproval}/${encodeURIComponent(updated.metadata.name)}`,
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
