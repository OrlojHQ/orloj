export interface ObjectMeta {
  name: string;
  namespace?: string;
  labels?: Record<string, string>;
  resourceVersion?: string;
  generation?: number;
  createdAt?: string;
}

export interface AgentSystem {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: AgentSystemSpec;
  status?: AgentSystemStatus;
}

export interface AgentSystemSpec {
  agents?: string[];
  graph?: Record<string, GraphEdge>;
}

export interface GraphEdge {
  next?: string;
  edges?: GraphRoute[];
  join?: GraphJoin;
}

export interface GraphRoute {
  to: string;
  labels?: Record<string, string>;
  policy?: Record<string, string>;
}

export interface GraphJoin {
  mode?: string;
  quorum_count?: number;
  quorum_percent?: number;
  on_failure?: string;
}

export interface AgentSystemStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface Agent {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: AgentSpec;
  status?: AgentStatus;
}

export interface AgentSpec {
  model_ref?: string;
  prompt: string;
  tools?: string[];
  roles?: string[];
  memory?: MemorySpec;
  limits?: AgentLimits;
}

export interface MemorySpec {
  ref?: string;
  type?: string;
  provider?: string;
}

export interface AgentLimits {
  max_steps?: number;
  timeout?: string;
}

export interface AgentStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface ModelEndpoint {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: ModelEndpointSpec;
  status?: ModelEndpointStatus;
}

export interface ModelEndpointSpec {
  provider?: string;
  base_url?: string;
  default_model?: string;
  options?: Record<string, string>;
  auth?: { secretRef?: string };
}

export interface ModelEndpointStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface Tool {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: ToolSpec;
  status?: ToolStatus;
}

export interface ToolSpec {
  type?: string;
  endpoint?: string;
  /** Reference to an McpServer resource when tools are sourced from MCP. */
  mcp_server_ref?: string;
  capabilities?: string[];
  operation_classes?: string[];
  risk_level?: string;
  runtime?: ToolRuntimePolicy;
  auth?: ToolAuth;
}

export interface ToolAuth {
  profile?: string;
  profiles?: ToolAuthProfile[];
  secretRef?: string;
  headerName?: string;
  tokenURL?: string;
  scopes?: string[];
}

export interface ToolAuthProfile {
  name?: string;
  profile?: string;
  secretRef?: string;
  headerName?: string;
  tokenURL?: string;
  scopes?: string[];
}

export interface ToolRuntimePolicy {
  timeout?: string;
  isolation_mode?: string;
  retry?: {
    max_attempts?: number;
    backoff?: string;
    max_backoff?: string;
    jitter?: string;
  };
}

export interface ToolStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface Secret {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: SecretSpec;
  status?: SecretStatus;
}

export interface SecretSpec {
  data?: Record<string, string>;
  stringData?: Record<string, string>;
}

export interface SecretStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface Memory {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: MemoryConfig;
  status?: MemoryStatus;
}

export interface MemoryConfig {
  type?: string;
  provider?: string;
  embedding_model?: string;
  endpoint?: string;
  auth?: MemoryAuthConfig;
}

export interface MemoryAuthConfig {
  secretRef?: string;
}

export interface MemoryStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface MemoryEntry {
  key: string;
  value: string;
  score?: number;
}

export interface MemoryEntriesResponse {
  entries: MemoryEntry[];
  count: number;
}

export interface AgentPolicy {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: AgentPolicySpec;
  status?: PolicyStatus;
}

export interface AgentPolicySpec {
  max_tokens_per_run?: number;
  allowed_models?: string[];
  blocked_tools?: string[];
  apply_mode?: string;
  target_systems?: string[];
  target_tasks?: string[];
}

export interface PolicyStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface AgentRole {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: AgentRoleSpec;
  status?: AgentRoleStatus;
}

export interface AgentRoleSpec {
  description?: string;
  permissions?: string[];
}

export interface AgentRoleStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface ToolPermission {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: ToolPermissionSpec;
  status?: ToolPermissionStatus;
}

export interface ToolPermissionSpec {
  tool_ref?: string;
  action?: string;
  required_permissions?: string[];
  match_mode?: string;
  apply_mode?: string;
  target_agents?: string[];
  operation_rules?: OperationRule[];
}

export interface OperationRule {
  operation_class?: string;
  verdict?: string;
}

export interface ToolPermissionStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface Task {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: TaskSpec;
  status?: TaskStatus;
}

