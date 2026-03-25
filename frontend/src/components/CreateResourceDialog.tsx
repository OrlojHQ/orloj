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
    spec: { model_ref: "openai-default", prompt: "You are a helpful assistant.", tools: [], limits: { max_steps: 10 } },
  }, null, 2),
  AgentSystem: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "AgentSystem",
    metadata: { name: "my-system", namespace: "default" },
    spec: { agents: ["agent-a", "agent-b"], graph: { "agent-a": { edges: [{ to: "agent-b" }] } } },
  }, null, 2),
  Tool: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "Tool",
    metadata: { name: "my-tool", namespace: "default" },
    spec: {
      type: "http",
      endpoint: "https://api.example.com/action",
      capabilities: ["search"],
      operation_classes: ["read"],
      risk_level: "low",
      runtime: { timeout: "30s", isolation_mode: "none" },
    },
  }, null, 2),
  Secret: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "Secret",
    metadata: { name: "my-secret", namespace: "default" },
    spec: { stringData: { api_key: "your-key-here" } },
  }, null, 2),
  ModelEndpoint: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "ModelEndpoint",
    metadata: { name: "my-endpoint", namespace: "default" },
    spec: {
      provider: "openai",
      base_url: "https://api.openai.com/v1",
      default_model: "gpt-4o",
      auth: { secretRef: "my-api-key-secret" },
    },
  }, null, 2),
  Memory: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "Memory",
    metadata: { name: "my-memory", namespace: "default" },
    spec: { type: "vector", provider: "in-memory", embedding_model: "text-embedding-3-small" },
  }, null, 2),
  AgentPolicy: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "AgentPolicy",
    metadata: { name: "my-policy", namespace: "default" },
    spec: {
      max_tokens_per_run: 100000,
      allowed_models: ["gpt-4o", "gpt-4o-mini"],
      blocked_tools: [],
      apply_mode: "global",
    },
  }, null, 2),
  AgentRole: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "AgentRole",
    metadata: { name: "my-role", namespace: "default" },
    spec: { description: "Standard agent role", permissions: ["tool:invoke"] },
  }, null, 2),
  ToolPermission: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "ToolPermission",
    metadata: { name: "my-tool-permission", namespace: "default" },
    spec: {
      tool_ref: "my-tool",
      action: "invoke",
      required_permissions: ["tool:invoke"],
      match_mode: "all",
      apply_mode: "global",
      operation_rules: [{ operation_class: "read", verdict: "allow" }],
    },
  }, null, 2),
  ToolApproval: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "ToolApproval",
    metadata: { name: "my-approval", namespace: "default" },
    spec: {
      task_ref: "my-task",
      tool: "my-tool",
      operation_class: "write",
      agent: "my-agent",
      reason: "Requires human authorization for write operation",
      ttl: "10m",
    },
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
  McpServer: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "McpServer",
    metadata: { name: "my-mcp-server", namespace: "default" },
    spec: {
      transport: "stdio",
      command: "npx",
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      reconnect: { max_attempts: 3, backoff: "2s" },
    },
  }, null, 2),
  Worker: JSON.stringify({
    apiVersion: "orloj.dev/v1",
    kind: "Worker",
    metadata: { name: "worker-1", namespace: "default" },
    spec: {
      region: "us-east-1",
      capabilities: { gpu: false, supported_models: [] },
      max_concurrent_tasks: 4,
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
          <YamlEditor value={yaml} onChange={setYaml} readOnly={false} height="clamp(200px, 50vh, 400px)" />
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
