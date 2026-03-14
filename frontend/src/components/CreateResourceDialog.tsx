import { useState } from "react";
import { X } from "lucide-react";
import { YamlEditor } from "./YamlEditor";
import { useCreateResource } from "../api/hooks";
import { toast } from "./Toast";
import type { ResourceKind } from "../api/types";

const TEMPLATES: Record<string, string> = {
  Agent: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "Agent",
    metadata: { name: "my-agent", namespace: "default" },
    spec: { model: "gpt-4o-mini", prompt: "You are a helpful assistant.", tools: [], limits: { max_steps: 10 } },
  }, null, 2),
  AgentSystem: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "AgentSystem",
    metadata: { name: "my-system", namespace: "default" },
    spec: { agents: ["agent-a", "agent-b"], graph: { "agent-a": { edges: [{ to: "agent-b" }] } } },
  }, null, 2),
  Task: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "Task",
    metadata: { name: "my-task", namespace: "default" },
    spec: { system: "my-system", input: { query: "Hello" }, priority: "normal" },
  }, null, 2),
  TaskSchedule: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "TaskSchedule",
    metadata: { name: "my-task-schedule", namespace: "default" },
    spec: {
      task_ref: "my-template-task",
      schedule: "*/5 * * * *",
      time_zone: "UTC",
      suspend: false,
      starting_deadline_seconds: 300,
      concurrency_policy: "forbid",
      successful_history_limit: 10,
      failed_history_limit: 3,
    },
  }, null, 2),
  TaskWebhook: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "TaskWebhook",
    metadata: { name: "my-task-webhook", namespace: "default" },
    spec: {
      task_ref: "my-template-task",
      suspend: false,
      auth: {
        profile: "generic",
        secret_ref: "webhook-shared-secret",
        signature_header: "X-Signature",
        signature_prefix: "sha256=",
        timestamp_header: "X-Timestamp",
        max_skew_seconds: 300,
      },
      idempotency: {
        event_id_header: "X-Event-Id",
        dedupe_window_seconds: 86400,
      },
      payload: {
        mode: "raw",
        input_key: "webhook_payload",
      },
    },
  }, null, 2),
};

interface CreateResourceDialogProps {
  kind: ResourceKind;
  open: boolean;
  onClose: () => void;
}

export function CreateResourceDialog({ kind, open, onClose }: CreateResourceDialogProps) {
  const [yaml, setYaml] = useState(TEMPLATES[kind] ?? JSON.stringify({ apiVersion: "orloj.dev/v1", kind, metadata: { name: "", namespace: "default" }, spec: {} }, null, 2));
  const createMutation = useCreateResource(kind);

  if (!open) return null;

  const handleCreate = async () => {
    try {
      const body = JSON.parse(yaml);
      await createMutation.mutateAsync(body);
      toast("success", `${kind} created successfully`);
      onClose();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to create resource");
    }
  };

  return (
    <div className="search-overlay" onClick={onClose}>
      <div className="create-dialog" onClick={(e) => e.stopPropagation()}>
        <div className="create-dialog__header">
          <h2>Create {kind}</h2>
          <button className="detail-panel__close" onClick={onClose} aria-label="Close">
            <X size={18} />
          </button>
        </div>
        <div className="create-dialog__body">
          <YamlEditor value={yaml} onChange={setYaml} readOnly={false} height="400px" />
        </div>
        <div className="create-dialog__footer">
          <button className="btn-secondary" onClick={onClose}>Cancel</button>
          <button className="btn-primary" onClick={handleCreate} disabled={createMutation.isPending}>
            {createMutation.isPending ? "Creating..." : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}