export interface TaskSpec {
  mode?: "run" | "template";
  system?: string;
  input?: Record<string, string>;
  priority?: string;
  max_turns?: number;
  retry?: { max_attempts?: number; backoff?: string };
  message_retry?: {
    max_attempts?: number;
    backoff?: string;
    max_backoff?: string;
    jitter?: string;
    non_retryable?: string[];
  };
  requirements?: { region?: string; gpu?: boolean; model?: string };
}

export interface TaskStatus {
  phase?: string;
  lastError?: string;
  startedAt?: string;
  completedAt?: string;
  nextAttemptAt?: string;
  attempts?: number;
  output?: Record<string, string>;
  assignedWorker?: string;
  claimedBy?: string;
  leaseUntil?: string;
  lastHeartbeat?: string;
  trace?: TaskTraceEvent[];
  history?: TaskHistoryEvent[];
  messages?: TaskMessage[];
  message_idempotency?: TaskMessageIdempotency[];
  join_states?: TaskJoinState[];
  observedGeneration?: number;
}

export interface TaskSchedule {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: TaskScheduleSpec;
  status?: TaskScheduleStatus;
}

export interface TaskScheduleSpec {
  task_ref?: string;
  schedule?: string;
  time_zone?: string;
  suspend?: boolean;
  starting_deadline_seconds?: number;
  concurrency_policy?: "forbid";
  successful_history_limit?: number;
  failed_history_limit?: number;
}

export interface TaskScheduleStatus {
  phase?: string;
  lastError?: string;
  lastScheduleTime?: string;
  lastSuccessfulTime?: string;
  nextScheduleTime?: string;
  lastTriggeredTask?: string;
  activeRuns?: string[];
  observedGeneration?: number;
}

export interface TaskWebhook {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: TaskWebhookSpec;
  status?: TaskWebhookStatus;
}

export interface TaskWebhookSpec {
  task_ref?: string;
  suspend?: boolean;
  auth?: TaskWebhookAuthSpec;
  idempotency?: TaskWebhookIdempotency;
  payload?: TaskWebhookPayloadSpec;
}

export interface TaskWebhookAuthSpec {
  profile?: "generic" | "github";
  secret_ref?: string;
  signature_header?: string;
  signature_prefix?: string;
  timestamp_header?: string;
  max_skew_seconds?: number;
}

export interface TaskWebhookIdempotency {
  event_id_header?: string;
  dedupe_window_seconds?: number;
}

export interface TaskWebhookPayloadSpec {
  mode?: "raw";
  input_key?: string;
}

export interface TaskWebhookStatus {
  phase?: string;
  lastError?: string;
  observedGeneration?: number;
  endpointID?: string;
  endpointPath?: string;
  lastDeliveryTime?: string;
  lastEventID?: string;
  lastTriggeredTask?: string;
  acceptedCount?: number;
  duplicateCount?: number;
  rejectedCount?: number;
}

export interface TaskTraceEvent {
  timestamp?: string;
  step_id?: string;
  attempt?: number;
  step?: number;
  branch_id?: string;
  type?: string;
  agent?: string;
  tool?: string;
  error_code?: string;
  error_reason?: string;
  retryable?: boolean;
  message?: string;
  latency_ms?: number;
  tokens?: number;
  input_tokens?: number;
  output_tokens?: number;
  token_usage_source?: string;
  tool_calls?: number;
}

export interface TaskHistoryEvent {
  timestamp?: string;
  type?: string;
  worker?: string;
  message?: string;
}

export interface TaskMessage {
  timestamp?: string;
  message_id?: string;
  idempotency_key?: string;
  task_id?: string;
  attempt?: number;
  system?: string;
  from_agent?: string;
  to_agent?: string;
  branch_id?: string;
  parent_branch_id?: string;
  type?: string;
  content?: string;
  trace_id?: string;
  parent_id?: string;
  phase?: string;
  attempts?: number;
  max_attempts?: number;
  last_error?: string;
  worker?: string;
  processed_at?: string;
  next_attempt_at?: string;
}

export interface TaskMessageIdempotency {
  key?: string;
  message_id?: string;
  state?: string;
  updated_at?: string;
  expires_at?: string;
  worker?: string;
}

export interface TaskJoinSource {
  message_id?: string;
  from_agent?: string;
  branch_id?: string;
  timestamp?: string;
  payload?: string;
}

export interface TaskJoinState {
  attempt?: number;
  node?: string;
  mode?: string;
  expected?: number;
  quorum_required?: number;
  activated?: boolean;
  activated_at?: string;
  activated_by?: string;
  sources?: TaskJoinSource[];
}

export interface ToolApproval {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: ToolApprovalSpec;
  status?: ToolApprovalStatus;
}

export interface ToolApprovalSpec {
  task_ref?: string;
  tool?: string;
  operation_class?: string;
  agent?: string;
  input?: Record<string, string>;
  reason?: string;
  ttl?: string;
}

export interface ToolApprovalStatus {
  phase?: string;
  decision?: string;
  decided_by?: string;
  decided_at?: string;
  expires_at?: string;
}

export interface Worker {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: WorkerSpec;
  status?: WorkerStatus;
}

export interface WorkerSpec {
  region?: string;
  capabilities?: { gpu?: boolean; supported_models?: string[] };
  max_concurrent_tasks?: number;
}

export interface WorkerStatus {
  phase?: string;
  lastError?: string;
  lastHeartbeat?: string;
  observedGeneration?: number;
  currentTasks?: number;
}

export interface McpServerEnvVar {
  name?: string;
  value?: string;
  secretRef?: string;
}

export interface McpToolFilter {
  include?: string[];
}

export interface McpReconnectPolicy {
  max_attempts?: number;
  backoff?: string;
}

export interface McpServerSpec {
  transport?: string;
  command?: string;
  args?: string[];
  env?: McpServerEnvVar[];
  endpoint?: string;
  auth?: ToolAuth;
  tool_filter?: McpToolFilter;
  reconnect?: McpReconnectPolicy;
}

export interface McpServerStatus {
  phase?: string;
  discoveredTools?: string[];
  generatedTools?: string[];
  lastSyncedAt?: string;
  lastError?: string;
  observedGeneration?: number;
}

export interface McpServer {
  apiVersion: string;
  kind: string;
  metadata: ObjectMeta;
  spec: McpServerSpec;
  status?: McpServerStatus;
}

export interface ListResponse<T> {
  items: T[];
  /** Cursor for the next page; pass as `after` query param. Omitted when there is no next page. */
  continue?: string;
}

export interface Capability {
  id: string;
  enabled: boolean;
  description?: string;
  source?: string;
}

export interface CapabilitySnapshot {
  generated_at: string;
  capabilities: Capability[];
}

/** Aggregate counts from GET /tasks/{name}/metrics (`response.totals`). */
export interface TaskMetricsTotals {
  messages: number;
  queued: number;
  running: number;
  retrypending: number;
  succeeded: number;
  deadletter: number;
  in_flight: number;
  retry_count: number;
  deadletters: number;
  latency_ms_avg: number;
  latency_ms_p95: number;
  latency_sample_size: number;
}

export interface TaskMetricsPerAgent {
  agent: string;
  inbound: number;
  outbound: number;
  queued: number;
  running: number;
  retrypending: number;
  succeeded: number;
  deadletter: number;
  in_flight: number;
  retry_count: number;
  deadletters: number;
  latency_ms_avg: number;
  latency_ms_p95: number;
  latency_sample_size: number;
}

export interface TaskMetricsPerEdge {
  from_agent: string;
  to_agent: string;
  messages: number;
  queued: number;
  running: number;
  retrypending: number;
  succeeded: number;
  deadletter: number;
  in_flight: number;
  retry_count: number;
  deadletters: number;
  latency_ms_avg: number;
  latency_ms_p95: number;
  latency_sample_size: number;
}

/** Full JSON body from GET /tasks/{name}/metrics. */
export interface TaskMetrics {
  name: string;
  namespace: string;
  generated_at: string;
  totals: TaskMetricsTotals;
  per_agent: TaskMetricsPerAgent[];
  per_edge: TaskMetricsPerEdge[];
  filters?: Record<string, unknown>;
}

export type ResourceKind =
  | "Agent"
  | "AgentSystem"
  | "ModelEndpoint"
  | "Tool"
  | "Secret"
  | "Memory"
  | "AgentPolicy"
  | "AgentRole"
  | "ToolPermission"
  | "ToolApproval"
  | "Task"
  | "TaskSchedule"
  | "TaskWebhook"
  | "Worker"
  | "McpServer";

export const RESOURCE_ENDPOINTS: Record<ResourceKind, string> = {
  Agent: "agents",
  AgentSystem: "agent-systems",
  ModelEndpoint: "model-endpoints",
  Tool: "tools",
  Secret: "secrets",
  Memory: "memories",
  AgentPolicy: "agent-policies",
  AgentRole: "agent-roles",
  ToolPermission: "tool-permissions",
  ToolApproval: "tool-approvals",
  Task: "tasks",
  TaskSchedule: "task-schedules",
  TaskWebhook: "task-webhooks",
  Worker: "workers",
  McpServer: "mcp-servers",
};
